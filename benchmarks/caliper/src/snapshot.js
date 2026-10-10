'use strict';

const fs = require('node:fs');
const path = require('node:path');

const {
    SNAPSHOT_SCHEMA_ID,
    canonicalJSON,
    fileDescriptor,
    readSnapshot,
    sha256,
    sha256File,
    validateDocument,
} = require('./artifacts');
const {resolveOrganizationCredentials} = require('./credentials');
const {GatewayPool} = require('./gateway-bridge');
const {buildHostMetadata} = require('./host');
const {buildPreparationMetadata, writeJSONAtomic} = require('./metadata');
const {buildMultiOrganizationNetworkConfig, inspectOrganizationRuntime} = require('./network-config');
const {buildGoldenSnapshotPlan} = require('./snapshot-plan');
const {
    COMPOSE_PROJECT,
    assertVolumeManifest,
    compose,
    removeManagedChaincodeContainers,
    restoreVolumes,
    run,
    saveVolumes,
} = require('./snapshot-volumes');
const {
    PreparationJournal,
    buildPreconditions,
    journalStatistics,
    parallel,
    prepareRecipe,
    verifySnapshotState,
} = require('./snapshot-state');
const {loadDatasetBundle, readSourceTruth} = require('./sources');

const CALIPER_VERSION = require('../package.json').devDependencies['@hyperledger/caliper-cli'];
const CANONICAL_DATASET_SHA256 = 'de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d';
const DEFAULT_CONCURRENCY = 64;
const SOURCE_ROOTS = Object.freeze([
    'benchmarks/caliper/package.json',
    'benchmarks/caliper/package-lock.json',
    'benchmarks/caliper/src',
    'benchmarks/caliper/workloads',
    'benchmarks/schema',
    'docs/measurement-protocol.md',
    'network/chaincode-package.lock',
    'network/collections_config.json',
    'network/compose.yaml',
    'network/organizations-manifest.json',
]);

function repositoryCommit(repoRoot) {
    return run('git', ['rev-parse', 'HEAD'], {cwd: repoRoot}).stdout.trim();
}

function collectFiles(candidate) {
    const stat = fs.statSync(candidate);
    if (stat.isFile()) return [candidate];
    const files = [];
    for (const entry of fs.readdirSync(candidate, {withFileTypes: true}).sort((a, b) => a.name.localeCompare(b.name))) {
        if (entry.name === 'node_modules') continue;
        const child = path.join(candidate, entry.name);
        if (entry.isDirectory()) files.push(...collectFiles(child));
        else if (entry.isFile()) files.push(child);
    }
    return files;
}

function digestFiles(repoRoot, roots) {
    const entries = roots.flatMap((root) => collectFiles(path.join(repoRoot, root)))
        .sort((left, right) => left.localeCompare(right))
        .map((filePath) => ({
            path: path.relative(repoRoot, filePath).split(path.sep).join('/'),
            sha256: sha256File(filePath),
        }));
    return sha256(canonicalJSON(entries));
}

function networkConfigDigest(repoRoot) {
    return digestFiles(repoRoot, [
        'network/chaincode-package.lock',
        'network/collections_config.json',
        'network/compose.yaml',
        'network/organizations-manifest.json',
    ]);
}

function sourceDigest(repoRoot) {
    return digestFiles(repoRoot, SOURCE_ROOTS);
}

function assertCanonicalBundle(bundle) {
    if (bundle.manifest.dataset.sha256 !== CANONICAL_DATASET_SHA256) {
        throw new Error(`dataset must have canonical SHA-256 ${CANONICAL_DATASET_SHA256}`);
    }
    if (bundle.manifest.dataset.units !== 50_000) throw new Error('canonical dataset must contain exactly 50000 units');
}

function networkScript(repoRoot, command, options = {}) {
    return run(path.join(repoRoot, 'network', 'network.sh'), [command], {cwd: repoRoot, ...options});
}

