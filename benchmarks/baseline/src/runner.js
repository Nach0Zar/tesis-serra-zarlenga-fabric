'use strict';

const fs = require('node:fs');
const path = require('node:path');

const {writeJSONAtomic} = require('../../caliper/src/metadata');
const {buildPlan} = require('../../caliper/src/planner');
const {buildProfile} = require('../../caliper/src/profiles');
const {aggregateResults} = require('../../caliper/src/raw-results');
const {loadDatasetBundle, readSourceTruth, sha256File} = require('../../caliper/src/sources');
const {resolveCredentials, publicCredentialConfiguration} = require('./credentials');
const {runFixedRate} = require('./execution');
const {BaselineClient} = require('./http-client');
const {buildHostMetadata, buildRoundMetadata, buildSmokeMetadata} = require('./metadata');
const {createOutputDirectory, repoRoot} = require('./paths');
const {readPreconditions, sequencesForPlan, verifyPreconditions} = require('./preconditions');
const {compose, repositoryCommit, run} = require('./runtime');
const {runReadSmoke} = require('./smoke');
const {
    CANONICAL_DATASET_SHA256, CONTRACT_VERSION, PACKAGE_ID, databaseCounts, restoreSnapshot,
} = require('./snapshot');

function datasetMetadata(bundle) {
    return {
        seed: bundle.manifest.seed,
        sha256: bundle.manifest.dataset.sha256,
        manifestSHA256: bundle.manifestSHA256,
        units: bundle.manifest.dataset.units,
    };
}

function assertCanonical(bundle, sourceTruth) {
    if (bundle.manifest.dataset.sha256 !== CANONICAL_DATASET_SHA256) {
        throw new Error(`dataset must have canonical SHA-256 ${CANONICAL_DATASET_SHA256}`);
    }
    if (sourceTruth.contractVersion !== CONTRACT_VERSION || sourceTruth.chaincode.packageID !== PACKAGE_ID) {
        throw new Error('canonical contract version or packageID changed; this benchmark must be reviewed');
    }
}

function validateMetadata(metadataPath) {
    run('go', ['run', './cmd/runmeta', metadataPath], {
        cwd: path.join(repoRoot, 'client'), inherit: true,
    });
}

function roundContext({bundle, profile, restored, sourceTruth, credentials}) {
    return {
        repositoryCommit: repositoryCommit(repoRoot),
        dataset: datasetMetadata(bundle),
        profile,
        snapshot: {
            id: restored.manifest.snapshotId,
            directory: restored.directory,
            manifestSHA256: sha256File(path.join(restored.directory, 'manifest.json')),
        },
        referenceArtifacts: {
            contractVersion: sourceTruth.contractVersion,
            packageID: sourceTruth.chaincode.packageID,
            fabric: restored.manifest.artifacts.fabric,
        },
        credentials: publicCredentialConfiguration(credentials),
        host: buildHostMetadata(),
        environment: {
            docker: restored.runtime.docker,
            dockerCompose: restored.runtime.dockerCompose,
            baselineCommit: restored.manifest.artifacts.baselineCommit,
            baselineImage: restored.manifest.artifacts.baselineImage,
            postgres: restored.runtime.postgres,
        },
    };
}

async function executeSmoke({bundle, restored, sourceTruth, credentials, outputDirectory}) {
    fs.mkdirSync(outputDirectory, {recursive: true});
    const client = new BaselineClient({baseURL: restored.apiURL, credentials});
    const context = roundContext({bundle, restored, sourceTruth, credentials});
    writeJSONAtomic(path.join(outputDirectory, 'run-context.json'), context);
    let summary;
    let failure;
    const started = Date.now();
    try {
        const preconditions = readPreconditions(restored);
        await verifyPreconditions(client, bundle.dataset, preconditions, [restored.manifest.smokeSequence]);
        summary = await runReadSmoke({
            client, dataset: bundle.dataset, sequence: restored.manifest.smokeSequence, outputDirectory,
        });
        const counts = databaseCounts(repoRoot, restored.environment);
        writeJSONAtomic(path.join(outputDirectory, 'post-run-counts.json'), counts);
        if (Number(counts.labInterventions) !== 0 || Number(counts.labInterventionEvents) !== 0) {
            throw new Error('laboratory intervention tables received an unexpected write during smoke');
        }
        if (summary.discardReason) throw new Error(summary.discardReason);
    } catch (error) {
        failure = error;
        if (!summary) {
            const ended = Math.max(Date.now(), started + 1);
            summary = {
                transactions: 0, successfulTransactions: 0, failedTransactions: 0,
                startedAt: new Date(started).toISOString(), endedAt: new Date(ended).toISOString(),
                observedDurationSeconds: (ended - started) / 1000,
                effectiveTransactionsPerSecond: 0,
                discardReason: error.message,
            };
            writeJSONAtomic(path.join(outputDirectory, 'raw.json'), []);
            writeJSONAtomic(path.join(outputDirectory, 'summary.json'), summary);
        } else if (!summary.discardReason) {
            summary.discardReason = error.message;
            writeJSONAtomic(path.join(outputDirectory, 'summary.json'), summary);
        }
    }
    const metadata = buildSmokeMetadata(context, summary);
    const metadataPath = path.join(outputDirectory, 'metadata.json');
    writeJSONAtomic(metadataPath, metadata);
    validateMetadata(metadataPath);
    if (failure) throw failure;
    return {context, summary, metadata};
}

