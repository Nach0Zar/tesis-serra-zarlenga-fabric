#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const {SERIES_SCHEMA_ID, readSnapshot, sha256File, validateDocument} = require('./artifacts');
const {writeJSONAtomic} = require('./metadata');
const {PROFILES} = require('./profiles');

function fullScenarioMatrix() {
    const matrix = [];
    for (const scenario of ['write-register', 'write-transfer', 'write-dispense', 'read-unit', 'read-history', 'mixed']) {
        for (const rate of PROFILES[scenario].rates) matrix.push({key: `${scenario}-${rate}`, scenario, rate});
    }
    for (const [family, operation] of [
        ['UNAUTHORIZED_TRANSFER', undefined],
        ['DUPLICATE_IDENTITY', undefined],
        ['BLOCKING_STATE', 'transfer'],
        ['BLOCKING_STATE', 'dispense'],
    ]) {
        const suffix = operation ? `${family.toLowerCase()}-${operation}` : family.toLowerCase();
        matrix.push({key: `expected-rejections-${suffix}-5`, scenario: 'expected-rejections', rate: 5, family, operation});
    }
    if (matrix.length !== 20) throw new Error(`expected 20 full-series scenarios, got ${matrix.length}`);
    return matrix;
}

function trialScenarioMatrix() {
    return [
        {key: 'read-unit-50', scenario: 'read-unit', rate: 50},
        {key: 'write-transfer-20', scenario: 'write-transfer', rate: 20},
        {key: 'mixed-20', scenario: 'mixed', rate: 20},
    ];
}

function coefficientOfVariation(values) {
    if (!Array.isArray(values) || values.length === 0) throw new Error('CV requires at least one value');
    if (values.some((value) => !Number.isFinite(value) || value < 0)) throw new Error('CV values must be finite and non-negative');
    const mean = values.reduce((sum, value) => sum + value, 0) / values.length;
    if (mean === 0) return 0;
    const variance = values.reduce((sum, value) => sum + ((value - mean) ** 2), 0) / values.length;
    return Math.sqrt(variance) / mean;
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
    const supported = new Set(['snapshot-dir', 'dataset-dir', 'plan', 'series-token', 'output-root']);
    for (const key of Object.keys(values)) if (!supported.has(key)) throw new Error(`unsupported argument --${key}`);
    if (!values['snapshot-dir']) throw new Error('--snapshot-dir is required');
    if (!['full', 'trial'].includes(values.plan)) throw new Error('--plan must be full or trial');
    return {
        snapshotDirectory: values['snapshot-dir'],
        datasetDirectory: values['dataset-dir'],
        plan: values.plan,
        seriesToken: values['series-token'],
        outputRoot: values['output-root'],
    };
}

function createSeriesDirectory(repoRoot, options) {
    const token = options.seriesToken ?? new Date().toISOString().replace(/[:.]/gu, '-');
    if (!/^[A-Za-z0-9._-]{1,100}$/u.test(token)) throw new Error('series token contains unsupported characters');
    const outputRoot = path.resolve(
        repoRoot, options.outputRoot ?? path.join('build', 'benchmarks', 'fabric', 'series'),
    );
    const relative = path.relative(path.join(repoRoot, 'build'), outputRoot);
    if (relative.startsWith('..') || path.isAbsolute(relative)) {
        throw new Error('--output-root must resolve inside the repository build directory');
    }
    fs.mkdirSync(outputRoot, {recursive: true});
    const directory = path.join(outputRoot, token);
    fs.mkdirSync(directory, {recursive: false, mode: 0o700});
    return {directory, token};
}

function runLogged(command, args, logPath, options = {}) {
    const result = (options.spawnSync ?? spawnSync)(command, args, {
        cwd: options.cwd,
        env: options.env ?? process.env,
        encoding: 'utf8',
        stdio: ['ignore', 'pipe', 'pipe'],
        maxBuffer: 128 * 1024 * 1024,
    });
    const output = `${result.stdout ?? ''}${result.stderr ?? ''}`;
    fs.writeFileSync(logPath, output, {mode: 0o600});
    if (result.error) throw result.error;
    if (result.status !== 0) {
        const error = new Error(`${path.basename(command)} ${args.join(' ')} failed with exit code ${result.status}`);
        error.exitCode = result.status;
        error.logPath = logPath;
        throw error;
    }
    return result;
}

function scenarioArguments(scenario, phase, repetition) {
    const args = [
        '--scenario', scenario.scenario,
        '--phase', phase,
        '--repetition', String(repetition),
        '--rate', String(scenario.rate),
    ];
    if (scenario.family) args.push('--family', scenario.family);
    if (scenario.operation) args.push('--operation', scenario.operation);
    return args;
}

