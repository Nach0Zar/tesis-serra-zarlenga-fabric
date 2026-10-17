#!/usr/bin/env node
'use strict';

// Ronda de disponibilidad de la baseline (EVAL-5, #45; protocolo, secciones 3.3,
// 8.2 y 9.5). Simetrica con EVAL-4 (#44): la misma carga mixta de 180 s, el
// mismo plan determinístico, las mismas tres ventanas de 60 s y el mismo nivel
// de simulacion (docker kill de un contenedor en un unico host). La falla se
// inyecta desde este proceso, fuera del ejecutor de carga.

const fs = require('node:fs');
const path = require('node:path');
const timers = require('node:timers/promises');

const {
    FAULT_SECONDS, PRE_FAULT_SECONDS, ROUND_SECONDS, buildAvailabilityPlan, eventLines, faultWindows,
} = require('../../caliper/src/availability');
const {writeJSONAtomic} = require('../../caliper/src/metadata');
const planner = require('../../caliper/src/planner');
const {buildProfile} = require('../../caliper/src/profiles');
const {aggregateResults} = require('../../caliper/src/raw-results');
const {loadDatasetBundle, readSourceTruth} = require('../../caliper/src/sources');
const {pairs} = require('./arguments');
const {resolveCredentials} = require('./credentials');
const {runFixedRate} = require('./execution');
const {BaselineClient} = require('./http-client');
const {buildRoundMetadata} = require('./metadata');
const {createOutputDirectory, repoRoot} = require('./paths');
const {readPreconditions, sequencesForPlan, verifyPreconditions} = require('./preconditions');
const {assertCanonical, executeSmoke, roundContext, validateMetadata} = require('./runner');
const {compose} = require('./runtime');
const {databaseCounts, restoreSnapshot} = require('./snapshot');

const MIXED_RATE = 20;
const LOG_MARGIN_MS = 15_000;

// Los dos escenarios de la seccion 8.2. En la baseline no hay un componente
// redundante: cada uno es un punto unico de falla de la topologia centralizada.
const SCENARIOS = Object.freeze({
    'db-1': {
        service: 'postgres',
        target: 'PostgreSQL de la baseline: unica base de datos del sistema centralizado',
    },
    'api-1': {
        service: 'api',
        target: 'API REST de la baseline: unico punto de entrada del sistema centralizado',
    },
});

const SIMULATION = 'Falla simulada con docker kill sobre un contenedor, en un unico host, con el mismo nivel de '
    + 'simulacion que EVAL-4; el punto unico de falla es una propiedad de la topologia centralizada, no del montaje.';

const SERVICE_EVENT = /error|fatal|panic|refused|terminat|shutting down|shut down|ready to accept|listening|starting|connection|database system/iu;

function scenarioDefinition(name) {
    const definition = SCENARIOS[name];
    if (!definition) throw new Error(`unsupported baseline availability scenario ${name}`);
    return {name, ...definition};
}

function parseArguments(arguments_) {
    const values = pairs(arguments_);
    const supported = new Set(['scenario', 'phase', 'repetition', 'snapshot-dir', 'dataset-dir', 'run-token', 'output-root']);
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
        runToken: values['run-token'],
        outputRoot: values['output-root'],
    };
}

// La misma carga que EVAL-4: la mixta de 20 operaciones conceptuales/s
// extendida a las tres ventanas.
function availabilityProfile(options) {
    const mixed = buildProfile({scenario: 'mixed', phase: options.phase, repetition: options.repetition, rate: String(MIXED_RATE)});
    return Object.freeze({...mixed, durationSeconds: ROUND_SECONDS});
}

