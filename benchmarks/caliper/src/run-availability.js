#!/usr/bin/env node
'use strict';

// Ronda de disponibilidad de Fabric (EVAL-4, #44; protocolo, secciones 3.3, 8.1
// y 9.5). Corre la carga mixta de referencia sin modificar sus workloads y, desde
// este proceso, da de baja el componente del escenario a los 60 s de iniciada la
// carga y lo restaura a los 120 s (salvo Raft-2). Despues clasifica cada
// operacion del crudo, conserva los logs que explican la falla y emite la
// metadata con su faultInjection.

const fs = require('node:fs');
const path = require('node:path');
const {spawn, spawnSync} = require('node:child_process');
const timers = require('node:timers/promises');

const {readSnapshot, sha256File} = require('./artifacts');
const {
    ORDERERS, PEERS, PEER_EVENT, RAFT_EVENT, SIMULATION,
    buildAvailabilityPlan, classifyOperations, eventLines, faultWindows, leaderFromLogs, orderersToStop, probeSequences,
    scenarioDefinition, schedule, withBoundedReconnect,
} = require('./availability');
const {resolveOrganizationCredentials} = require('./credentials');
const {extractContractError} = require('./contract-errors');
const {waitForDiscovery} = require('./discovery-readiness');
const {GatewayPool} = require('./gateway-bridge');
const {buildHostMetadata} = require('./host');
const {buildRoundMetadata, writeJSONAtomic} = require('./metadata');
const {buildMultiOrganizationNetworkConfig, inspectOrganizationRuntime} = require('./network-config');
const {MIX_PATTERN, buildPlan, candidatePools, plannedCalls} = require('./planner');
const {buildProfile} = require('./profiles');
const {aggregateResults} = require('./raw-results');
const {normalizeInvocation} = require('./requests');
const {buildBenchmarkConfig, createRunDirectory} = require('./run-round');
const {assertSnapshotCompatibility} = require('./snapshot');
const {readPreconditions, sequencesForPlan, verifyPlanPreconditions} = require('./snapshot-preconditions');
const {loadDatasetBundle, readSourceTruth} = require('./sources');

const CALIPER_VERSION = require('../package.json').devDependencies['@hyperledger/caliper-cli'];
const MIXED_RATE = 20;
const PROBE_RATE = 1;
const PROBE_REASON = 'cuarentena preventiva dispuesta por ANMAT (sonda de disponibilidad EVAL-4)';
const LOG_MARGIN_MS = 15_000;

function parseArguments(arguments_) {
    const values = {};
    for (let index = 0; index < arguments_.length; index += 2) {
        const name = arguments_[index];
        const value = arguments_[index + 1];
        if (!name?.startsWith('--') || value === undefined || value.startsWith('--')) {
            throw new Error('arguments must be supplied as --name value pairs');
        }
        const key = name.slice(2);
        if (values[key] !== undefined) throw new Error(`duplicate argument --${key}`);
        values[key] = value;
    }
    const supported = new Set(['scenario', 'phase', 'repetition', 'snapshot-dir', 'dataset-dir', 'output-root', 'run-token']);
    for (const key of Object.keys(values)) if (!supported.has(key)) throw new Error(`unsupported argument --${key}`);
    for (const required of ['scenario', 'phase', 'repetition', 'snapshot-dir']) {
        if (!values[required]) throw new Error(`--${required} is required`);
    }
    scenarioDefinition(values.scenario);
    return {
        scenario: values.scenario,
        phase: values.phase,
        repetition: values.repetition,
        snapshotDirectory: values['snapshot-dir'],
        datasetDirectory: values['dataset-dir'],
        outputRoot: values['output-root'],
        runToken: values['run-token'],
    };
}

// La carga es la mixta de EVAL-2 con la misma tasa y el mismo plan
// determinístico; sólo se extiende a las tres ventanas de 60 s.
function availabilityProfile(options, durationSeconds) {
    const mixed = buildProfile({scenario: 'mixed', phase: options.phase, repetition: options.repetition, rate: String(MIXED_RATE)});
    return Object.freeze({...mixed, durationSeconds});
}

function docker(args, options = {}) {
    const result = spawnSync('docker', args, {encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], maxBuffer: 256 * 1024 * 1024});
    if (result.error) throw result.error;
    if (result.status !== 0 && !options.allowFailure) {
        throw new Error(`docker ${args.join(' ')} failed: ${(result.stderr || result.stdout || '').trim()}`);
    }
    return `${result.stdout ?? ''}${options.withStderr ? result.stderr ?? '' : ''}`;
}