function networkLogs(repoRoot, outputPath, options = {}) {
    const result = (options.spawnSync ?? spawnSync)('docker', [
        'compose', '-f', path.join(repoRoot, 'network', 'compose.yaml'), 'logs', '--no-color',
    ], {cwd: repoRoot, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], maxBuffer: 128 * 1024 * 1024});
    fs.writeFileSync(outputPath, `${result.stdout ?? ''}${result.stderr ?? ''}`, {mode: 0o600});
}

function executeAttempt(repoRoot, options) {
    const attemptDirectory = options.attemptDirectory;
    fs.mkdirSync(attemptDirectory, {recursive: false, mode: 0o700});
    const sourceDirectory = path.join(repoRoot, 'benchmarks', 'caliper', 'src');
    const common = ['--snapshot-dir', options.snapshotDirectory];
    if (options.datasetDirectory) common.push('--dataset-dir', options.datasetDirectory);
    try {
        runLogged(process.execPath, [path.join(sourceDirectory, 'restore-snapshot.js'), ...common],
            path.join(attemptDirectory, 'restore.log'), {cwd: repoRoot});
        runLogged(process.execPath, [path.join(sourceDirectory, 'verify-snapshot.js'), ...common],
            path.join(attemptDirectory, 'verify.log'), {cwd: repoRoot});
        runLogged(process.execPath, [path.join(sourceDirectory, 'warm-peers.js'), ...common],
            path.join(attemptDirectory, 'warm-peers.log'), {cwd: repoRoot});
        runLogged(path.join(repoRoot, 'network', 'scripts', 'measure-resources.sh'), [],
            path.join(attemptDirectory, 'resources.txt'), {cwd: repoRoot});
        runLogged(process.execPath, [
            path.join(sourceDirectory, 'run-smoke.js'), ...common,
            '--output-root', path.relative(repoRoot, attemptDirectory), '--run-token', 'smoke',
        ], path.join(attemptDirectory, 'smoke.log'), {cwd: repoRoot});
        runLogged(process.execPath, [
            path.join(sourceDirectory, 'run-round.js'),
            ...scenarioArguments(options.scenario, options.kind, options.repetition),
            ...common,
            '--state-mode', 'snapshot',
            '--output-root', path.relative(repoRoot, attemptDirectory), '--run-token', 'round',
        ], path.join(attemptDirectory, 'round.log'), {cwd: repoRoot});
        const summaryPath = path.join(attemptDirectory, 'round', 'summary.json');
        const summary = JSON.parse(fs.readFileSync(summaryPath, 'utf8'));
        if (summary.discardReason) {
            return {status: 'discarded', reason: summary.discardReason, summaryPath};
        }
        return {status: 'completed', summaryPath, summary};
    } catch (error) {
        networkLogs(repoRoot, path.join(attemptDirectory, 'sut.log'));
        const summaryPath = path.join(attemptDirectory, 'round', 'summary.json');
        if (fs.existsSync(summaryPath)) {
            const summary = JSON.parse(fs.readFileSync(summaryPath, 'utf8'));
            return {status: 'discarded', reason: summary.discardReason ?? error.message, summaryPath, summary};
        }
        return {status: 'failed', reason: error.message};
    }
}

function writeIndex(repoRoot, indexPath, index) {
    validateDocument(repoRoot, 'fabric-series-index.schema.json', index);
    writeJSONAtomic(indexPath, index);
}

function attemptRecord(seriesDirectory, kind, repetition, tryNumber, attemptDirectory, startedAt, endedAt, result) {
    const record = {
        kind,
        repetition,
        try: tryNumber,
        status: result.status,
        startedAt,
        endedAt,
        directory: path.relative(seriesDirectory, attemptDirectory).split(path.sep).join('/'),
        resources: path.relative(seriesDirectory, path.join(attemptDirectory, 'resources.txt')).split(path.sep).join('/'),
    };
    if (result.reason) record.reason = result.reason;
    if (result.summaryPath) record.summary = path.relative(seriesDirectory, result.summaryPath).split(path.sep).join('/');
    return record;
}

