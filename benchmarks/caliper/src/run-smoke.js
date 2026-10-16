#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const {resolveLabCredentials} = require('./credentials');
const {buildHostMetadata} = require('./host');
const {buildNetworkConfig, inspectRuntime} = require('./network-config');
const {readSnapshot, sha256File} = require('./artifacts');
const {assertSnapshotCompatibility} = require('./snapshot');
const {readPreconditions} = require('./snapshot-preconditions');
const {registrationForUnit} = require('./snapshot-plan');
const {loadDatasetBundle, readSourceTruth} = require('./sources');

const EXPECTED_TRANSACTIONS = 30;
const CALIPER_VERSION = require('../package.json').devDependencies['@hyperledger/caliper-cli'];

function run(command, args, options = {}) {
    const result = spawnSync(command, args, {
        cwd: options.cwd,
        encoding: 'utf8',
        stdio: options.inherit ? 'inherit' : ['ignore', 'pipe', 'pipe'],
        env: options.env ?? process.env,
    });
    if (result.error) {
        throw result.error;
    }
    if (result.status !== 0 && !options.allowFailure) {
        throw new Error(`${command} ${args.join(' ')} failed: ${(result.stderr || result.stdout || '').trim()}`);
    }
    return result;
}

function verifyExistingUnit(repoRoot, recipe, runCommand = run) {
    const baseArgs = [
        'run', './cmd/snt-client',
        'read-unit',
        '--repo-root', repoRoot,
        '--org', 'lab',
        '--gtin', recipe.request.gtin,
        '--serial', recipe.request.numeroSerie,
    ];
    const clientDirectory = path.join(repoRoot, 'client');
    const readResult = runCommand('go', baseArgs, {cwd: clientDirectory, allowFailure: true});
    if (readResult.status !== 0) {
        const failure = `${readResult.stdout ?? ''}\n${readResult.stderr ?? ''}`.trim();
        throw new Error(`read-only smoke unit is not available: ${failure}`);
    }
    return 'read-only-existing';
}

function createRunDirectory(repoRoot, options = {}) {
    const token = options.runToken ?? process.env.SNT_CALIPER_RUN_TOKEN
        ?? new Date().toISOString().replace(/[:.]/gu, '-');
    if (!/^[A-Za-z0-9._-]{1,100}$/u.test(token)) {
        throw new Error('SNT_CALIPER_RUN_TOKEN contains unsupported characters');
    }
    const outputRoot = path.resolve(repoRoot, options.outputRoot ?? path.join('build', 'benchmarks', 'caliper'));
    const relative = path.relative(path.join(repoRoot, 'build'), outputRoot);
    if (relative.startsWith('..') || path.isAbsolute(relative)) {
        throw new Error('--output-root must resolve inside the repository build directory');
    }
    const runDirectory = path.join(outputRoot, token);
    fs.mkdirSync(path.dirname(runDirectory), {recursive: true});
    fs.mkdirSync(runDirectory, {recursive: false});
    return runDirectory;
}

function writeJSON(filePath, value) {
    fs.writeFileSync(filePath, `${JSON.stringify(value, null, 2)}\n`, {mode: 0o600});
}

function verifyReport(reportPath, sourceTruth) {
    const contents = fs.readFileSync(reportPath, 'utf8');
    if (!contents.includes(sourceTruth.contractVersion) || !contents.includes(sourceTruth.chaincode.packageID)) {
        throw new Error('Caliper report does not contain the canonical contract version and packageID');
    }
    const metrics = contents.match(
        /<td>smoke-read-unit<\/td>\s*<td>(\d+)<\/td>\s*<td>(\d+)<\/td>\s*<td>([0-9.]+)<\/td>\s*<td>([0-9.]+)<\/td>\s*<td>([0-9.]+)<\/td>\s*<td>([0-9.]+)<\/td>\s*<td>([0-9.]+)<\/td>/u,
    );
    if (!metrics) {
        throw new Error('Caliper report does not contain the expected throughput and latency columns');
    }
    const [successful, failed] = metrics.slice(1, 3).map(Number);
    const [sendRate, maximumLatency, minimumLatency, averageLatency, throughput] = metrics.slice(3).map(Number);
    if (successful !== EXPECTED_TRANSACTIONS || failed !== 0 || throughput <= 0) {
        throw new Error('Caliper report does not record 30 successful requests and non-zero throughput');
    }
    if (![sendRate, maximumLatency, minimumLatency, averageLatency, throughput].every(Number.isFinite)
        || minimumLatency < 0 || averageLatency < minimumLatency || maximumLatency < averageLatency) {
        throw new Error('Caliper report contains invalid throughput or latency metrics');
    }
}

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
    const supported = new Set(['snapshot-dir', 'sequence', 'dataset-dir', 'run-token', 'output-root']);
    for (const key of Object.keys(values)) if (!supported.has(key)) throw new Error(`unsupported argument --${key}`);
    if (!values['snapshot-dir'] && !values.sequence) {
        throw new Error('read-only smoke requires --snapshot-dir or an explicit existing --sequence');
    }
    return {
        snapshotDirectory: values['snapshot-dir'],
        sequence: values.sequence === undefined ? undefined : Number(values.sequence),
        datasetDirectory: values['dataset-dir'],
        runToken: values['run-token'],
        outputRoot: values['output-root'],
    };
}

function smokeRegistration(dataset, sequence) {
    if (!Number.isInteger(sequence) || sequence < 1) throw new Error('smoke sequence must be a positive integer');
    const unit = dataset.units.find((entry) => entry.sequence === sequence);
    if (!unit) throw new Error(`smoke sequence ${sequence} is absent from the dataset`);
    const registration = registrationForUnit(unit);
    if (registration.invokerMspId !== 'LabMSP') throw new Error('smoke sequence must be readable through LabMSP');
    return {...registration, sequence};
}