// Sólo invalida la ronda una carga que ya fallaba antes de inyectar la falla;
// las operaciones en vuelo al inyectarla fallan por ella y terminan despues.
function availabilitySummary(summary, operations, windows, definition) {
    const failedBeforeFault = operations.filter((operation) => operation.outcome !== 'success'
        && Date.parse(operation.endedAt) < Date.parse(windows.injectedAt));
    const discardReason = operations.length === 0 ? 'la ronda no produjo operaciones'
        : failedBeforeFault.length > 0
            ? `${failedBeforeFault.length} operaciones fallaron antes de inyectar la falla: el estado previo no era estable`
            : undefined;
    return {
        ...summary,
        scenario: definition.name,
        discardReason,
        availability: {target: definition.target, preFaultFailures: failedBeforeFault.length},
    };
}

function serviceAction(environment, action, service) {
    const issuedAt = new Date().toISOString();
    compose(repoRoot, [action, service], {env: environment});
    return {action, service, issuedAt, completedAt: new Date().toISOString()};
}

function serviceLogs(environment, service, fromMs, toMs) {
    return compose(repoRoot, [
        'logs', '--no-color', '--timestamps', '--no-log-prefix',
        '--since', new Date(fromMs - LOG_MARGIN_MS).toISOString(),
        '--until', new Date(toMs + LOG_MARGIN_MS).toISOString(),
        service,
    ], {env: environment, allowFailure: true}).stdout ?? '';
}

async function sleepUntil(instantMs) {
    const remaining = instantMs - Date.now();
    if (remaining > 0) await timers.setTimeout(remaining);
}

