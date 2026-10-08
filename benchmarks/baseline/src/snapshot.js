'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const timers = require('node:timers/promises');

const {writeJSONAtomic} = require('../../caliper/src/metadata');
const {loadDatasetBundle, readSourceTruth, sha256File} = require('../../caliper/src/sources');
const {resolveCredentials} = require('./credentials');
const {BaselineClient} = require('./http-client');
const {createOutputDirectory, repoRoot} = require('./paths');
const {
    benchmarkEnvironment, compose, dumpDatabase, ensureFabricStopped, inspectRuntime,
    queryDatabase, repositoryCommit, restoreDatabase, writePrivateFile,
} = require('./runtime');
const {buildGoldenSnapshotPlan, registrationForUnit} = require('./snapshot-plan');

const SNAPSHOT_SCHEMA_VERSION = '1.0.0';
const CANONICAL_DATASET_SHA256 = 'de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d';
const CONTRACT_VERSION = '2.11.2';
const PACKAGE_ID = 'snt_1.0:a4d9315a304ac10b467c096f37f4960f86e4ce2d828310aed58d5ad34082d9a9';
const STABLE_UNIT_FIELDS = ['gtin', 'numeroSerie', 'lote', 'fechaVencimiento', 'custodioActual', 'estado'];

function sha256(value) {
    return crypto.createHash('sha256').update(value).digest('hex');
}

function stableUnit(value) {
    return Object.fromEntries(STABLE_UNIT_FIELDS.map((field) => [field, value[field]]));
}

function initialUnit(unit) {
    const registration = registrationForUnit(unit);
    return {
        gtin: registration.request.gtin,
        numeroSerie: registration.request.numeroSerie,
        lote: registration.request.lote,
        fechaVencimiento: registration.request.fechaVencimiento,
        custodioActual: unit.initialCustodian,
        estado: unit.initialState,
    };
}

function equalUnit(left, right) {
    return STABLE_UNIT_FIELDS.every((field) => left?.[field] === right?.[field]);
}

async function parallel(items, concurrency, operation) {
    let next = 0;
    let firstError;
    await Promise.all(Array.from({length: Math.min(concurrency, Math.max(1, items.length))}, async () => {
        while (!firstError) {
            const index = next;
            next += 1;
            if (index >= items.length) return;
            try {
                await operation(items[index], index);
            } catch (error) {
                firstError ??= error;
            }
        }
    }));
    if (firstError) throw firstError;
}

function requiredMspIds(plan) {
    return [...new Set(plan.preparations.flatMap((entry) => entry.invocations.map((invocation) => invocation.invokerMspId)))].sort();
}

async function prepareSnapshot(client, plan, expectedBySequence, concurrency, options = {}) {
    const sleep = options.sleep ?? timers.setTimeout;
    const retryDelayMs = options.retryDelayMs ?? 1100;
    let completed = 0;
    let receiveRetries = 0;
    await parallel(plan.preparations, concurrency, async (preparation) => {
        let expected = expectedBySequence.get(preparation.datasetSequence);
        for (const invocation of preparation.invocations) {
            if (invocation.operation === 'RegisterUnit') continue;
            let result = await client.invoke(invocation);
            if (!result.ok && invocation.operation === 'ReceiveTransfer'
                && result.transaction.errorCode === 'INTERNAL_ERROR') {
                receiveRetries += 1;
                await sleep(retryDelayMs);
                result = await client.invoke(invocation);
            }
            if (!result.ok) {
                const classification = result.transaction.errorCode
                    ?? result.transaction.transportError
                    ?? result.transaction.responseError
                    ?? result.transaction.httpStatus;
                const envelope = result.errorEnvelope ? ` ${JSON.stringify(result.errorEnvelope)}` : '';
                throw new Error(`snapshot preparation ${preparation.datasetSequence}/${invocation.operation} failed with ${classification}${envelope}`);
            }
            expected = stableUnit(result.value);
        }
        expectedBySequence.set(preparation.datasetSequence, expected);
        completed += 1;
        if (completed % 500 === 0 || completed === plan.preparations.length) {
            process.stdout.write(`Prepared ${completed}/${plan.preparations.length} baseline recipes\n`);
        }
    });
    return {recipes: completed, receiveRetries};
}