function main(arguments_ = process.argv.slice(2)) {
    const options = parseArguments(arguments_);
    const repoRoot = path.resolve(__dirname, '..', '..', '..');
    const datasetDirectory = path.resolve(
        repoRoot,
        options.datasetDirectory ?? process.env.SNT_CALIPER_DATASET_DIR ?? path.join('build', 'dataset'),
    );
    const sourceTruth = readSourceTruth(repoRoot);
    const datasetBundle = loadDatasetBundle(datasetDirectory);
    let snapshot;
    let sequence = options.sequence;
    if (options.snapshotDirectory) {
        snapshot = readSnapshot(repoRoot, options.snapshotDirectory);
        assertSnapshotCompatibility(repoRoot, snapshot);
        if (snapshot.manifest.dataset.sha256 !== datasetBundle.manifest.dataset.sha256) {
            throw new Error('snapshot and smoke dataset hashes differ');
        }
        sequence = snapshot.manifest.smokeSequence;
        const expected = readPreconditions(snapshot).get(sequence);
        if (!expected || expected.unit === null) throw new Error('snapshot smoke sequence is not a registered filler unit');
    }
    const recipe = smokeRegistration(datasetBundle.dataset, sequence);
    const credentials = resolveLabCredentials(repoRoot);
    const runtime = inspectRuntime(repoRoot, credentials.peerHostname);
    const preparation = verifyExistingUnit(repoRoot, recipe);
    const runDirectory = createRunDirectory(repoRoot, options);
    const metadataPath = path.join(runDirectory, 'metadata.json');
    const contextPath = path.join(runDirectory, 'run-context.json');
    const networkConfigPath = path.join(runDirectory, 'network-config.json');
    const benchmarkConfigPath = path.join(runDirectory, 'benchmark-config.json');
    const reportPath = path.join(runDirectory, 'report.html');

    const networkConfig = buildNetworkConfig({
        credentials,
        endpoint: runtime.endpoint,
        sourceTruth,
        versions: runtime,
        caliperVersion: CALIPER_VERSION,
    });
    const repositoryCommit = run('git', ['rev-parse', 'HEAD'], {cwd: repoRoot}).stdout.trim();
    const context = {
        repositoryCommit,
        expectedTransactions: EXPECTED_TRANSACTIONS,
        dataset: {
            seed: datasetBundle.manifest.seed,
            sha256: datasetBundle.manifest.dataset.sha256,
            manifestSHA256: datasetBundle.manifestSHA256,
            units: datasetBundle.manifest.dataset.units,
        },
        host: buildHostMetadata(),
        environment: {
            docker: runtime.docker,
            dockerCompose: runtime.dockerCompose,
            contractVersion: sourceTruth.contractVersion,
            packageID: sourceTruth.chaincode.packageID,
            fabric: runtime.fabric,
            fabricCA: runtime.fabricCA,
            caliper: CALIPER_VERSION,
        },
        preparation,
        datasetSequence: recipe.sequence,
    };
    if (snapshot) {
        context.snapshot = {
            id: snapshot.manifest.snapshotId,
            manifestSHA256: sha256File(snapshot.manifestPath),
        };
    }
    const benchmarkConfig = {
        test: {
            name: 'SNT Caliper smoke',
            description: 'ReadUnit diagnóstico sobre la red Fabric local',
            workers: {type: 'local', number: 1},
            rounds: [{
                label: 'smoke-read-unit',
                description: '30 ReadUnit a 1 TPS',
                txNumber: EXPECTED_TRANSACTIONS,
                rateControl: {type: 'fixed-rate', opts: {tps: 1}},
                workload: {
                    module: path.join(repoRoot, 'benchmarks', 'caliper', 'workloads', 'smoke-read-unit.js'),
                    arguments: {
                        gtin: recipe.request.gtin,
                        serialNumber: recipe.request.numeroSerie,
                        runContextPath: contextPath,
                        metadataPath,
                    },
                },
            }],
        },
    };
    writeJSON(networkConfigPath, networkConfig);
    writeJSON(benchmarkConfigPath, benchmarkConfig);
    writeJSON(contextPath, context);

    const caliperBinary = path.join(__dirname, '..', 'node_modules', '.bin', 'caliper');
    run(caliperBinary, [
        'launch', 'manager',
        '--caliper-workspace', repoRoot,
        '--caliper-benchconfig', benchmarkConfigPath,
        '--caliper-networkconfig', networkConfigPath,
        '--caliper-flow-only-test',
        '--caliper-report-path', reportPath,
    ], {cwd: repoRoot, inherit: true});

    if (!fs.existsSync(metadataPath)) {
        throw new Error('workload did not emit metadata.json');
    }
    run('go', ['run', './cmd/runmeta', metadataPath], {
        cwd: path.join(repoRoot, 'client'),
        inherit: true,
    });
    verifyReport(reportPath, sourceTruth);

    const metadata = JSON.parse(fs.readFileSync(metadataPath, 'utf8'));
    if (metadata.discarded || metadata.transactions !== EXPECTED_TRANSACTIONS) {
        throw new Error('smoke metadata marks an incomplete or discarded run');
    }
    if (metadata.rate.effectiveTransactionsPerSecond <= 0) {
        throw new Error('smoke reported zero throughput');
    }
    process.stdout.write(`Caliper smoke completed: ${runDirectory}\n`);
}

if (require.main === module) {
    try {
        main();
    } catch (error) {
        process.stderr.write(`Caliper smoke failed: ${error.message}\n`);
        process.exitCode = 1;
    }
}

module.exports = {createRunDirectory, main, parseArguments, smokeRegistration, verifyExistingUnit, verifyReport};
