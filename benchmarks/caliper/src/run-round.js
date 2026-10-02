#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const {resolveOrganizationCredentials} = require('./credentials');
const {GatewayPool, invokePreparation} = require('./gateway-bridge');
const {buildRoundMetadata, writeJSONAtomic} = require('./metadata');
const {buildMultiOrganizationNetworkConfig, inspectOrganizationRuntime} = require('./network-config');
const {buildPlan} = require('./planner');
const {buildProfile} = require('./profiles');
const {aggregateResults} = require('./raw-results');
const {loadDatasetBundle, readSourceTruth} = require('./sources');

const CALIPER_VERSION = require('../package.json').devDependencies['@hyperledger/caliper-cli'];
const PREPARATION_CONCURRENCY = 8;

function run(command, args, options = {}) {
    const result = spawnSync(command, args, {
        cwd: options.cwd,
        encoding: 'utf8',
        stdio: options.inherit ? 'inherit' : ['ignore', 'pipe', 'pipe'],
        env: process.env,
    });
    if (result.error) throw result.error;
    if (result.status !== 0 && !options.allowFailure) {
        throw new Error(`${command} ${args.join(' ')} failed: ${(result.stderr || result.stdout || '').trim()}`);
    }
    return result;
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
    const supported = new Set(['scenario', 'phase', 'repetition', 'rate', 'family', 'operation', 'dataset-dir', 'run-token']);
    for (const key of Object.keys(values)) {
        if (!supported.has(key)) throw new Error(`unsupported argument --${key}`);
    }
    for (const required of ['scenario', 'phase', 'repetition', 'rate']) {
        if (!values[required]) throw new Error(`--${required} is required`);
    }
    return {
        scenario: values.scenario,
        phase: values.phase,
        repetition: values.repetition,
        rate: values.rate,
        family: values.family,
        operation: values.operation,
        datasetDirectory: values['dataset-dir'],
        runToken: values['run-token'],
    };
}

function createRunDirectory(repoRoot, explicitToken) {
    const token = explicitToken ?? new Date().toISOString().replace(/[:.]/gu, '-');
    if (!/^[A-Za-z0-9._-]{1,100}$/u.test(token)) throw new Error('--run-token contains unsupported characters');
    const runDirectory = path.join(repoRoot, 'build', 'benchmarks', 'caliper', token);
    fs.mkdirSync(path.dirname(runDirectory), {recursive: true});
    fs.mkdirSync(runDirectory, {recursive: false});
    return runDirectory;
}

function readOSName() {
    try {
        const contents = fs.readFileSync('/etc/os-release', 'utf8');
        return contents.match(/^PRETTY_NAME=(?:"([^"]+)"|(.*))$/mu)?.slice(1).find(Boolean) ?? os.type();
    } catch {
        return os.type();
    }
}

function buildHostMetadata() {
    const cpu = os.cpus()[0];
    if (!cpu) throw new Error('cannot identify benchmark host CPU');
    const host = {
        cpu: cpu.model.trim(), cpuCores: os.cpus().length,
        memoryGB: Number((os.totalmem() / (1024 ** 3)).toFixed(3)),
        os: readOSName(), kernel: os.release(),
    };
    if (/microsoft/iu.test(os.release())) host.wsl = process.env.WSL_DISTRO_NAME ? `WSL2 ${process.env.WSL_DISTRO_NAME}` : 'WSL2';
    return host;
}

async function preparePlan(plan, networkConfig, options = {}) {
    const pool = options.gatewayPool ?? new GatewayPool(networkConfig);
    const invoke = options.invoke ?? invokePreparation;
    const concurrency = Math.min(
        options.concurrency ?? PREPARATION_CONCURRENCY,
        Math.max(1, plan.preparations.length),
    );
    let nextIndex = 0;
    let completed = 0;
    let firstError;
    try {
        await Promise.all(Array.from({length: concurrency}, async () => {
            while (!firstError) {
                const index = nextIndex;
                nextIndex += 1;
                if (index >= plan.preparations.length) return;
                const preparation = plan.preparations[index];
                let currentOperation = 'unknown';
                try {
                    for (const invocation of preparation.invocations) {
                        currentOperation = invocation.operation;
                        await invoke(pool, invocation);
                    }
                    completed += 1;
                    if (!options.quiet && (completed % 100 === 0 || completed === plan.preparations.length)) {
                        process.stdout.write(`Prepared ${completed}/${plan.preparations.length} dataset recipes\n`);
                    }
                } catch (error) {
                    firstError ??= new Error(
                        `preparation failed for dataset sequence ${preparation.datasetSequence ?? 'unknown'} at ${currentOperation}: ${error.message}`,
                        {cause: error},
                    );
                }
            }
        }));
        if (firstError) throw firstError;
    } finally {
        if (!options.gatewayPool) pool.close();
    }
}