async function verifyAllUnits(client, dataset, expectedBySequence, concurrency) {
    let present = 0;
    let absent = 0;
    await parallel(dataset.units, concurrency, async (unit) => {
        const registration = registrationForUnit(unit);
        const result = await client.invoke({
            operation: 'ReadUnit', invokerMspId: registration.invokerMspId,
            request: {gtin: registration.request.gtin, numeroSerie: registration.request.numeroSerie},
        });
        const expected = expectedBySequence.get(unit.sequence);
        if (expected === null) {
            if (result.ok || result.transaction.errorCode !== 'UNIT_NOT_FOUND') {
                throw new Error(`dataset sequence ${unit.sequence} should be absent from the snapshot`);
            }
            absent += 1;
            return;
        }
        if (!result.ok || !equalUnit(result.value, expected)) {
            throw new Error(`dataset sequence ${unit.sequence} does not match its snapshot precondition`);
        }
        present += 1;
    });
    return {present, absent};
}

function databaseCounts(repoRootPath, environment) {
    const sql = `SELECT json_build_object(
      'organizations',(SELECT count(*) FROM public.organizations),
      'units',(SELECT count(*) FROM public.medication_units),
      'unitEvents',(SELECT count(*) FROM public.unit_events),
      'transfers',(SELECT count(*) FROM public.transfer_operations),
      'activeTransfers',(SELECT count(*) FROM public.transfer_operations WHERE estado='ACTIVA'),
      'returns',(SELECT count(*) FROM public.return_operations),
      'labInterventions',(SELECT count(*) FROM public.lab_interventions),
      'labInterventionEvents',(SELECT count(*) FROM public.lab_intervention_events)
    )::text;`;
    return JSON.parse(queryDatabase(repoRootPath, environment, sql));
}

function effectiveAPIURL(environment) {
    return `http://127.0.0.1:${environment.SNT_BASELINE_API_PORT}`;
}

function hostUserSpec() {
    if (typeof process.getuid !== 'function' || typeof process.getgid !== 'function') {
        throw new Error('snapshot build requires a POSIX host user to read the private dataset bundle');
    }
    return `${process.getuid()}:${process.getgid()}`;
}

