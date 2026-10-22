#!/usr/bin/env node
'use strict';

// Series reproducibles de la baseline con el mismo indice que Fabric
// (benchmarks/schema/fabric-series-index.schema.json). Reutiliza el
// orquestador de Caliper: warm-up con un reintento, mediciones, extension por CV
// en `full` y el indice estricto. Cada intento es una ronda de la baseline, que
// ya restaura su snapshot, verifica las precondiciones y corre su smoke. Sin
// este indice, el procesamiento no puede acreditar rondas de la baseline como
// citables (#47).

const fs = require('node:fs');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

// El orquestador de Caliper valida el indice con ajv, que Node resuelve desde
// benchmarks/caliper/node_modules: instalarlo sólo en la baseline no alcanza.
function requireCaliperDependencies(resolve = require.resolve) {
    const caliperSource = path.resolve(__dirname, '..', '..', 'caliper', 'src');
    try {
        resolve('ajv/dist/2020', {paths: [caliperSource]});
    } catch {
        throw new Error('missing shared Caliper dependencies; run `npm ci --prefix benchmarks/caliper` from the repository root');
    }
}

try {
    requireCaliperDependencies();
} catch (error) {
    if (require.main !== module) throw error;
    process.stderr.write(`Baseline series failed: ${error.message}\n`);
    process.exit(1);
}

const {sha256File} = require('../../caliper/src/sources');
const {PLANS, createSeriesDirectory, fullScenarioMatrix, runSeries, trialScenarioMatrix} = require('../../caliper/src/run-series');
const {pairs} = require('./arguments');
const {repoRoot} = require('./paths');
const {SCENARIOS: AVAILABILITY_SCENARIOS} = require('./run-availability');
const {readSnapshot} = require('./snapshot');

function scenarioMatrix(plan) {
    if (plan === 'full') return fullScenarioMatrix();
    if (plan === 'trial') return trialScenarioMatrix();
    return Object.keys(AVAILABILITY_SCENARIOS).map((scenario) => ({key: scenario, scenario, rate: 20}));
}

function parseArguments(arguments_) {
    const values = pairs(arguments_);
    for (const key of Object.keys(values)) {
        if (!['snapshot-dir', 'dataset-dir', 'plan', 'series-token', 'output-root'].includes(key)) {
            throw new Error(`unsupported argument --${key}`);
        }
    }
    if (!values['snapshot-dir']) throw new Error('--snapshot-dir is required');
    if (!PLANS[values.plan]) throw new Error(`--plan must be one of ${Object.keys(PLANS).join(', ')}`);
    return {
        snapshotDirectory: values['snapshot-dir'],
        datasetDirectory: values['dataset-dir'],
        plan: values.plan,
        seriesToken: values['series-token'],
        outputRoot: values['output-root'] ?? path.join('build', 'benchmarks', 'baseline', 'series'),
    };
}

function roundArguments(scenario, phase, repetition) {
    const args = ['--scenario', scenario.scenario, '--phase', phase, '--repetition', String(repetition)];
    if (AVAILABILITY_SCENARIOS[scenario.scenario]) return args;
    args.push('--rate', String(scenario.rate));
    if (scenario.family) args.push('--family', scenario.family);
    if (scenario.operation) args.push('--operation', scenario.operation);
    return args;
}

function attemptExecutor(options = {}) {
    const spawn = options.spawnSync ?? spawnSync;
    return async (root, attempt) => {
        fs.mkdirSync(attempt.attemptDirectory, {recursive: false, mode: 0o700});
        const availability = Boolean(AVAILABILITY_SCENARIOS[attempt.scenario.scenario]);
        const script = path.join(root, 'benchmarks', 'baseline', 'src', availability ? 'run-availability.js' : 'run-round.js');
        const args = [
            script,
            ...roundArguments(attempt.scenario, attempt.kind, attempt.repetition),
            '--snapshot-dir', attempt.snapshotDirectory,
            '--output-root', path.relative(root, attempt.attemptDirectory),
            '--run-token', 'round',
        ];
        if (attempt.datasetDirectory) args.push('--dataset-dir', attempt.datasetDirectory);
        const result = spawn(process.execPath, args, {
            cwd: root, env: process.env, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], maxBuffer: 128 * 1024 * 1024,
        });
        fs.writeFileSync(path.join(attempt.attemptDirectory, 'round.log'), `${result.stdout ?? ''}${result.stderr ?? ''}`, {mode: 0o600});
        // La ronda de la baseline registra los recursos del host en su
        // run-context.json; no hay un resources.txt aparte como en Fabric.
        const resourcesPath = path.join(attempt.attemptDirectory, 'round', 'run-context.json');
        const summaryPath = path.join(attempt.attemptDirectory, 'round', 'summary.json');
        if (!fs.existsSync(summaryPath)) {
            return {status: 'failed', reason: result.error?.message ?? `round exited with code ${result.status} before writing summary.json`, resourcesPath};
        }
        const summary = JSON.parse(fs.readFileSync(summaryPath, 'utf8'));
        if (summary.discardReason || result.status !== 0) {
            return {status: 'discarded', reason: summary.discardReason ?? `round exited with code ${result.status}`, summaryPath, summary, resourcesPath};
        }
        return {status: 'completed', summaryPath, summary, resourcesPath};
    };
}

function baselineSnapshot(root, snapshotDirectory) {
    const snapshot = readSnapshot(path.resolve(root, snapshotDirectory));
    const manifestPath = path.join(snapshot.directory, 'manifest.json');
    return {directory: snapshot.directory, manifestPath, manifest: snapshot.manifest, manifestSHA256: sha256File(manifestPath)};
}

async function main() {
    const options = parseArguments(process.argv.slice(2));
    const result = await runSeries(repoRoot, options, {
        readSnapshot: baselineSnapshot,
        createSeriesDirectory,
        scenarioMatrix: scenarioMatrix(options.plan),
        executeAttempt: attemptExecutor(),
    });
    process.stdout.write(`Baseline series ${result.index.status}: ${result.directory}\n`);
    if (result.index.status !== 'completed') process.exitCode = 1;
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write(`Baseline series failed: ${error.message}\n`);
        process.exitCode = 1;
    });
}

module.exports = {attemptExecutor, baselineSnapshot, parseArguments, requireCaliperDependencies, roundArguments, scenarioMatrix};
