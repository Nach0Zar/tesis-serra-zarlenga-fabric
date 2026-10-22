#!/usr/bin/env node
'use strict';

// Reconstruye la evidencia de una ronda de disponibilidad cuya carga termino
// pero que abortó antes de escribirla, como la repeticion 2 de Raft-1 de
// eval4-citable: el orderer detenido no volvio a arrancar y el runner de
// f471793 fallaba antes de guardar el resumen (#153). Usa sólo lo que la
// ronda conservó: los crudos de los workers, el plan, round.log (instantes de
// la falla y error de restauracion), sut.log (logs de la red) y la metadata
// del smoke del mismo intento (host y entorno). No vuelve a ejecutar nada ni
// imputa lo que esos archivos no registran; reconstruction.json declara la
// herramienta, el commit de ejecucion y los hashes de cada insumo.

const fs = require('node:fs');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const {sha256File} = require('./artifacts');
const {ORDERERS, SIMULATION, leaderFromLogs, scenarioDefinition, schedule} = require('./availability');
const {availabilityProfile, finalizeRound} = require('./run-availability');
const {loadDatasetBundle} = require('./sources');

const FABRIC_STAMP = /(\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}:\d{2}(?:\.\d+)?) UTC/u;
const ANSI = /\u001b\[[0-9;]*m/gu;

function parseArguments(arguments_) {
    const values = {};
    for (let index = 0; index < arguments_.length; index += 2) {
        const name = arguments_[index];
        const value = arguments_[index + 1];
        if (!name?.startsWith('--') || value === undefined || value.startsWith('--')) {
            throw new Error('arguments must be supplied as --name value pairs');
        }
        values[name.slice(2)] = value;
    }
    const supported = new Set(['attempt-dir', 'scenario', 'phase', 'repetition', 'execution-commit', 'dataset-dir']);
    for (const key of Object.keys(values)) if (!supported.has(key)) throw new Error(`unsupported argument --${key}`);
    for (const required of ['attempt-dir', 'scenario', 'phase', 'repetition', 'execution-commit']) {
        if (!values[required]) throw new Error(`--${required} is required`);
    }
    return {
        attemptDirectory: values['attempt-dir'],
        scenario: values.scenario,
        phase: values.phase,
        repetition: values.repetition,
        executionCommit: values['execution-commit'],
        datasetDirectory: values['dataset-dir'],
    };
}

// Instantes de la falla tal como los imprimió el runner en round.log.
function faultActions(roundLog) {
    const actions = [];
    for (const [pattern, action] of [
        [/^Fault injected: docker kill (.+) at (\S+)$/mu, 'kill'],
        [/^Fault recovered: docker start (.+) at (\S+)$/mu, 'start'],
    ]) {
        const match = pattern.exec(roundLog);
        if (match) actions.push({action, containers: match[1].split(' '), issuedAt: match[2]});
    }
    if (!actions.some((entry) => entry.action === 'kill')) throw new Error('round.log does not record the fault injection');
    return actions;
}

// Ultima linea de error del runner: el motivo por el que abortó.
function abortReason(roundLog) {
    const lines = roundLog.split(/\r?\n/u).map((line) => line.trim()).filter(Boolean);
    const index = lines.findIndex((line) => line.startsWith('Fabric availability round failed:'));
    if (index < 0) throw new Error('round.log does not record a failed round');
    return lines.at(-1);
}

// Logs por contenedor a partir de `docker compose logs` (sut.log). Compose no
// agrega marcas de tiempo propias: cada linea toma la del log de Fabric, y las
// lineas sin marca (por ejemplo, la continuacion de un panic) heredan la
// anterior. Se reescriben con la marca ISO al inicio, como `docker logs
// --timestamps`, para filtrarlas por ventana con el mismo criterio.
function composeLogsByContainer(sutLog) {
    const byContainer = new Map();
    const lastStamp = new Map();
    for (const raw of sutLog.split(/\r?\n/u)) {
        const match = /^(\S+)\s+\| ?(.*)$/u.exec(raw);
        if (!match) continue;
        const [, container, rest] = match;
        const line = rest.replace(ANSI, '');
        const stamp = FABRIC_STAMP.exec(line);
        const at = stamp ? `${stamp[1]}T${stamp[2]}Z` : lastStamp.get(container);
        if (stamp) lastStamp.set(container, at);
        if (!at) continue;
        if (!byContainer.has(container)) byContainer.set(container, []);
        byContainer.get(container).push({at: Date.parse(at), text: `${new Date(Date.parse(at)).toISOString()} ${line}`});
    }
    return (container, {since, until} = {}) => {
        const from = since ? Date.parse(since) : -Infinity;
        const to = until ? Date.parse(until) : Infinity;
        return (byContainer.get(container) ?? []).filter((entry) => entry.at >= from && entry.at <= to)
            .map((entry) => entry.text).join('\n');
    };
}

function reconstruct(repoRoot, options) {
    const attemptDirectory = path.resolve(repoRoot, options.attemptDirectory);
    const runDirectory = path.join(attemptDirectory, 'round');
    for (const name of ['raw.json', 'summary.json', 'metadata.json', 'fault-events.json', 'reconstruction.json']) {
        if (fs.existsSync(path.join(runDirectory, name))) throw new Error(`${name} already exists: the round is not missing its evidence`);
    }
    const definition = scenarioDefinition(options.scenario);
    const windowsPlan = schedule(definition);
    const profile = availabilityProfile(options, windowsPlan.durationSeconds);
    const roundLogPath = path.join(attemptDirectory, 'round.log');
    const sutLogPath = path.join(attemptDirectory, 'sut.log');
    const smokeMetadataPath = path.join(attemptDirectory, 'smoke', 'metadata.json');
    const workPlanPath = path.join(runDirectory, 'work-plan.json');
    const partPaths = fs.readdirSync(runDirectory).filter((name) => /^raw-worker-\d+\.jsonl$/u.test(name)).sort()
        .map((name) => path.join(runDirectory, name));
    if (partPaths.length !== profile.workers) throw new Error(`expected ${profile.workers} worker raw files, found ${partPaths.length}`);
    const inputs = [roundLogPath, sutLogPath, smokeMetadataPath, workPlanPath,
        path.join(runDirectory, 'snapshot-context.json'), ...partPaths]
        .map((file) => ({path: path.relative(attemptDirectory, file).split(path.sep).join('/'), sha256: sha256File(file)}));

    const roundLog = fs.readFileSync(roundLogPath, 'utf8');
    const actions = faultActions(roundLog);
    const kill = actions.find((entry) => entry.action === 'kill');
    const recovery = actions.find((entry) => entry.action === 'start');
    const containers = kill.containers;
    const logsFor = composeLogsByContainer(fs.readFileSync(sutLogPath, 'utf8'));
    const smoke = JSON.parse(fs.readFileSync(smokeMetadataPath, 'utf8'));
    if (smoke.repositoryCommit !== options.executionCommit) {
        throw new Error(`smoke metadata commit ${smoke.repositoryCommit} differs from --execution-commit`);
    }
    const bundle = loadDatasetBundle(path.resolve(repoRoot, options.datasetDirectory ?? path.join('build', 'dataset')));
    if (bundle.manifest.dataset.sha256 !== smoke.dataset.sha256) throw new Error('dataset differs from the one the round used');

    const restoration = {status: 'failed', reason: abortReason(roundLog)};
    const reconstructedAt = new Date().toISOString();
    const toolCommit = spawnSync('git', ['rev-parse', 'HEAD'], {cwd: repoRoot, encoding: 'utf8'}).stdout.trim();
    const faultEvents = {
        scenario: definition.name,
        simulation: SIMULATION,
        actions: actions.map((entry) => ({...entry, reconstructed: true})),
        containers,
        reconstructed: {reconstructedAt, toolCommit, record: 'reconstruction.json'},
    };
    if (definition.kind === 'orderer') {
        // El runner elige el lider como primer orderer a detener.
        faultEvents.leaderBefore = ORDERERS.find((orderer) => orderer.container === containers[0]);
        const until = new Date(Date.parse(kill.issuedAt) + (windowsPlan.durationSeconds - windowsPlan.injectAfter) * 1000).toISOString();
        faultEvents.leaderAfter = leaderFromLogs(Object.fromEntries(ORDERERS.map((orderer) => [
            orderer.container, logsFor(orderer.container, {until}).replace(/^\S+ /gmu, ''),
        ])));
    }
    faultEvents.restoration = restoration;

    const plan = JSON.parse(fs.readFileSync(workPlanPath, 'utf8'));
    const caliperFinished = /Benchmark successfully finished!/u.test(roundLog);
    const {summary} = finalizeRound({
        runDirectory, profile, definition, plan, containers, kill, recovery, faultEvents, restoration,
        caliperCode: caliperFinished ? 0 : 1,
        repositoryCommit: options.executionCommit,
        bundle,
        host: smoke.host,
        environment: smoke.environment,
        logsFor,
        notes: [`Evidencia reconstruida el ${reconstructedAt} con reconstruct-availability-round.js (commit ${toolCommit}) `
            + `a partir de los crudos, round.log y sut.log conservados; la ronda se ejecuto en ${options.executionCommit}.`],
    });
    const record = {
        tool: 'benchmarks/caliper/src/reconstruct-availability-round.js',
        toolCommit,
        reconstructedAt,
        executionCommit: options.executionCommit,
        reason: 'la ronda aborto al exigir que el componente volviera sano, antes de escribir su evidencia',
        restoration,
        inputs,
        derivedFrom: {
            faultInstants: 'round.log (Fault injected / Fault recovered)',
            logs: 'sut.log (docker compose logs), con la marca de tiempo del propio log de Fabric',
            hostAndEnvironment: 'smoke/metadata.json del mismo intento',
        },
        notRecovered: [
            'completedAt de las acciones docker kill y docker start',
            'instante en que se comprobo la restauracion',
            'marcas de tiempo de Docker por linea de log',
        ],
        outputs: ['raw.json', 'summary.json', 'fault-events.json', 'metadata.json', 'logs/'],
    };
    fs.writeFileSync(path.join(runDirectory, 'reconstruction.json'), `${JSON.stringify(record, null, 2)}\n`, {mode: 0o600});
    return {summary, runDirectory};
}

if (require.main === module) {
    try {
        const repoRoot = path.resolve(__dirname, '..', '..', '..');
        const result = reconstruct(repoRoot, parseArguments(process.argv.slice(2)));
        process.stdout.write(`Reconstructed availability round: ${result.runDirectory}\n`);
    } catch (error) {
        process.stderr.write(`Availability round reconstruction failed: ${error.message}\n`);
        process.exitCode = 1;
    }
}

module.exports = {abortReason, composeLogsByContainer, faultActions, parseArguments, reconstruct};