function initializeNetwork(repoRoot, resume, options = {}) {
    if (!resume) {
        removeManagedChaincodeContainers(options);
        compose(repoRoot, ['down', '--volumes', '--remove-orphans'], {...options, allowFailure: true});
    }
    networkScript(repoRoot, 'up', options);
    if (!resume) {
        networkScript(repoRoot, 'createChannel', options);
        networkScript(repoRoot, 'deployCC', options);
    }
    networkScript(repoRoot, 'verify', options);
}

function createBuildDirectory(repoRoot, options) {
    const snapshotsRoot = path.join(repoRoot, 'build', 'benchmarks', 'fabric', 'snapshots');
    fs.mkdirSync(snapshotsRoot, {recursive: true});
    if (options.resume) {
        if (!options.snapshotDirectory) throw new Error('--resume requires --snapshot-dir');
        const existing = path.resolve(repoRoot, options.snapshotDirectory);
        if (!fs.statSync(existing).isDirectory()) throw new Error('resume snapshot directory is not a directory');
        return existing;
    }
    const token = options.snapshotToken ?? new Date().toISOString().replace(/[:.]/gu, '-');
    if (!/^[A-Za-z0-9._-]{1,100}$/u.test(token)) throw new Error('snapshot token contains unsupported characters');
    const directory = path.join(snapshotsRoot, `.building-${token}`);
    fs.mkdirSync(directory, {recursive: false, mode: 0o700});
    return directory;
}

function datasetMetadata(bundle) {
    return {
        seed: bundle.manifest.seed,
        sha256: bundle.manifest.dataset.sha256,
        manifestSHA256: bundle.manifestSHA256,
        units: bundle.manifest.dataset.units,
    };
}

function environmentMetadata(sourceTruth, runtime) {
    return {
        docker: runtime.docker,
        dockerCompose: runtime.dockerCompose,
        contractVersion: sourceTruth.contractVersion,
        packageID: sourceTruth.chaincode.packageID,
        fabric: runtime.fabric,
        fabricCA: runtime.fabricCA,
        caliper: CALIPER_VERSION,
    };
}

function readBuildState(filePath) {
    if (!fs.existsSync(filePath)) {
        return {activeDurationMs: 0, startedAt: new Date().toISOString()};
    }
    const state = JSON.parse(fs.readFileSync(filePath, 'utf8'));
    if (!Number.isFinite(state.activeDurationMs) || state.activeDurationMs < 0) throw new Error('invalid snapshot build state');
    if (!state.startedAt || !Number.isFinite(Date.parse(state.startedAt))) throw new Error('invalid snapshot build start timestamp');
    return state;
}

function createBuildTimer(state, buildStatePath, now = Date.now) {
    let accountedThrough = now();
    return {
        elapsedMs() {
            return state.activeDurationMs + Math.max(0, now() - accountedThrough);
        },
        checkpoint() {
            const current = now();
            state.activeDurationMs += Math.max(0, current - accountedThrough);
            accountedThrough = current;
            writeJSONAtomic(buildStatePath, state);
            return state.activeDurationMs;
        },
    };
}

function assertPlanResume(planPath, plan, resume) {
    if (!resume) {
        writeJSONAtomic(planPath, plan);
        return;
    }
    const existing = JSON.parse(fs.readFileSync(planPath, 'utf8'));
    if (canonicalJSON(existing) !== canonicalJSON(plan)) {
        throw new Error('resume plan differs from the canonical dataset plan');
    }
}

function snapshotIdentifier(binding) {
    return sha256(canonicalJSON({
        dataset: binding.dataset,
        counts: binding.counts,
        artifacts: binding.artifacts,
        smokeSequence: binding.smokeSequence,
        files: {
            logicalPlan: binding.files.logicalPlan,
            preconditions: binding.files.preconditions,
            semanticVerification: binding.files.semanticVerification,
        },
        volumes: binding.volumes,
    }));
}