async function buildSnapshot(options = {}) {
    const datasetDirectory = path.resolve(repoRoot, options.datasetDirectory ?? path.join('build', 'dataset'));
    const bundle = loadDatasetBundle(datasetDirectory);
    if (bundle.manifest.dataset.sha256 !== CANONICAL_DATASET_SHA256) {
        throw new Error(`dataset must have canonical SHA-256 ${CANONICAL_DATASET_SHA256}`);
    }
    const sourceTruth = readSourceTruth(repoRoot);
    if (sourceTruth.contractVersion !== CONTRACT_VERSION || sourceTruth.chaincode.packageID !== PACKAGE_ID) {
        throw new Error('canonical contract version or packageID changed; this benchmark must be reviewed');
    }
    const plan = buildGoldenSnapshotPlan(bundle.dataset, bundle.manifest.seed);
    const snapshotDirectory = createOutputDirectory('snapshots', options.snapshotToken);
    const selectionPath = path.join(snapshotDirectory, 'seed-selection.json');
    const planPath = path.join(snapshotDirectory, 'snapshot-plan.json');
    writeJSONAtomic(selectionPath, {
        schemaVersion: '1.0.0', datasetSHA256: bundle.manifest.dataset.sha256,
        excludedSequences: plan.excludedSequences,
    });
    writeJSONAtomic(planPath, plan);

    const environment = benchmarkEnvironment();
    if (!environment.SNT_BASELINE_DB_PASSWORD || !environment.SNT_BASELINE_API_KEYS) {
        throw new Error('SNT_BASELINE_DB_PASSWORD and SNT_BASELINE_API_KEYS are required');
    }
    environment.SNT_BASELINE_DATASET_DIR = datasetDirectory;
    ensureFabricStopped(repoRoot, environment);
    const required = requiredMspIds(plan);
    const credentials = resolveCredentials(repoRoot, required, environment);
    const client = new BaselineClient({baseURL: effectiveAPIURL(environment), credentials});
    const excluded = new Set(plan.excludedSequences);
    const expectedBySequence = new Map(bundle.dataset.units.map((unit) => [
        unit.sequence, excluded.has(unit.sequence) ? null : initialUnit(unit),
    ]));
    const concurrency = Number(environment.SNT_BASELINE_PREPARATION_CONCURRENCY ?? 8);
    if (!Number.isInteger(concurrency) || concurrency < 1) throw new Error('preparation concurrency must be a positive integer');

    compose(repoRoot, ['--profile', 'seed', 'down', '--volumes', '--remove-orphans'], {env: environment, allowFailure: true});
    try {
        compose(repoRoot, ['up', '-d', '--build', '--wait', 'postgres', 'api'], {env: environment, inherit: true});
        compose(repoRoot, [
            '--profile', 'seed', 'run', '--rm',
            '--user', hostUserSpec(),
            '-v', `${selectionPath}:/selection.json:ro`,
            'seed', '--selection-file', '/selection.json',
        ], {env: environment, inherit: true});
        const preparation = await prepareSnapshot(client, plan, expectedBySequence, concurrency);
        const verified = await verifyAllUnits(client, bundle.dataset, expectedBySequence, concurrency);
        if (verified.present !== plan.counts.total - plan.counts.unregistered || verified.absent !== plan.counts.unregistered) {
            throw new Error(`snapshot presence counts are inconsistent: ${JSON.stringify(verified)}`);
        }
        const counts = databaseCounts(repoRoot, environment);
        if (Number(counts.organizations) !== 7 || Number(counts.units) !== verified.present
            || Number(counts.labInterventions) !== 0 || Number(counts.labInterventionEvents) !== 0) {
            throw new Error(`snapshot database counts are inconsistent: ${JSON.stringify(counts)}`);
        }
        const preconditions = bundle.dataset.units.map((unit) => ({sequence: unit.sequence, unit: expectedBySequence.get(unit.sequence)}));
        const preconditionsPath = path.join(snapshotDirectory, 'preconditions.json');
        writeJSONAtomic(preconditionsPath, preconditions);
        compose(repoRoot, ['stop', 'api'], {env: environment});
        const dump = dumpDatabase(repoRoot, environment);
        const dumpPath = path.join(snapshotDirectory, 'baseline.dump');
        writePrivateFile(dumpPath, dump);
        const runtime = inspectRuntime(repoRoot, environment);
        const manifest = {
            schemaVersion: SNAPSHOT_SCHEMA_VERSION,
            snapshotId: sha256(Buffer.concat([dump, Buffer.from(bundle.manifest.dataset.sha256)])),
            dataset: {
                seed: bundle.manifest.seed,
                sha256: bundle.manifest.dataset.sha256,
                manifestSHA256: bundle.manifestSHA256,
                units: bundle.manifest.dataset.units,
            },
            counts: {...plan.counts, database: counts},
            preparation,
            smokeSequence: plan.smokeSequence,
            artifacts: {
                repositoryCommit: repositoryCommit(repoRoot),
                baselineCommit: repositoryCommit(repoRoot),
                baselineImage: runtime.baselineImage,
                postgres: runtime.postgres,
                contractVersion: sourceTruth.contractVersion,
                packageID: sourceTruth.chaincode.packageID,
                fabric: runtime.fabric,
            },
            files: {
                dump: {name: path.basename(dumpPath), sha256: sha256File(dumpPath), bytes: dump.length},
                preconditions: {name: path.basename(preconditionsPath), sha256: sha256File(preconditionsPath)},
                selection: {name: path.basename(selectionPath), sha256: sha256File(selectionPath)},
                plan: {name: path.basename(planPath), sha256: sha256File(planPath)},
            },
            environment: {docker: runtime.docker, dockerCompose: runtime.dockerCompose},
        };
        writeJSONAtomic(path.join(snapshotDirectory, 'manifest.json'), manifest);
        process.stdout.write(`Baseline snapshot completed: ${snapshotDirectory}\n`);
        return {snapshotDirectory, manifest};
    } catch (error) {
        const logs = compose(repoRoot, ['logs', '--no-color', 'api', 'postgres'], {
            env: environment, allowFailure: true,
        });
        writePrivateFile(
            path.join(snapshotDirectory, 'failure.log'),
            `${logs.stdout ?? ''}${logs.stderr ?? ''}`,
        );
        throw error;
    } finally {
        compose(repoRoot, ['--profile', 'seed', 'down', '--volumes', '--remove-orphans'], {env: environment, allowFailure: true});
    }
}

