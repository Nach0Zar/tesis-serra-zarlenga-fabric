#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const {
    CANONICAL_DATASET_SHA256, SNAPSHOT_SCHEMA_VERSION, connectAllOrganizations, readArtifacts, run, saveVolumes, sha256,
    writeSnapshotManifest,
} = require('./fabric-snapshot');
const {writeJSONAtomic} = require('./metadata');
const {buildHostMetadata} = require('./run-round');
const {
    buildPreparationMetadata, countMarkers, currentHeight, executeRecipes, verifyPreconditions,
} = require('./snapshot-builder');
const {
    buildGoldenSnapshotPlan, expectedPreconditions, expectedPreparationMarkers, snapshotRecipes,
} = require('./snapshot-plan');
const {loadDatasetBundle, readJSON, readSourceTruth, sha256File} = require('./sources');

const CALIPER_VERSION = require('../package.json').devDependencies['@hyperledger/caliper-cli'];
const DEFAULT_CONCURRENCY = 64;

function parseArguments(arguments_) {
    const options = {concurrency: DEFAULT_CONCURRENCY};
    for (let index = 0; index < arguments_.length; index += 2) {
        const [name, value] = [arguments_[index], arguments_[index + 1]];
        if (value === undefined || value.startsWith('--')) throw new Error(`${name} requires a value`);
        if (name === '--dataset-dir') options.datasetDirectory = value;
        else if (name === '--snapshot-token') options.token = value;
        else if (name === '--concurrency') options.concurrency = Number(value);
        else throw new Error(`unsupported argument ${name}`);
    }
    if (!Number.isInteger(options.concurrency) || options.concurrency < 1) throw new Error('--concurrency must be a positive integer');
    options.token ??= `golden-${new Date().toISOString().replace(/[:.]/gu, '-')}`;
    if (!/^[A-Za-z0-9._-]{1,100}$/u.test(options.token)) throw new Error('--snapshot-token contains unsupported characters');
    return options;
}

function log(message) {
    process.stdout.write(`${new Date().toISOString()} ${message}\n`);
}

// Cuenta las transacciones que cada funcion debe confirmar: sirve para detectar
// escrituras de mas o de menos en el rango de bloques de la construccion.
function expectedTransactionsByFunction(recipes) {
    const counts = {};
    for (const recipe of recipes) {
        for (const step of recipe.steps) counts[step.operation] = (counts[step.operation] ?? 0) + 1;
    }
    return counts;
}

function compareCounts(expected, observed) {
    const names = new Set([...Object.keys(expected), ...Object.keys(observed)]);
    return [...names].filter((name) => (expected[name] ?? 0) !== (observed[name] ?? 0))
        .map((name) => `${name}: esperadas ${expected[name] ?? 0}, confirmadas ${observed[name] ?? 0}`);
}