function verifyJournalEvidence(snapshot, preconditions) {
    const journalPath = path.join(snapshot.directory, snapshot.manifest.files.journal.name);
    const journal = new PreparationJournal(journalPath);
    const statistics = journalStatistics(journal);
    const expected = snapshot.manifest.preparation;
    if (statistics.successfulWriteTransactions !== expected.successfulWriteTransactions
        || canonicalJSON(statistics.markers) !== canonicalJSON({
            observed: expected.markers.observed,
            fromRegistrations: expected.markers.fromRegistrations,
            fromRegulatoryEvents: expected.markers.fromRegulatoryEvents,
        })) {
        throw new Error('snapshot journal marker evidence differs from its manifest');
    }
    const preconditionBySequence = new Map(preconditions.map((entry) => [entry.sequence, entry]));
    for (const event of journal.committed()) {
        const history = preconditionBySequence.get(event.datasetSequence)?.history;
        if (!history || history[event.step]?.txId !== event.transactionId) {
            throw new Error(`snapshot journal transaction ${event.transactionId} is not confirmed at its recipe step`);
        }
    }
    const semantic = JSON.parse(fs.readFileSync(path.join(
        snapshot.directory, snapshot.manifest.files.semanticVerification.name,
    ), 'utf8'));
    if (semantic.pairPrivateWrites !== statistics.pairPrivateWrites
        || canonicalJSON(semantic.participationMarkers) !== canonicalJSON({
            expected: expected.markers.expected,
            ...statistics.markers,
        })) {
        throw new Error('snapshot semantic private-write evidence differs from its journal');
    }
    return statistics;
}