function containerLogs(container, options = {}) {
    const args = ['logs', '--timestamps'];
    if (options.since) args.push('--since', options.since);
    if (options.until) args.push('--until', options.until);
    args.push(container);
    return docker(args, {allowFailure: true, withStderr: true});
}

function currentLeader() {
    const logs = Object.fromEntries(ORDERERS.map((orderer) => [orderer.container, containerLogs(orderer.container)]));
    return leaderFromLogs(logs);
}

// Inicio real de la carga: el primer startedAt que escriben los workers.
async function waitForFirstOperation(runDirectory, caliperExit, timeoutMs = 120_000) {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
        if (caliperExit.done) throw new Error('Caliper finished before the first operation was recorded');
        const starts = [];
        for (const name of fs.readdirSync(runDirectory)) {
            if (!/^raw-worker-\d+\.jsonl$/u.test(name)) continue;
            const contents = fs.readFileSync(path.join(runDirectory, name), 'utf8');
            for (const line of contents.split(/\r?\n/u)) {
                if (!line.trim()) continue;
                try {
                    starts.push(Date.parse(JSON.parse(line).startedAt));
                } catch {
                    // Linea a medio escribir: se relee en la proxima vuelta.
                }
            }
        }
        if (starts.length > 0) return Math.min(...starts);
        await timers.setTimeout(100);
    }
    throw new Error('no operation was recorded within 120 s of launching Caliper');
}

async function sleepUntil(instantMs) {
    const remaining = instantMs - Date.now();
    if (remaining > 0) await timers.setTimeout(remaining);
}

// Flujo regulatorio de Peer-1c: ANMAT pone en cuarentena unidades filler a 1
// operacion/s. Cada cuarentena exige el endoso de ANMAT (su marcador de
// participacion) y el del laboratorio custodio. Es una desviacion declarada
// de la carga: la carga mixta no contiene eventos regulatorios.
function probeInvocation(unit) {
    const registration = unit.preparation[0];
    return normalizeInvocation({
        operation: 'Quarantine',
        invokerMspId: 'AnmatMSP',
        request: {gtin: registration.request.gtin, numeroSerie: registration.request.numeroSerie, motivo: PROBE_REASON},
    }, ['AnmatMSP', registration.invokerMspId], {allowPreparation: true});
}

async function runProbe({pool, units, workerIndex, startMs, durationSeconds, partPath}) {
    const records = [];
    const pending = [];
    for (let ordinal = 0; ordinal < units.length; ordinal += 1) {
        const scheduled = startMs + ordinal * (1000 / PROBE_RATE);
        if (scheduled >= startMs + durationSeconds * 1000) break;
        await sleepUntil(scheduled);
        const {sequence, invocation} = units[ordinal];
        pending.push((async () => {
            const started = Date.now();
            let status = 'success';
            let transactionId;
            let errorCode;
            try {
                transactionId = (await pool.invoke(invocation)).transactionId;
            } catch (error) {
                status = 'failed';
                transactionId = error.transactionId || undefined;
                const envelope = extractContractError(error);
                errorCode = envelope?.code ?? `${error.phase ?? 'gateway'}:${error.code ?? 'UNAVAILABLE'}`;
            }
            const ended = Date.now();
            const record = {
                workerIndex,
                ordinal,
                datasetSequence: sequence,
                type: 'regulatory-event',
                outcome: status === 'success' ? 'success' : 'unexpected-failure',
                retryExhausted: false,
                startedAt: new Date(started).toISOString(),
                endedAt: new Date(ended).toISOString(),
                latencyMs: ended - started,
                transactions: [{
                    function: 'Quarantine', transactionId,
                    startedAt: new Date(started).toISOString(), endedAt: new Date(ended).toISOString(),
                    latencyMs: ended - started, status, retry: false, errorCode,
                }],
            };
            records.push(record);
            fs.appendFileSync(partPath, `${JSON.stringify(record)}\n`, {mode: 0o600});
        })());
    }
    await Promise.all(pending);
    return records.sort((left, right) => left.ordinal - right.ordinal);
}

function killContainers(containers) {
    const at = new Date().toISOString();
    docker(['kill', ...containers]);
    return {action: 'kill', containers, issuedAt: at, completedAt: new Date().toISOString()};
}

function startContainers(containers) {
    const at = new Date().toISOString();
    docker(['start', ...containers]);
    return {action: 'start', containers, issuedAt: at, completedAt: new Date().toISOString()};
}