async function runStandaloneSmoke(options) {
    const datasetDirectory = path.resolve(repoRoot, options.datasetDirectory ?? path.join('build', 'dataset'));
    const bundle = loadDatasetBundle(datasetDirectory);
    const sourceTruth = readSourceTruth(repoRoot);
    assertCanonical(bundle, sourceTruth);
    const restored = restoreSnapshot(options.snapshotDirectory);
    const credentials = resolveCredentials(repoRoot, ['LabMSP'], restored.environment);
    const runDirectory = createOutputDirectory('runs', options.runToken);
    try {
        await executeSmoke({bundle, restored, sourceTruth, credentials, outputDirectory: runDirectory});
        process.stdout.write(`Baseline smoke completed: ${runDirectory}\n`);
        return runDirectory;
    } finally {
        compose(repoRoot, ['down', '--volumes', '--remove-orphans'], {env: restored.environment, allowFailure: true});
    }
}

async function runRound(options) {
    const profile = buildProfile(options);
    const datasetDirectory = path.resolve(repoRoot, options.datasetDirectory ?? path.join('build', 'dataset'));
    const bundle = loadDatasetBundle(datasetDirectory);
    const sourceTruth = readSourceTruth(repoRoot);
    assertCanonical(bundle, sourceTruth);
    const plan = buildPlan(bundle.dataset, profile, bundle.manifest.seed);
    const restored = restoreSnapshot(options.snapshotDirectory);
    const credentials = resolveCredentials(repoRoot, plan.requiredMspIds, restored.environment);
    const client = new BaselineClient({baseURL: restored.apiURL, credentials});
    const runDirectory = createOutputDirectory('runs', options.runToken);
    const context = roundContext({bundle, profile, restored, sourceTruth, credentials});
    const attemptStarted = Date.now();
    try {
        writeJSONAtomic(path.join(runDirectory, 'effective-config.json'), {
            apiURL: restored.apiURL,
            composeProject: restored.environment.COMPOSE_PROJECT_NAME,
            profile,
            credentials: context.credentials,
        });
        writeJSONAtomic(path.join(runDirectory, 'work-plan.json'), plan);
        writeJSONAtomic(path.join(runDirectory, 'run-context.json'), context);
        const preconditions = readPreconditions(restored);
        const verified = await verifyPreconditions(
            client, bundle.dataset, preconditions, sequencesForPlan(plan, restored.manifest.smokeSequence),
        );
        writeJSONAtomic(path.join(runDirectory, 'snapshot-context.json'), {
            snapshot: context.snapshot,
            referenceArtifacts: context.referenceArtifacts,
            verifiedPreconditions: verified,
            counts: restored.manifest.counts,
        });
        await executeSmoke({
            bundle, restored, sourceTruth, credentials,
            outputDirectory: path.join(runDirectory, 'preflight'),
        });

        const partPaths = Array.from({length: profile.workers}, (_, index) =>
            path.join(runDirectory, `raw-worker-${index}.jsonl`));
        const observedWindow = await runFixedRate({client, plan, profile, partPaths});
        const aggregated = aggregateResults(runDirectory, profile, observedWindow);
        const counts = databaseCounts(repoRoot, restored.environment);
        writeJSONAtomic(path.join(runDirectory, 'post-run-counts.json'), counts);
        if (Number(counts.labInterventions) !== 0 || Number(counts.labInterventionEvents) !== 0) {
            aggregated.summary.discardReason = 'laboratory intervention tables received an unexpected write';
            writeJSONAtomic(path.join(runDirectory, 'summary.json'), aggregated.summary);
        }
        const metadata = buildRoundMetadata({
            ...context,
            startedAt: aggregated.summary.startedAt,
            endedAt: aggregated.summary.endedAt,
        }, aggregated.summary);
        const metadataPath = path.join(runDirectory, 'metadata.json');
        writeJSONAtomic(metadataPath, metadata);
        validateMetadata(metadataPath);
        if (aggregated.summary.discardReason) throw new Error(aggregated.summary.discardReason);
        process.stdout.write(`Baseline round completed: ${runDirectory}\n`);
        return runDirectory;
    } catch (error) {
        const metadataPath = path.join(runDirectory, 'metadata.json');
        if (!fs.existsSync(metadataPath)) {
            const ended = Math.max(Date.now(), attemptStarted + 1);
            const durationSeconds = (ended - attemptStarted) / 1000;
            if (!fs.existsSync(path.join(runDirectory, 'raw.json'))) {
                writeJSONAtomic(path.join(runDirectory, 'raw.json'), []);
            }
            const summary = {
                scenario: profile.scenario,
                operationCount: 0,
                successfulOperations: 0,
                expectedRejections: 0,
                unexpectedFailures: 0,
                transactionCount: 0,
                successfulTransactions: 0,
                failedTransactions: 0,
                observedErrorCodes: {},
                rate: {
                    operation: profile.operation,
                    targetOperationsPerSecond: profile.rate,
                    targetTransactionsPerSecond: profile.rate * profile.transactionsPerOperation,
                    effectiveTransactionsPerSecond: 0,
                },
                startedAt: new Date(attemptStarted).toISOString(),
                endedAt: new Date(ended).toISOString(),
                observedDurationSeconds: durationSeconds,
                discardReason: error.message,
            };
            writeJSONAtomic(path.join(runDirectory, 'summary.json'), summary);
            const metadata = buildRoundMetadata({
                ...context,
                profile: {...profile, durationSeconds},
                startedAt: summary.startedAt,
                endedAt: summary.endedAt,
            }, summary);
            writeJSONAtomic(metadataPath, metadata);
            validateMetadata(metadataPath);
        }
        throw error;
    } finally {
        compose(repoRoot, ['down', '--volumes', '--remove-orphans'], {env: restored.environment, allowFailure: true});
    }
}

module.exports = {
    assertCanonical, datasetMetadata, executeSmoke, roundContext, runRound, runStandaloneSmoke, validateMetadata,
};