async function buildSnapshot(repoRoot, options = {}) {
    const concurrency = Number(options.concurrency ?? DEFAULT_CONCURRENCY);
    const maxAttempts = Number(options.maxAttempts ?? 5);
    if (!Number.isInteger(concurrency) || concurrency < 1) throw new Error('snapshot concurrency must be a positive integer');
    if (!Number.isInteger(maxAttempts) || maxAttempts < 1) throw new Error('snapshot max attempts must be a positive integer');
    const datasetDirectory = path.resolve(repoRoot, options.datasetDirectory ?? path.join('build', 'dataset'));
    const bundle = loadDatasetBundle(datasetDirectory);
    assertCanonicalBundle(bundle);
    const sourceTruth = readSourceTruth(repoRoot);
    const plan = buildGoldenSnapshotPlan(bundle.dataset, bundle.manifest.seed);
    const snapshotDirectory = createBuildDirectory(repoRoot, options);
    const planPath = path.join(snapshotDirectory, 'logical-plan.json');
    const journalPath = path.join(snapshotDirectory, 'preparation-journal.jsonl');
    const buildStatePath = path.join(snapshotDirectory, 'build-state.json');
    assertPlanResume(planPath, plan, options.resume === true);
    const state = readBuildState(buildStatePath);
    if (!options.resume) writeJSONAtomic(buildStatePath, state);
    const buildTimer = createBuildTimer(state, buildStatePath);
    const startedAt = state.startedAt;
    const signalHandlers = new Map(['SIGINT', 'SIGTERM'].map((signal) => {
        const handler = () => {
            try {
                buildTimer.checkpoint();
            } finally {
                process.removeListener(signal, handler);
                process.kill(process.pid, signal);
            }
        };
        process.once(signal, handler);
        return [signal, handler];
    }));

    try {
        initializeNetwork(repoRoot, options.resume === true, options.commandOptions);
        const credentials = resolveOrganizationCredentials(repoRoot);
        const runtime = inspectOrganizationRuntime(repoRoot, credentials);
        const networkConfig = buildMultiOrganizationNetworkConfig({
            organizations: credentials,
            endpoints: runtime.endpoints,
            sourceTruth,
            versions: runtime,
            caliperVersion: CALIPER_VERSION,
        });
        const gatewayPool = new GatewayPool(networkConfig);
        const journal = new PreparationJournal(journalPath);
        const unitBySequence = new Map(bundle.dataset.units.map((unit) => [unit.sequence, unit]));
        let completed = 0;
        try {
            const presentUnits = plan.units.filter((entry) => entry.expected.present);
            await parallel(presentUnits, concurrency, async (logicalUnit) => {
                const unit = unitBySequence.get(logicalUnit.datasetSequence);
                if (!unit) throw new Error(`snapshot recipe references unknown sequence ${logicalUnit.datasetSequence}`);
                await prepareRecipe({
                    gatewayPool,
                    journal,
                    unit,
                    recipe: {datasetSequence: logicalUnit.datasetSequence, invocations: logicalUnit.recipe},
                    maxAttempts,
                });
                completed += 1;
                if (completed % 500 === 0 || completed === presentUnits.length) {
                    process.stdout.write(`Prepared ${completed}/${presentUnits.length} golden recipes\n`);
                }
            });
            const statistics = journalStatistics(journal);
            if (statistics.markers.observed !== plan.expectedMarkers.total
                || statistics.markers.fromRegistrations !== plan.expectedMarkers.registrations
                || statistics.markers.fromRegulatoryEvents !== plan.expectedMarkers.regulatoryEvents) {
                throw new Error(`participation marker mismatch: ${JSON.stringify({expected: plan.expectedMarkers, observed: statistics.markers})}`);
            }
            const preconditions = await buildPreconditions({
                gatewayPool, dataset: bundle.dataset, excludedSequences: plan.excludedSequences, concurrency,
            });
            const preconditionsPath = path.join(snapshotDirectory, 'preconditions.json');
            writeJSONAtomic(preconditionsPath, preconditions);
            const semantic = await verifySnapshotState({
                gatewayPool, dataset: bundle.dataset, preconditions, logicalPlan: plan, concurrency,
            });
            semantic.pairPrivateWrites = statistics.pairPrivateWrites;
            semantic.participationMarkers = {
                expected: plan.expectedMarkers.total,
                ...statistics.markers,
            };
            if (semantic.present !== plan.counts.registered || semantic.absent !== plan.counts.unregistered) {
                throw new Error(`semantic verification counts differ: ${JSON.stringify(semantic)}`);
            }
            const semanticPath = path.join(snapshotDirectory, 'semantic-verification.json');
            writeJSONAtomic(semanticPath, semantic);

            const endedAt = new Date().toISOString();
            const durationSeconds = Math.max(
                buildTimer.elapsedMs() / 1000,
                Number.EPSILON,
            );
            const repositorySHA = repositoryCommit(repoRoot);
            const metadata = buildPreparationMetadata({
                repositoryCommit: repositorySHA,
                dataset: datasetMetadata(bundle),
                concurrency,
                host: buildHostMetadata(),
                environment: environmentMetadata(sourceTruth, runtime),
            }, {
                startedAt,
                endedAt,
                durationSeconds,
                successfulWriteTransactions: statistics.successfulWriteTransactions,
                markers: {expected: plan.expectedMarkers.total, ...statistics.markers},
            });
            const metadataPath = path.join(snapshotDirectory, 'metadata.json');
            writeJSONAtomic(metadataPath, metadata);
            run('go', ['run', './cmd/runmeta', metadataPath], {
                cwd: path.join(repoRoot, 'client'), inherit: true,
            });

            const volumes = saveVolumes(repoRoot, snapshotDirectory, options.commandOptions);
            const files = {
                logicalPlan: fileDescriptor(planPath, snapshotDirectory),
                preconditions: fileDescriptor(preconditionsPath, snapshotDirectory),
                semanticVerification: fileDescriptor(semanticPath, snapshotDirectory),
                preparationMetadata: fileDescriptor(metadataPath, snapshotDirectory),
                journal: fileDescriptor(journalPath, snapshotDirectory),
            };
            const preparation = {
                durationSeconds,
                successfulWriteTransactions: statistics.successfulWriteTransactions,
                concurrency,
                markers: {expected: plan.expectedMarkers.total, ...statistics.markers},
            };
            const artifacts = {
                repositoryCommit: repositorySHA,
                contractVersion: sourceTruth.contractVersion,
                packageID: sourceTruth.chaincode.packageID,
                fabric: runtime.fabric,
                fabricCA: runtime.fabricCA,
                docker: runtime.docker,
                dockerCompose: runtime.dockerCompose,
                composeProject: COMPOSE_PROJECT,
                networkConfigSHA256: networkConfigDigest(repoRoot),
                sourceSHA256: sourceDigest(repoRoot),
            };
            const binding = {
                dataset: datasetMetadata(bundle),
                counts: plan.counts,
                preparation,
                artifacts,
                smokeSequence: plan.smokeSequence,
                files,
                volumes,
            };
            const manifest = {
                $schema: SNAPSHOT_SCHEMA_ID,
                schemaVersion: '1.0.0',
                snapshotId: snapshotIdentifier(binding),
                createdAt: endedAt,
                ...binding,
            };
            validateDocument(repoRoot, 'fabric-snapshot-manifest.schema.json', manifest);
            writeJSONAtomic(path.join(snapshotDirectory, 'manifest.json'), manifest);
            const finalDirectory = path.join(path.dirname(snapshotDirectory), manifest.snapshotId);
            if (path.resolve(finalDirectory) !== path.resolve(snapshotDirectory)) {
                if (fs.existsSync(finalDirectory)) throw new Error(`snapshot directory already exists: ${finalDirectory}`);
                fs.renameSync(snapshotDirectory, finalDirectory);
            }
            process.stdout.write(`Fabric golden snapshot completed: ${finalDirectory}\n`);
            return {snapshotDirectory: finalDirectory, manifest};
        } finally {
            gatewayPool.close();
        }
    } catch (error) {
        buildTimer.checkpoint();
        fs.appendFileSync(path.join(snapshotDirectory, 'failure.log'), `${new Date().toISOString()} ${error.message}\n`, {mode: 0o600});
        throw error;
    } finally {
        for (const [signal, handler] of signalHandlers) process.removeListener(signal, handler);
    }
}