// Al terminar, la red vuelve a quedar completa y sana aunque la ronda haya
// fallado: el proximo intento restaura el snapshot sobre una red levantada.
function ensureNetworkRunning(repoRoot) {
    docker(['compose', '-f', path.join(repoRoot, 'network', 'compose.yaml'), 'up', '--detach', '--wait', '--wait-timeout', '180']);
}

function writeLogs(runDirectory, containers, fromMs, toMs) {
    const directory = path.join(runDirectory, 'logs');
    fs.mkdirSync(directory, {recursive: true});
    const since = new Date(fromMs - LOG_MARGIN_MS).toISOString();
    const until = new Date(toMs + LOG_MARGIN_MS).toISOString();
    const events = {};
    for (const container of containers) {
        const logs = containerLogs(container, {since, until});
        fs.writeFileSync(path.join(directory, `${container}.log`), logs, {mode: 0o600});
        const pattern = container.startsWith('orderer.') ? RAFT_EVENT : PEER_EVENT;
        events[container] = eventLines(logs, pattern, fromMs - LOG_MARGIN_MS, toMs + LOG_MARGIN_MS);
    }
    return events;
}

function runLaunch(command, args, options) {
    const child = spawn(command, args, {cwd: options.cwd, stdio: 'inherit', env: process.env});
    const state = {done: false, code: undefined, child};
    const finished = new Promise((resolve, reject) => {
        child.on('error', reject);
        child.on('exit', (code) => {
            state.done = true;
            state.code = code;
            resolve(code);
        });
    });
    return {state, finished};
}

function availabilitySummary(summary, operations, windows, definition) {
    const preFault = operations.filter((operation) => Date.parse(operation.startedAt) < Date.parse(windows.injectedAt));
    const failedBeforeFault = preFault.filter((operation) => operation.outcome !== 'success');
    const byClass = {};
    for (const operation of operations) {
        const name = operation.availabilityClass ?? 'all';
        const entry = byClass[name] ?? {operations: 0, successes: 0};
        entry.operations += 1;
        if (operation.outcome === 'success') entry.successes += 1;
        byClass[name] = entry;
    }
    // En una ronda de disponibilidad los fallos durante la falla son lo que se
    // mide; sólo invalida la ronda una carga que ya fallaba antes de inyectarla.
    const discardReason = operations.length === 0 ? 'la ronda no produjo operaciones'
        : failedBeforeFault.length > 0
            ? `${failedBeforeFault.length} operaciones fallaron antes de inyectar la falla: el estado previo no era estable`
            : undefined;
    return {
        ...summary,
        scenario: definition.name,
        discardReason,
        availability: {
            target: definition.target,
            preFaultFailures: failedBeforeFault.length,
            operationsByClass: byClass,
        },
    };
}