function buildBenchmarkConfig(repoRoot, profile, paths) {
    const controllerTransactionsPerSecond = profile.rate * profile.transactionsPerOperation;
    return {
        test: {
            name: `EVAL-2 ${profile.scenario}`,
            description: 'Ronda individual conforme a docs/measurement-protocol.md',
            workers: {type: 'local', number: profile.workers},
            rounds: [{
                label: `${profile.scenario}-${profile.rate}`,
                description: `${profile.rate} operaciones conceptuales por segundo (${controllerTransactionsPerSecond} tx/s) durante ${profile.durationSeconds} s`,
                txDuration: profile.durationSeconds,
                rateControl: {type: 'fixed-rate', opts: {tps: controllerTransactionsPerSecond}},
                workload: {
                    module: path.join(repoRoot, 'benchmarks', 'caliper', 'workloads', `${profile.module}.js`),
                    arguments: {
                        moduleName: profile.module,
                        workPlanPath: paths.workPlanPath,
                        networkConfigPath: paths.networkConfigPath,
                        partPaths: paths.partPaths,
                    },
                },
            }],
        },
    };
}

async function main() {
    const options = parseArguments(process.argv.slice(2));
    const profile = buildProfile(options);
    const repoRoot = path.resolve(__dirname, '..', '..', '..');
    const datasetDirectory = path.resolve(repoRoot, options.datasetDirectory ?? path.join('build', 'dataset'));
    const sourceTruth = readSourceTruth(repoRoot);
    const bundle = loadDatasetBundle(datasetDirectory);
    const plan = buildPlan(bundle.dataset, profile, bundle.manifest.seed);
    const credentials = resolveOrganizationCredentials(repoRoot, plan.requiredMspIds);
    const runtime = inspectOrganizationRuntime(repoRoot, credentials);
    const networkConfig = buildMultiOrganizationNetworkConfig({
        organizations: credentials,
        endpoints: runtime.endpoints,
        sourceTruth,
        versions: runtime,
        caliperVersion: CALIPER_VERSION,
    });
    const runDirectory = createRunDirectory(repoRoot, options.runToken);
    const paths = {
        networkConfigPath: path.join(runDirectory, 'network-config.json'),
        benchmarkConfigPath: path.join(runDirectory, 'benchmark-config.json'),
        workPlanPath: path.join(runDirectory, 'work-plan.json'),
        reportPath: path.join(runDirectory, 'report.html'),
        metadataPath: path.join(runDirectory, 'metadata.json'),
        partPaths: Array.from({length: profile.workers}, (_, index) => path.join(runDirectory, `raw-worker-${index}.jsonl`)),
    };
    const benchmarkConfig = buildBenchmarkConfig(repoRoot, profile, paths);
    writeJSONAtomic(paths.networkConfigPath, networkConfig);
    writeJSONAtomic(paths.workPlanPath, plan);
    writeJSONAtomic(paths.benchmarkConfigPath, benchmarkConfig);

    await preparePlan(plan, networkConfig);
    const repositoryCommit = run('git', ['rev-parse', 'HEAD'], {cwd: repoRoot}).stdout.trim();
    const startedAt = new Date().toISOString();
    const caliper = path.join(__dirname, '..', 'node_modules', '.bin', 'caliper');
    const caliperResult = run(caliper, [
        'launch', 'manager',
        '--caliper-workspace', repoRoot,
        '--caliper-benchconfig', paths.benchmarkConfigPath,
        '--caliper-networkconfig', paths.networkConfigPath,
        '--caliper-flow-only-test',
        '--caliper-report-path', paths.reportPath,
    ], {cwd: repoRoot, inherit: true, allowFailure: true});
    const endedAt = new Date().toISOString();
    const aggregated = aggregateResults(runDirectory, profile);
    if (caliperResult.status !== 0 && !aggregated.summary.discardReason) {
        aggregated.summary.discardReason = `Caliper terminó con código ${caliperResult.status}`;
        writeJSONAtomic(path.join(runDirectory, 'summary.json'), aggregated.summary);
    }
    const metadata = buildRoundMetadata({
        profile, repositoryCommit, startedAt, endedAt,
        dataset: {
            seed: bundle.manifest.seed,
            sha256: bundle.manifest.dataset.sha256,
            manifestSHA256: bundle.manifestSHA256,
            units: bundle.manifest.dataset.units,
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
    }, aggregated.summary);
    writeJSONAtomic(paths.metadataPath, metadata);
    run('go', ['run', './cmd/runmeta', paths.metadataPath], {cwd: path.join(repoRoot, 'client'), inherit: true});
    if (!fs.existsSync(paths.reportPath)) throw new Error('Caliper did not produce report.html');
    const report = fs.readFileSync(paths.reportPath, 'utf8');
    if (!report.includes(sourceTruth.contractVersion) || !report.includes(sourceTruth.chaincode.packageID)) {
        throw new Error('Caliper report omits the canonical contract version or packageID');
    }
    if (aggregated.summary.discardReason) throw new Error(aggregated.summary.discardReason);
    process.stdout.write(`EVAL-2 round completed: ${runDirectory}\n`);
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write(`EVAL-2 round failed: ${error.message}\n`);
        process.exitCode = 1;
    });
}

module.exports = {
    PREPARATION_CONCURRENCY,
    buildBenchmarkConfig,
    buildHostMetadata,
    createRunDirectory,
    parseArguments,
    preparePlan,
};