async function main() {
    const options = parseArguments(process.argv.slice(2));
    const definition = scenarioDefinition(options.scenario);
    const profile = availabilityProfile(options);
    const datasetDirectory = path.resolve(repoRoot, options.datasetDirectory ?? path.join('build', 'dataset'));
    const bundle = loadDatasetBundle(datasetDirectory);
    const sourceTruth = readSourceTruth(repoRoot);
    assertCanonical(bundle, sourceTruth);
    const plan = buildAvailabilityPlan(bundle.dataset, profile, bundle.manifest.seed, {...planner, buildProfile});
    const restored = restoreSnapshot(options.snapshotDirectory);
    const environment = restored.environment;
    const credentials = resolveCredentials(repoRoot, plan.requiredMspIds, environment);
    const client = new BaselineClient({baseURL: restored.apiURL, credentials});
    const runDirectory = createOutputDirectory('runs', options.runToken, options.outputRoot);
    const context = roundContext({bundle, profile, restored, sourceTruth, credentials});
    const faultEvents = {scenario: definition.name, simulation: SIMULATION, service: definition.service, actions: []};
    let killed = false;
    try {
        writeJSONAtomic(path.join(runDirectory, 'effective-config.json'), {
            apiURL: restored.apiURL, composeProject: environment.COMPOSE_PROJECT_NAME, profile, credentials: context.credentials,
        });
        writeJSONAtomic(path.join(runDirectory, 'work-plan.json'), plan);
        writeJSONAtomic(path.join(runDirectory, 'run-context.json'), context);
        const verified = await verifyPreconditions(
            client, bundle.dataset, readPreconditions(restored), sequencesForPlan(plan, restored.manifest.smokeSequence),
        );
        writeJSONAtomic(path.join(runDirectory, 'snapshot-context.json'), {
            snapshot: context.snapshot,
            referenceArtifacts: context.referenceArtifacts,
            verifiedPreconditions: verified,
            counts: restored.manifest.counts,
        });
        await executeSmoke({bundle, restored, sourceTruth, credentials, outputDirectory: path.join(runDirectory, 'preflight')});

        const partPaths = Array.from({length: profile.workers}, (_, index) => path.join(runDirectory, `raw-worker-${index}.jsonl`));
        const loadStarted = Date.now();
        const load = runFixedRate({client, plan, profile, partPaths});
        await sleepUntil(loadStarted + PRE_FAULT_SECONDS * 1000);
        const kill = serviceAction(environment, 'kill', definition.service);
        killed = true;
        faultEvents.actions.push(kill);
        process.stdout.write(`Fault injected: docker compose kill ${definition.service} at ${kill.issuedAt}\n`);
        await sleepUntil(loadStarted + (PRE_FAULT_SECONDS + FAULT_SECONDS) * 1000);
        const recovery = serviceAction(environment, 'start', definition.service);
        killed = false;
        faultEvents.actions.push(recovery);
        process.stdout.write(`Fault recovered: docker compose start ${definition.service} at ${recovery.issuedAt}\n`);
        const observedWindow = await load;

        const aggregated = aggregateResults(runDirectory, profile, observedWindow);
        const windows = faultWindows({
            startedAt: aggregated.summary.startedAt,
            endedAt: aggregated.summary.endedAt,
            injectedAt: kill.issuedAt,
            recoveredAt: recovery.issuedAt,
            durationSeconds: profile.durationSeconds,
        });
        const summary = availabilitySummary(aggregated.summary, aggregated.operations, windows, definition);
        const expectedOperations = profile.rate * profile.durationSeconds;
        if (!summary.discardReason && aggregated.operations.length !== expectedOperations) {
            summary.discardReason = `la ronda ofrecio ${aggregated.operations.length}/${expectedOperations} operaciones`;
        }
        compose(repoRoot, ['up', '-d', '--wait', 'postgres', 'api'], {env: environment});
        const counts = databaseCounts(repoRoot, environment);
        writeJSONAtomic(path.join(runDirectory, 'post-run-counts.json'), counts);
        if (!summary.discardReason && (Number(counts.labInterventions) !== 0 || Number(counts.labInterventionEvents) !== 0)) {
            summary.discardReason = 'laboratory intervention tables received an unexpected write';
        }
        writeJSONAtomic(path.join(runDirectory, 'summary.json'), summary);

        const logsDirectory = path.join(runDirectory, 'logs');
        fs.mkdirSync(logsDirectory, {recursive: true});
        faultEvents.logEvents = {};
        const from = Date.parse(summary.startedAt);
        const to = Date.parse(summary.endedAt);
        for (const service of ['postgres', 'api']) {
            const logs = serviceLogs(environment, service, from, to);
            fs.writeFileSync(path.join(logsDirectory, `${service}.log`), logs, {mode: 0o600});
            faultEvents.logEvents[service] = eventLines(logs, SERVICE_EVENT, from - LOG_MARGIN_MS, to + LOG_MARGIN_MS);
        }
        faultEvents.windows = windows;
        writeJSONAtomic(path.join(runDirectory, 'fault-events.json'), faultEvents);

        const metadata = buildRoundMetadata({
            ...context,
            profile: {...profile, scenario: definition.name},
            startedAt: summary.startedAt,
            endedAt: summary.endedAt,
        }, summary);
        metadata.faultInjection = {target: `${definition.target} (servicio ${definition.service})`, ...windows};
        metadata.notes = [
            `Escenario de disponibilidad ${definition.name}: carga mixta de referencia (20 operaciones conceptuales/s, ciclo 2/11/2/5) extendida a 180 s, con el mismo plan que EVAL-4.`,
            SIMULATION,
            'No se incorpora a las estadisticas de rendimiento estable.',
        ].join(' ');
        const metadataPath = path.join(runDirectory, 'metadata.json');
        writeJSONAtomic(metadataPath, metadata);
        validateMetadata(metadataPath);
        if (summary.discardReason) throw new Error(summary.discardReason);
        process.stdout.write(`Baseline availability round completed: ${runDirectory}\n`);
    } finally {
        if (killed) compose(repoRoot, ['start', definition.service], {env: environment, allowFailure: true});
        compose(repoRoot, ['down', '--volumes', '--remove-orphans'], {env: environment, allowFailure: true});
    }
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write(`Baseline availability round failed: ${error.message}\n`);
        process.exitCode = 1;
    });
}

module.exports = {SCENARIOS, SIMULATION, availabilityProfile, availabilitySummary, parseArguments, scenarioDefinition};