async function main() {
    const options = parseArguments(process.argv.slice(2));
    const definition = scenarioDefinition(options.scenario);
    const windowsPlan = schedule(definition);
    const profile = availabilityProfile(options, windowsPlan.durationSeconds);
    const repoRoot = path.resolve(__dirname, '..', '..', '..');
    const datasetDirectory = path.resolve(repoRoot, options.datasetDirectory ?? path.join('build', 'dataset'));
    const sourceTruth = readSourceTruth(repoRoot);
    const bundle = loadDatasetBundle(datasetDirectory);
    const plan = buildAvailabilityPlan(bundle.dataset, profile, bundle.manifest.seed, {
        buildPlan, buildProfile, candidatePools, MIX_PATTERN, plannedCalls,
    });
    const snapshot = readSnapshot(repoRoot, options.snapshotDirectory);
    assertSnapshotCompatibility(repoRoot, snapshot);
    if (snapshot.manifest.dataset.sha256 !== bundle.manifest.dataset.sha256) throw new Error('snapshot and round dataset hashes differ');

    const unitBySequence = new Map(bundle.dataset.units.map((unit) => [unit.sequence, unit]));
    const probeUnits = definition.probe
        ? probeSequences(fillerSequences(snapshot), snapshot.manifest.smokeSequence,
            windowsPlan.durationSeconds * PROBE_RATE)
            .map((sequence) => ({sequence, invocation: probeInvocation(unitBySequence.get(sequence))}))
        : [];

    const credentials = resolveOrganizationCredentials(repoRoot);
    const runtime = inspectOrganizationRuntime(repoRoot, credentials);
    const networkConfig = withBoundedReconnect(buildMultiOrganizationNetworkConfig({
        organizations: credentials, endpoints: runtime.endpoints, sourceTruth, versions: runtime, caliperVersion: CALIPER_VERSION,
    }));
    const runDirectory = createRunDirectory(repoRoot, options.runToken, options.outputRoot);
    const paths = {
        networkConfigPath: path.join(runDirectory, 'network-config.json'),
        benchmarkConfigPath: path.join(runDirectory, 'benchmark-config.json'),
        workPlanPath: path.join(runDirectory, 'work-plan.json'),
        reportPath: path.join(runDirectory, 'report.html'),
        metadataPath: path.join(runDirectory, 'metadata.json'),
        probePath: path.join(runDirectory, 'probe.jsonl'),
        partPaths: Array.from({length: profile.workers}, (_, index) => path.join(runDirectory, `raw-worker-${index}.jsonl`)),
    };
    writeJSONAtomic(paths.networkConfigPath, networkConfig);
    writeJSONAtomic(paths.workPlanPath, plan);
    writeJSONAtomic(paths.benchmarkConfigPath, buildBenchmarkConfig(repoRoot, profile, paths));

    const pool = new GatewayPool(networkConfig);
    const faultEvents = {scenario: definition.name, simulation: SIMULATION, actions: []};
    let killed = [];
    let launch;
    try {
        process.stdout.write(`Discovery ready: ${JSON.stringify(await waitForDiscovery(pool, credentials))}\n`);
        const preconditions = readPreconditions(snapshot);
        const sequences = [...new Set([
            ...sequencesForPlan(plan, snapshot.manifest.smokeSequence),
            ...probeUnits.map((unit) => unit.sequence),
        ])].sort((left, right) => left - right);
        const verified = await verifyPlanPreconditions(pool, bundle.dataset, preconditions, sequences);
        writeJSONAtomic(path.join(runDirectory, 'snapshot-context.json'), {
            snapshotId: snapshot.manifest.snapshotId,
            manifestSHA256: sha256File(snapshot.manifestPath),
            verifiedPreconditions: verified,
        });

        const containers = definition.kind === 'orderer'
            ? orderersToStop(currentLeader(), definition.orderers).map((orderer) => orderer.container)
            : [PEERS[definition.mspId]];
        faultEvents.leaderBefore = definition.kind === 'orderer' ? currentLeader() : undefined;
        faultEvents.containers = containers;

        const repositoryCommit = spawnSync('git', ['rev-parse', 'HEAD'], {cwd: repoRoot, encoding: 'utf8'}).stdout.trim();
        const caliper = path.join(__dirname, '..', 'node_modules', '.bin', 'caliper');
        launch = runLaunch(caliper, [
            'launch', 'manager',
            '--caliper-workspace', repoRoot,
            '--caliper-benchconfig', paths.benchmarkConfigPath,
            '--caliper-networkconfig', paths.networkConfigPath,
            '--caliper-flow-only-test',
            '--caliper-report-path', paths.reportPath,
        ], {cwd: repoRoot});

        const firstOperation = await waitForFirstOperation(runDirectory, launch.state);
        const probe = definition.probe ? runProbe({
            pool, units: probeUnits, workerIndex: profile.workers, startMs: firstOperation,
            durationSeconds: windowsPlan.durationSeconds, partPath: paths.probePath,
        }) : Promise.resolve([]);

        await sleepUntil(firstOperation + windowsPlan.injectAfter * 1000);
        const kill = killContainers(containers);
        killed = containers;
        faultEvents.actions.push(kill);
        process.stdout.write(`Fault injected: docker kill ${containers.join(' ')} at ${kill.issuedAt}\n`);
        let recovery;
        if (windowsPlan.recoverAfter !== undefined) {
            await sleepUntil(firstOperation + windowsPlan.recoverAfter * 1000);
            recovery = startContainers(containers);
            killed = [];
            faultEvents.actions.push(recovery);
            process.stdout.write(`Fault recovered: docker start ${containers.join(' ')} at ${recovery.issuedAt}\n`);
        }

        const caliperCode = await launch.finished;
        const probeRecords = await probe;
        if (killed.length > 0) {
            faultEvents.actions.push({...startContainers(killed), outsideMeasurement: true});
            killed = [];
        }
        ensureNetworkRunning(repoRoot);
        if (definition.kind === 'orderer') faultEvents.leaderAfter = currentLeader();

        const aggregated = aggregateResults(runDirectory, profile);
        const summaryWindow = {startedAt: aggregated.summary.startedAt, endedAt: aggregated.summary.endedAt};
        const windows = faultWindows({
            ...summaryWindow,
            injectedAt: kill.issuedAt,
            recoveredAt: recovery?.issuedAt,
            durationSeconds: profile.durationSeconds,
        });
        const probeByKey = new Map(probeUnits.map((unit, ordinal) => [`${profile.workers}:${ordinal}`, [unit.invocation]]));
        const operations = definition.kind === 'peer'
            ? classifyOperations([...aggregated.operations, ...probeRecords], plan, definition.mspId, probeByKey)
            : aggregated.operations;
        const summary = availabilitySummary(aggregated.summary, operations, windows, definition);
        if (caliperCode !== 0 && !summary.discardReason) summary.discardReason = `Caliper terminó con código ${caliperCode}`;
        summary.availability.probeOperations = probeRecords.length;
        writeJSONAtomic(path.join(runDirectory, 'raw.json'), operations);
        writeJSONAtomic(path.join(runDirectory, 'summary.json'), summary);

        const logContainers = [...new Set([
            ...ORDERERS.map((orderer) => orderer.container),
            ...(definition.kind === 'peer' ? [PEERS[definition.mspId]] : []),
            PEERS.LabMSP,
        ])];
        faultEvents.logEvents = writeLogs(runDirectory, logContainers, Date.parse(summaryWindow.startedAt), Date.parse(summaryWindow.endedAt));
        faultEvents.windows = windows;
        writeJSONAtomic(path.join(runDirectory, 'fault-events.json'), faultEvents);

        const metadata = buildRoundMetadata({
            profile: {...profile, scenario: definition.name},
            repositoryCommit,
            startedAt: summaryWindow.startedAt,
            endedAt: summaryWindow.endedAt,
            dataset: {
                seed: bundle.manifest.seed, sha256: bundle.manifest.dataset.sha256,
                manifestSHA256: bundle.manifestSHA256, units: bundle.manifest.dataset.units,
            },
            host: buildHostMetadata(),
            environment: {
                docker: runtime.docker, dockerCompose: runtime.dockerCompose, contractVersion: sourceTruth.contractVersion,
                packageID: sourceTruth.chaincode.packageID, fabric: runtime.fabric, fabricCA: runtime.fabricCA, caliper: CALIPER_VERSION,
            },
        }, summary);
        metadata.faultInjection = {target: `${definition.target} (${containers.join(', ')})`, ...windows};
        metadata.notes = [
            `Escenario de disponibilidad ${definition.name}: carga mixta de referencia (20 operaciones conceptuales/s, ciclo 2/11/2/5) extendida a 180 s.`,
            SIMULATION,
            definition.probe ? `Desviacion declarada: sonda regulatoria de ${PROBE_RATE} cuarentena/s de ANMAT sobre unidades filler, fuera de la carga mixta.` : undefined,
            'No se incorpora a las estadisticas de rendimiento estable.',
        ].filter(Boolean).join(' ');
        writeJSONAtomic(paths.metadataPath, metadata);
        const validation = spawnSync('go', ['run', './cmd/runmeta', paths.metadataPath], {
            cwd: path.join(repoRoot, 'client'), stdio: 'inherit',
        });
        if (validation.status !== 0) throw new Error('availability metadata does not validate');
        if (summary.discardReason) throw new Error(summary.discardReason);
        process.stdout.write(`Fabric availability round completed: ${runDirectory}\n`);
    } finally {
        pool.close();
        if (launch && !launch.state.done) {
            launch.state.child.kill('SIGTERM');
            await launch.finished.catch(() => {});
        }
        if (killed.length > 0) {
            startContainers(killed);
            ensureNetworkRunning(repoRoot);
        }
    }
}

// Unidades filler segun el plan logico del snapshot: registradas, en custodia
// del laboratorio y fuera de todos los planes de medicion.
function fillerSequences(snapshot) {
    const plan = JSON.parse(fs.readFileSync(path.join(snapshot.directory, snapshot.manifest.files.logicalPlan.name), 'utf8'));
    return plan.units.filter((unit) => unit.role === 'filler').map((unit) => unit.datasetSequence)
        .sort((left, right) => left - right);
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write(`Fabric availability round failed: ${error.message}\n`);
        process.exitCode = 1;
    });
}

module.exports = {availabilityProfile, availabilitySummary, fillerSequences, parseArguments, probeInvocation};