function readSnapshot(snapshotDirectory) {
    const directory = path.resolve(snapshotDirectory);
    const manifest = JSON.parse(fs.readFileSync(path.join(directory, 'manifest.json'), 'utf8'));
    if (manifest.schemaVersion !== SNAPSHOT_SCHEMA_VERSION) throw new Error('unsupported snapshot manifest version');
    for (const file of Object.values(manifest.files)) {
        const filePath = path.join(directory, file.name);
        if (sha256File(filePath) !== file.sha256) throw new Error(`snapshot file hash mismatch: ${file.name}`);
    }
    return {directory, manifest};
}

function restoreSnapshot(snapshotDirectory, options = {}) {
    const snapshot = readSnapshot(path.resolve(repoRoot, snapshotDirectory));
    const environment = benchmarkEnvironment(options.environment);
    if (!environment.SNT_BASELINE_DB_PASSWORD || !environment.SNT_BASELINE_API_KEYS) {
        throw new Error('SNT_BASELINE_DB_PASSWORD and SNT_BASELINE_API_KEYS are required');
    }
    ensureFabricStopped(repoRoot, environment);
    const sourceTruth = readSourceTruth(repoRoot);
    if (sourceTruth.contractVersion !== snapshot.manifest.artifacts.contractVersion
        || sourceTruth.chaincode.packageID !== snapshot.manifest.artifacts.packageID
        || repositoryCommit(repoRoot) !== snapshot.manifest.artifacts.baselineCommit) {
        throw new Error('snapshot artifact identifiers do not match the current checkout');
    }
    compose(repoRoot, ['up', '-d', '--wait', 'postgres'], {env: environment});
    const dump = fs.readFileSync(path.join(snapshot.directory, snapshot.manifest.files.dump.name));
    restoreDatabase(repoRoot, environment, dump);
    const runtime = inspectRuntime(repoRoot, environment);
    if (runtime.baselineImage !== snapshot.manifest.artifacts.baselineImage
        || runtime.postgres !== snapshot.manifest.artifacts.postgres) {
        throw new Error('restored baseline runtime differs from the snapshot manifest');
    }
    const counts = databaseCounts(repoRoot, environment);
    for (const key of Object.keys(snapshot.manifest.counts.database)) {
        if (Number(counts[key]) !== Number(snapshot.manifest.counts.database[key])) {
            throw new Error(`restored database count differs for ${key}`);
        }
    }
    return {...snapshot, environment, runtime, apiURL: effectiveAPIURL(environment)};
}

module.exports = {
    CANONICAL_DATASET_SHA256, CONTRACT_VERSION, PACKAGE_ID, SNAPSHOT_SCHEMA_VERSION,
    buildSnapshot, databaseCounts, effectiveAPIURL, equalUnit, initialUnit, readSnapshot,
    restoreSnapshot, stableUnit, verifyAllUnits, hostUserSpec, prepareSnapshot,
};