function assertSnapshotCompatibility(repoRoot, snapshot) {
    assertVolumeManifest(snapshot.manifest.volumes);
    if (snapshotIdentifier(snapshot.manifest) !== snapshot.manifest.snapshotId) {
        throw new Error('snapshot identifier does not match its immutable identifiers and hashes');
    }
    const sourceTruth = readSourceTruth(repoRoot);
    if (repositoryCommit(repoRoot) !== snapshot.manifest.artifacts.repositoryCommit) {
        throw new Error('snapshot repository commit differs from the current checkout');
    }
    if (sourceTruth.contractVersion !== snapshot.manifest.artifacts.contractVersion
        || sourceTruth.chaincode.packageID !== snapshot.manifest.artifacts.packageID) {
        throw new Error('snapshot contract version or packageID differs from the current checkout');
    }
    if (networkConfigDigest(repoRoot) !== snapshot.manifest.artifacts.networkConfigSHA256) {
        throw new Error('snapshot network configuration differs from the current checkout');
    }
    if (sourceDigest(repoRoot) !== snapshot.manifest.artifacts.sourceSHA256) {
        throw new Error('snapshot benchmark sources differ from the current checkout');
    }
    return sourceTruth;
}

function restoreSnapshot(repoRoot, snapshotDirectory, options = {}) {
    const started = Date.now();
    const snapshot = readSnapshot(repoRoot, snapshotDirectory);
    assertSnapshotCompatibility(repoRoot, snapshot);
    restoreVolumes(repoRoot, snapshot.directory, snapshot.manifest.volumes, options.commandOptions);
    networkScript(repoRoot, 'verify', options.commandOptions);
    const credentials = resolveOrganizationCredentials(repoRoot);
    const runtime = inspectOrganizationRuntime(repoRoot, credentials);
    if (runtime.fabric !== snapshot.manifest.artifacts.fabric
        || runtime.fabricCA !== snapshot.manifest.artifacts.fabricCA) {
        throw new Error('restored Fabric runtime versions differ from the snapshot manifest');
    }
    return {snapshot, runtime, durationSeconds: (Date.now() - started) / 1000};
}