async function runSeries(repoRoot, options, dependencies = {}) {
    const snapshot = (dependencies.readSnapshot ?? readSnapshot)(repoRoot, options.snapshotDirectory);
    const snapshotRelative = path.relative(repoRoot, snapshot.directory);
    if (snapshotRelative.startsWith('..') || path.isAbsolute(snapshotRelative)) {
        throw new Error('series snapshot must be stored inside the repository');
    }
    const created = (dependencies.createSeriesDirectory ?? createSeriesDirectory)(repoRoot, options);
    const matrix = dependencies.scenarioMatrix
        ?? (options.plan === 'full' ? fullScenarioMatrix() : trialScenarioMatrix());
    const scenarios = matrix.map((entry) => ({
        ...entry, status: 'pending', extended: false, attempts: [],
    }));
    const index = {
        $schema: SERIES_SCHEMA_ID,
        schemaVersion: '1.0.0',
        seriesToken: created.token,
        plan: options.plan,
        citable: options.plan === 'full',
        snapshot: {
            id: snapshot.manifest.snapshotId,
            directory: snapshotRelative.split(path.sep).join('/'),
            manifestSHA256: sha256File(snapshot.manifestPath),
        },
        startedAt: new Date().toISOString(),
        status: 'running',
        scenarios,
    };
    const indexPath = path.join(created.directory, 'series-index.json');
    writeIndex(repoRoot, indexPath, index);
    const execute = dependencies.executeAttempt ?? executeAttempt;
    const now = dependencies.now ?? (() => new Date().toISOString());
    const measuredRepetitions = options.plan === 'full' ? 5 : 2;

    for (const scenario of scenarios) {
        scenario.status = 'running';
        writeIndex(repoRoot, indexPath, index);
        let warmupSucceeded = false;
        for (let tryNumber = 1; tryNumber <= 2; tryNumber += 1) {
            const attemptDirectory = path.join(created.directory, scenario.key, 'warmup', `try-${tryNumber}`);
            fs.mkdirSync(path.dirname(attemptDirectory), {recursive: true});
            const startedAt = now();
            const result = await execute(repoRoot, {
                ...options, scenario, kind: 'warmup', repetition: 0, tryNumber, attemptDirectory,
            });
            scenario.attempts.push(attemptRecord(
                created.directory, 'warmup', 0, tryNumber, attemptDirectory, startedAt, now(), result,
            ));
            writeIndex(repoRoot, indexPath, index);
            if (result.status === 'completed') {
                warmupSucceeded = true;
                break;
            }
        }
        if (!warmupSucceeded) {
            scenario.status = 'blocked';
            scenario.reason = 'warm-up failed twice; measurements were not executed';
            writeIndex(repoRoot, indexPath, index);
            continue;
        }

        const summaries = [];
        async function measure(repetition) {
            const attemptDirectory = path.join(
                created.directory, scenario.key, 'measurements', `run-${String(repetition).padStart(2, '0')}`,
            );
            fs.mkdirSync(path.dirname(attemptDirectory), {recursive: true});
            const startedAt = now();
            const result = await execute(repoRoot, {
                ...options, scenario, kind: 'measurement', repetition, tryNumber: 1, attemptDirectory,
            });
            scenario.attempts.push(attemptRecord(
                created.directory, 'measurement', repetition, 1, attemptDirectory, startedAt, now(), result,
            ));
            if (result.status === 'completed') summaries.push(result.summary);
            writeIndex(repoRoot, indexPath, index);
            return result.status === 'completed';
        }
        let complete = true;
        for (let repetition = 1; repetition <= measuredRepetitions; repetition += 1) {
            if (!await measure(repetition)) complete = false;
        }
        if (complete) {
            scenario.throughputCV = coefficientOfVariation(
                summaries.map((summary) => summary.rate.effectiveOperationsPerSecond),
            );
            scenario.p95CV = coefficientOfVariation(summaries.map((summary) => summary.operationLatency.p95Ms));
            if (options.plan === 'full' && (scenario.throughputCV > 0.15 || scenario.p95CV > 0.15)) {
                scenario.extended = true;
                for (let repetition = 6; repetition <= 8; repetition += 1) {
                    if (!await measure(repetition)) complete = false;
                }
            }
        }
        scenario.status = complete ? 'completed' : 'incomplete';
        if (!complete) scenario.reason = 'one or more measured repetitions were discarded or failed';
        writeIndex(repoRoot, indexPath, index);
    }
    index.endedAt = now();
    index.status = scenarios.every((scenario) => scenario.status === 'completed') ? 'completed' : 'incomplete';
    writeIndex(repoRoot, indexPath, index);
    return {directory: created.directory, index};
}

async function main() {
    const repoRoot = path.resolve(__dirname, '..', '..', '..');
    const result = await runSeries(repoRoot, parseArguments(process.argv.slice(2)));
    process.stdout.write(`Fabric series ${result.index.status}: ${result.directory}\n`);
    if (result.index.status !== 'completed') process.exitCode = 1;
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write(`Fabric series failed: ${error.message}\n`);
        process.exitCode = 1;
    });
}

module.exports = {
    attemptRecord,
    coefficientOfVariation,
    createSeriesDirectory,
    executeAttempt,
    fullScenarioMatrix,
    main,
    parseArguments,
    runSeries,
    scenarioArguments,
    trialScenarioMatrix,
    writeIndex,
};
