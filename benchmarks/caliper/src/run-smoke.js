#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const {resolveLabCredentials} = require('./credentials');
const {selectLabRegistration} = require('./dataset');
const {buildNetworkConfig, inspectRuntime} = require('./network-config');
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

function readOSName() {
    try {
        const contents = fs.readFileSync('/etc/os-release', 'utf8');
        const match = contents.match(/^PRETTY_NAME=(?:"([^"]+)"|(.*))$/mu);
        return match?.[1] ?? match?.[2] ?? os.type();
    } catch {
        return os.type();
    }
}

function buildHostMetadata() {
    const cpu = os.cpus()[0];
    if (!cpu) {
        throw new Error('cannot identify the benchmark host CPU');
    }
    const host = {
        cpu: cpu.model.trim(),
        cpuCores: os.cpus().length,
        memoryGB: Number((os.totalmem() / (1024 ** 3)).toFixed(3)),
        os: readOSName(),
        kernel: os.release(),
    };
    if (/microsoft/iu.test(os.release())) {
        host.wsl = process.env.WSL_DISTRO_NAME
            ? `WSL2 ${process.env.WSL_DISTRO_NAME}`
            : 'WSL2';
    }
    return host;
}

function ensurePreparedUnit(repoRoot, recipe) {
    const baseArgs = [
        'run', './cmd/snt-client',
        'read-unit',
        '--repo-root', repoRoot,
        '--org', 'lab',
        '--gtin', recipe.request.gtin,
        '--serial', recipe.request.numeroSerie,
    ];
    const clientDirectory = path.join(repoRoot, 'client');
    const readResult = run('go', baseArgs, {cwd: clientDirectory, allowFailure: true});
    if (readResult.status === 0) {
        return 'already-present';
    }

    const readFailure = `${readResult.stdout ?? ''}\n${readResult.stderr ?? ''}`;
    if (!readFailure.includes('UNIT_NOT_FOUND')) {
        throw new Error(`cannot determine whether the smoke unit exists: ${readFailure.trim()}`);
    }

    run('go', [
        'run', './cmd/snt-client',
        'register-unit',
        '--repo-root', repoRoot,
        '--org', 'lab',
        '--gtin', recipe.request.gtin,
        '--serial', recipe.request.numeroSerie,
        '--lot', recipe.request.lote,
        '--expiry', recipe.request.fechaVencimiento,
    ], {cwd: clientDirectory, inherit: true});
    return 'registered';
}

function createRunDirectory(repoRoot) {
    const defaultToken = new Date().toISOString().replace(/[:.]/gu, '-');
    const token = process.env.SNT_CALIPER_RUN_TOKEN ?? defaultToken;
    if (!/^[A-Za-z0-9._-]{1,100}$/u.test(token)) {
        throw new Error('SNT_CALIPER_RUN_TOKEN contains unsupported characters');
    }
    const runDirectory = path.join(repoRoot, 'build', 'benchmarks', 'caliper', token);
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

function main() {
    const repoRoot = path.resolve(__dirname, '..', '..', '..');
    const datasetDirectory = path.resolve(
        repoRoot,
        process.env.SNT_CALIPER_DATASET_DIR ?? path.join('build', 'dataset'),
    );
    const sourceTruth = readSourceTruth(repoRoot);
    const datasetBundle = loadDatasetBundle(datasetDirectory);
    const recipe = selectLabRegistration(datasetBundle.dataset);
    const credentials = resolveLabCredentials(repoRoot);
    const runtime = inspectRuntime(repoRoot, credentials.peerHostname);
    const preparation = ensurePreparedUnit(repoRoot, recipe);
    const runDirectory = createRunDirectory(repoRoot);
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
    const benchmarkConfig = {
        test: {
            name: 'EVAL-1 Caliper smoke',
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
    process.stdout.write(`EVAL-1 smoke completed: ${runDirectory}\n`);
}

try {
    main();
} catch (error) {
    process.stderr.write(`EVAL-1 smoke failed: ${error.message}\n`);
    process.exitCode = 1;
}