async function verifySnapshot(repoRoot, snapshotDirectory, options = {}) {
    const snapshot = readSnapshot(repoRoot, snapshotDirectory);
    assertSnapshotCompatibility(repoRoot, snapshot);
    const datasetDirectory = path.resolve(repoRoot, options.datasetDirectory ?? path.join('build', 'dataset'));
    const bundle = loadDatasetBundle(datasetDirectory);
    assertCanonicalBundle(bundle);
    if (bundle.manifest.dataset.sha256 !== snapshot.manifest.dataset.sha256) {
        throw new Error('snapshot dataset differs from the canonical bundle');
    }
    const credentials = resolveOrganizationCredentials(repoRoot);
    const sourceTruth = readSourceTruth(repoRoot);
    const runtime = inspectOrganizationRuntime(repoRoot, credentials);
    const networkConfig = buildMultiOrganizationNetworkConfig({
        organizations: credentials,
        endpoints: runtime.endpoints,
        sourceTruth,
        versions: runtime,
        caliperVersion: CALIPER_VERSION,
    });
    const preconditions = JSON.parse(fs.readFileSync(path.join(
        snapshot.directory, snapshot.manifest.files.preconditions.name,
    ), 'utf8'));
    const logicalPlan = JSON.parse(fs.readFileSync(path.join(
        snapshot.directory, snapshot.manifest.files.logicalPlan.name,
    ), 'utf8'));
    const statistics = verifyJournalEvidence(snapshot, preconditions);
    const gatewayPool = new GatewayPool(networkConfig);
    try {
        const semantic = await verifySnapshotState({
            gatewayPool,
            dataset: bundle.dataset,
            preconditions,
            logicalPlan,
            concurrency: Number(options.concurrency ?? DEFAULT_CONCURRENCY),
        });
        if (semantic.present !== snapshot.manifest.counts.registered
            || semantic.absent !== snapshot.manifest.counts.unregistered) {
            throw new Error(`restored semantic counts differ: ${JSON.stringify(semantic)}`);
        }
        return {...semantic, privateWriteEvidence: statistics};
    } finally {
        gatewayPool.close();
    }
}

function parseSnapshotArguments(arguments_, mode) {
    const values = {};
    for (let index = 0; index < arguments_.length; index += 1) {
        const name = arguments_[index];
        if (name === '--resume') {
            values.resume = true;
            continue;
        }
        const value = arguments_[index + 1];
        if (!name?.startsWith('--') || value === undefined || value.startsWith('--')) {
            throw new Error('arguments must be supplied as --name value pairs; --resume is a flag');
        }
        const key = name.slice(2);
        if (values[key] !== undefined) throw new Error(`duplicate argument --${key}`);
        values[key] = value;
        index += 1;
    }
    const supported = mode === 'build'
        ? new Set(['snapshot-dir', 'snapshot-token', 'dataset-dir', 'concurrency', 'max-attempts', 'resume'])
        : new Set(['snapshot-dir', 'dataset-dir', 'concurrency']);
    for (const key of Object.keys(values)) if (!supported.has(key)) throw new Error(`unsupported argument --${key}`);
    if (mode !== 'build' && !values['snapshot-dir']) throw new Error('--snapshot-dir is required');
    return {
        snapshotDirectory: values['snapshot-dir'],
        snapshotToken: values['snapshot-token'],
        datasetDirectory: values['dataset-dir'],
        concurrency: values.concurrency,
        maxAttempts: values['max-attempts'],
        resume: values.resume === true,
    };
}

module.exports = {
    CALIPER_VERSION,
    CANONICAL_DATASET_SHA256,
    DEFAULT_CONCURRENCY,
    SOURCE_ROOTS,
    assertCanonicalBundle,
    assertPlanResume,
    assertSnapshotCompatibility,
    buildSnapshot,
    createBuildTimer,
    createBuildDirectory,
    datasetMetadata,
    digestFiles,
    environmentMetadata,
    initializeNetwork,
    networkConfigDigest,
    parseSnapshotArguments,
    repositoryCommit,
    restoreSnapshot,
    sourceDigest,
    snapshotIdentifier,
    verifyJournalEvidence,
    verifySnapshot,
};