async function main() {
    const options = parseArguments(process.argv.slice(2));
    const repoRoot = path.resolve(__dirname, '..', '..', '..');
    const sourceTruth = readSourceTruth(repoRoot);
    const bundle = loadDatasetBundle(path.resolve(repoRoot, options.datasetDirectory ?? path.join('build', 'dataset')));
    if (bundle.manifest.dataset.sha256 !== CANONICAL_DATASET_SHA256) {
        throw new Error(`dataset must have canonical SHA-256 ${CANONICAL_DATASET_SHA256}`);
    }
    const plan = buildGoldenSnapshotPlan(bundle.dataset, bundle.manifest.seed);
    const recipes = snapshotRecipes(bundle.dataset, plan);
    const preconditions = expectedPreconditions(bundle.dataset, plan);
    const expectedMarkers = expectedPreparationMarkers(recipes);
    const planSHA256 = sha256(JSON.stringify(plan));

    const directory = path.join(repoRoot, 'build', 'benchmarks', 'fabric-snapshots', options.token);
    if (fs.existsSync(path.join(directory, 'manifest.json'))) throw new Error(`snapshot ${options.token} is already complete`);
    fs.mkdirSync(directory, {recursive: true});
    const statePath = path.join(directory, 'build-state.json');
    writeJSONAtomic(path.join(directory, 'snapshot-plan.json'), plan);
    const preconditionsPath = path.join(directory, 'preconditions.json');
    writeJSONAtomic(preconditionsPath, preconditions);

    const session = connectAllOrganizations(repoRoot, sourceTruth, CALIPER_VERSION);
    const {pool, runtime} = session;
    let state;
    try {
        if (fs.existsSync(statePath)) {
            state = readJSON(statePath);
            if (state.planSHA256 !== planSHA256) throw new Error('existing build state belongs to another snapshot plan');
            log(`snapshot ${options.token}: se reanuda desde el bloque ${state.startBlock}`);
        } else {
            state = {planSHA256, startBlock: await currentHeight(pool), sessions: []};
            writeJSONAtomic(statePath, state);
            log(`snapshot ${options.token}: construccion desde el bloque ${state.startBlock}`);
        }

        const sessionStarted = new Date();
        const execution = await executeRecipes({
            pool, dataset: bundle.dataset, recipes, directory, concurrency: options.concurrency, log,
        });
        const sessionEnded = new Date();
        state.sessions.push({
            startedAt: sessionStarted.toISOString(), endedAt: sessionEnded.toISOString(),
            seconds: (sessionEnded - sessionStarted) / 1000, concurrency: options.concurrency, ...execution,
        });
        writeJSONAtomic(statePath, state);

        log('snapshot: verificacion semantica de las 50.000 unidades');
        const verification = await verifyPreconditions({
            pool, dataset: bundle.dataset, preconditions, concurrency: options.concurrency,
        });
        writeJSONAtomic(path.join(directory, 'verification.json'), verification);
        if (verification.mismatches.length > 0) {
            throw new Error(`${verification.mismatches.length} unidades no coinciden con su precondicion (verification.json)`);
        }

        const endBlock = await currentHeight(pool);
        log(`snapshot: recuento de marcadores en los bloques ${state.startBlock}..${endBlock - 1}`);
        const tally = await countMarkers(pool, state.startBlock, endBlock);
        writeJSONAtomic(path.join(directory, 'block-tally.json'), {startBlock: state.startBlock, endBlock, ...tally});
        const transactionDifferences = compareCounts(expectedTransactionsByFunction(recipes), tally.byFunction);
        if (transactionDifferences.length > 0) {
            throw new Error(`las transacciones confirmadas no coinciden con las recetas: ${transactionDifferences.join('; ')}`);
        }
        const markers = {
            ...expectedMarkers,
            observed: tally.markers.total,
            fromRegistrations: tally.markers.fromRegistrations,
            fromRegulatoryEvents: tally.markers.fromRegulatoryEvents,
            successfulWriteTransactions: tally.validTransactions,
        };
        if (markers.observed !== expectedMarkers.expected
            || markers.fromRegistrations !== expectedMarkers.fromRegistrations
            || markers.fromRegulatoryEvents !== expectedMarkers.fromRegulatoryEvents) {
            throw new Error(`marcadores observados ${JSON.stringify(tally.markers)} difieren de los esperados ${JSON.stringify(expectedMarkers)}`);
        }

        const durationSeconds = state.sessions.reduce((sum, entry) => sum + entry.seconds, 0);
        const context = {
            repositoryCommit: run('git', ['rev-parse', 'HEAD'], {cwd: repoRoot}).stdout.trim(),
            dataset: {
                seed: bundle.manifest.seed, sha256: bundle.manifest.dataset.sha256,
                manifestSHA256: bundle.manifestSHA256, units: bundle.manifest.dataset.units,
            },
            host: buildHostMetadata(),
            environment: {
                docker: runtime.docker, dockerCompose: runtime.dockerCompose,
                contractVersion: sourceTruth.contractVersion, packageID: sourceTruth.chaincode.packageID,
                fabric: runtime.fabric, fabricCA: runtime.fabricCA, caliper: CALIPER_VERSION,
            },
        };
        const transactions = recipes.reduce((sum, recipe) => sum + recipe.steps.length, 0);
        const metadata = buildPreparationMetadata({
            context, recipes: recipes.length, transactions, transactionsByFunction: tally.byFunction, durationSeconds,
            startedAt: state.sessions[0].startedAt, endedAt: state.sessions.at(-1).endedAt,
            markers, concurrency: options.concurrency,
        });
        const metadataPath = path.join(directory, 'metadata.json');
        writeJSONAtomic(metadataPath, metadata);
        run('go', ['run', './cmd/runmeta', metadataPath], {cwd: path.join(repoRoot, 'client'), inherit: true});

        pool.close();
        log('snapshot: guardado de los diez volumenes del ledger con la red detenida');
        const saveStarted = Date.now();
        const volumes = saveVolumes(repoRoot, directory, runtime);
        const manifest = {
            schemaVersion: SNAPSHOT_SCHEMA_VERSION,
            snapshotId: sha256(volumes.map((entry) => entry.sha256).join('') + bundle.manifest.dataset.sha256),
            dataset: context.dataset,
            counts: plan.counts,
            smokeSequence: plan.smokeSequence,
            blocks: {startBlock: state.startBlock, endBlock},
            preparation: {sessions: state.sessions, durationSeconds, transactions, markers},
            artifacts: {...readArtifacts(repoRoot, sourceTruth, runtime), repositoryCommit: context.repositoryCommit},
            files: {
                preconditions: {name: path.basename(preconditionsPath), sha256: sha256File(preconditionsPath)},
                metadata: {name: 'metadata.json', sha256: sha256File(metadataPath)},
                volumes,
            },
            saveSeconds: (Date.now() - saveStarted) / 1000,
            diskBytes: volumes.reduce((sum, entry) => sum + entry.bytes, 0),
        };
        writeSnapshotManifest(directory, manifest);
        log(`snapshot ${manifest.snapshotId} completo en ${path.relative(repoRoot, directory)}`);
    } finally {
        pool.close();
    }
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write(`Fabric snapshot build failed: ${error.message}\n`);
        process.exitCode = 1;
    });
}

module.exports = {compareCounts, expectedTransactionsByFunction, parseArguments};
