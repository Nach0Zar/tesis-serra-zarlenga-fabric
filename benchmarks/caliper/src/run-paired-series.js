#!/usr/bin/env node
'use strict';
const fs = require('node:fs');
const path = require('node:path');
const {setImmediate: yieldEventLoop} = require('node:timers/promises');
const {readSnapshot, sha256File, validateDocument} = require('./artifacts');
const {writeJSONAtomic} = require('./metadata');
const {attemptRecord, coefficientOfVariation, initializeSeries, writeIndex} = require('./series-state');
const {executeAttempt, fullScenarioMatrix, trialScenarioMatrix} = require('./run-series');
const {attemptExecutor, baselineSnapshot} = require('../../baseline/src/run-series');
const {benchmarkEnvironment, compose: baselineCompose} = require('../../baseline/src/runtime');
const {CHAINCODE_CONTAINER} = require('./snapshot-volumes');
const {assertSnapshotCompatibility} = require('./snapshot');
const {command, guardAttempt} = require('./host-guard');
const {repetitionOrder, extensionTriggers} = require('./series-policy');

const SCHEMA = 'urn:pfi-snt:paired-campaign:schema:1.0.0';
const SUTS = ['fabric', 'baseline'];

function parseArguments(args) {
    const values = {};
    const allowed = new Set(['plan', 'fabric-snapshot-dir', 'baseline-snapshot-dir', 'dataset-dir', 'campaign-token']);
    for (let index = 0; index < args.length; index += 2) {
        const key = args[index]?.slice(2);
        if (!args[index]?.startsWith('--') || !allowed.has(key) || !args[index + 1]
            || args[index + 1].startsWith('--') || values[key] !== undefined) throw new Error('invalid paired campaign arguments');
        values[key] = args[index + 1];
    }
    if (!['trial', 'full'].includes(values.plan) || !values['fabric-snapshot-dir'] || !values['baseline-snapshot-dir']
        || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$/u.test(values['campaign-token'] ?? '')) {
        throw new Error('plan, both snapshot directories and a safe campaign token are required');
    }
    return {plan: values.plan, fabricSnapshotDirectory: values['fabric-snapshot-dir'],
        baselineSnapshotDirectory: values['baseline-snapshot-dir'], datasetDirectory: values['dataset-dir'],
        campaignToken: values['campaign-token']};
}
function stopFabric(root) {
    command('docker', ['compose', '-f', path.join(root, 'network/compose.yaml'), 'stop'], root);
    const containers = command('docker', ['ps', '--format', '{{.ID}} {{.Names}}'], root);
    for (const line of containers.split('\n').filter(Boolean)) {
        const [id, name] = line.split(' ');
        if (CHAINCODE_CONTAINER.test(name)) command('docker', ['stop', id], root);
    }
    const remaining = command('docker', ['compose', '-f', path.join(root, 'network/compose.yaml'),
        'ps', '--status', 'running', '-q'], root);
    const chaincodes = command('docker', ['ps', '--format', '{{.Names}}'], root).split('\n');
    if (remaining || chaincodes.some((name) => CHAINCODE_CONTAINER.test(name))) throw new Error('Fabric did not stop');
}
function stopBaseline(root) {
    const env = benchmarkEnvironment();
    baselineCompose(root, ['stop'], {env});
    const remaining = baselineCompose(root, ['ps', '--status', 'running', '-q'], {env}).stdout.trim();
    if (remaining) throw new Error('baseline did not stop');
}
function handoff(root, sut) {
    if (sut === 'fabric') stopBaseline(root);
    else stopFabric(root);
}
function loadSnapshots(root, options, commit) {
    const fabric = readSnapshot(root, options.fabricSnapshotDirectory);
    assertSnapshotCompatibility(root, fabric);
    const baseline = baselineSnapshot(root, options.baselineSnapshotDirectory);
    if (baseline.manifest.artifacts.baselineCommit !== commit
        || fabric.manifest.dataset.sha256 !== baseline.manifest.dataset.sha256
        || fabric.manifest.files.logicalPlan.sha256 !== baseline.manifest.logicalPlanSHA256) {
        throw new Error('snapshot artifact, dataset or logical plan mismatch');
    }
    for (const snapshot of [fabric, baseline]) {
        const relative = path.relative(root, snapshot.directory);
        if (relative.startsWith('..') || path.isAbsolute(relative)) throw new Error('snapshot must be inside repository');
    }
    return {fabric, baseline};
}
async function runPaired(root, options, dependencies = {}) {
    const commit = (dependencies.commit ?? (() => command('git', ['rev-parse', 'HEAD'], root)))();
    const snapshots = (dependencies.snapshots ?? loadSnapshots)(root, options, commit);
    const parent = path.join(root, 'build/benchmarks/paired');
    fs.mkdirSync(parent, {recursive: true});
    const lockPath = path.join(parent, '.active.lock');
    const lock = fs.openSync(lockPath, 'wx', 0o600);
    fs.writeFileSync(lock, JSON.stringify({pid: process.pid, campaignToken: options.campaignToken, commit}));
    const directory = path.join(parent, options.campaignToken);
    const states = {};
    const now = dependencies.now ?? (() => new Date().toISOString());
    const startedAt = now();
    const matrix = dependencies.matrix ?? (options.plan === 'full' ? fullScenarioMatrix() : trialScenarioMatrix());
    const campaign = {$schema: SCHEMA, schemaVersion: '1.0.0', campaignToken: options.campaignToken,
        plan: options.plan, repositoryCommit: commit, startedAt, status: 'running',
        snapshots: {}, series: {}, attempts: [], extensions: []};
    const manifestPath = path.join(directory, 'campaign.json');
    let created = false;
    let interrupted = false;
    const interrupt = () => { interrupted = true; };
    const save = () => {
        validateDocument(root, 'paired-campaign.schema.json', campaign);
        writeJSONAtomic(manifestPath, campaign);
        for (const state of Object.values(states)) writeIndex(root, state.indexPath, state.index);
    };
    try {
        fs.mkdirSync(directory, {mode: 0o700});
        created = true;
        for (const sut of SUTS) {
            const seriesDirectory = path.join(directory, sut);
            fs.mkdirSync(seriesDirectory);
            states[sut] = {...initializeSeries(root, seriesDirectory, options.campaignToken + '-' + sut,
                options.plan, options.plan === 'full', snapshots[sut], matrix, startedAt), directory: seriesDirectory};
            campaign.snapshots[sut] = states[sut].index.snapshot;
            campaign.series[sut] = sut + '/series-index.json';
        }
        save();
        process.on('SIGINT', interrupt);
        process.on('SIGTERM', interrupt);
        const executors = dependencies.executors ?? {fabric: executeAttempt, baseline: attemptExecutor()};
        const guard = dependencies.guard ?? guardAttempt;
        const switchTo = dependencies.handoff ?? handoff;
        async function attempt(sut, scenarioIndex, kind, repetition, tryNumber) {
            await yieldEventLoop();
            if (interrupted) throw new Error('campaign interrupted');
            const state = states[sut];
            const scenario = state.scenarios[scenarioIndex];
            const local = kind === 'warmup' ? ['warmup', 'try-' + tryNumber]
                : ['measurements', 'run-' + String(repetition).padStart(2, '0')];
            const attemptDirectory = path.join(state.directory, scenario.key, ...local);
            fs.mkdirSync(path.dirname(attemptDirectory), {recursive: true});
            const start = now();
            let result;
            try {
                await guard(root, commit);
                await switchTo(root, sut);
                result = await executors[sut](root, {snapshotDirectory: snapshots[sut].directory,
                    datasetDirectory: options.datasetDirectory, scenario, kind, repetition, tryNumber, attemptDirectory});
            } catch (error) {
                result = {status: 'failed', reason: error.message};
            }
            const record = attemptRecord(state.directory, kind, repetition, tryNumber, attemptDirectory, start, now(), result);
            scenario.attempts.push(record);
            campaign.attempts.push({sut, scenarioKey: scenario.key, ...record,
                directory: path.relative(directory, attemptDirectory).split(path.sep).join('/')});
            save();
            process.stdout.write(sut + ' ' + scenario.key + ' ' + kind + '/' + repetition + ': ' + result.status + '\n');
            if (interrupted) throw new Error('campaign interrupted');
            return result;
        }
        for (let index = 0; index < matrix.length; index += 1) {
            for (const sut of SUTS) states[sut].scenarios[index].status = 'running';
            save();
            for (const sut of SUTS) {
                let warmed = false;
                for (let tryNumber = 1; tryNumber <= 2; tryNumber += 1) {
                    if ((await attempt(sut, index, 'warmup', 0, tryNumber)).status === 'completed') { warmed = true; break; }
                }
                if (!warmed) {
                    states[sut].scenarios[index].status = 'blocked';
                    states[sut].scenarios[index].reason = 'warm-up failed twice';
                    throw new Error(sut + ' warm-up failed twice');
                }
            }
            const summaries = {fabric: [], baseline: []};
            async function measure(repetition) {
                for (const sut of repetitionOrder(repetition)) {
                    const result = await attempt(sut, index, 'measurement', repetition, 1);
                    if (result.status !== 'completed') throw new Error(sut + ' measured repetition failed or discarded');
                    summaries[sut].push(result.summary);
                }
            }
            const count = options.plan === 'full' ? 5 : 2;
            for (let repetition = 1; repetition <= count; repetition += 1) await measure(repetition);
            for (const sut of SUTS) {
                const scenario = states[sut].scenarios[index];
                scenario.throughputCV = coefficientOfVariation(summaries[sut].map((summary) => summary.rate.effectiveOperationsPerSecond));
                scenario.p95CV = coefficientOfVariation(summaries[sut].map((summary) => summary.operationLatency.p95Ms));
            }
            const triggers = options.plan === 'full' ? extensionTriggers(summaries) : [];
            if (triggers.length) {
                campaign.extensions.push({scenarioKey: matrix[index].key, triggers});
                for (const sut of SUTS) states[sut].scenarios[index].extended = true;
                save();
                for (let repetition = 6; repetition <= 8; repetition += 1) await measure(repetition);
            }
            for (const sut of SUTS) states[sut].scenarios[index].status = 'completed';
            save();
        }
        campaign.status = 'completed';
    } catch (error) {
        if (!created) throw error;
        campaign.status = 'failed';
        campaign.reason = error.message;
        for (const state of Object.values(states)) {
            for (const scenario of state.scenarios) {
                if (scenario.status === 'running') { scenario.status = 'incomplete'; scenario.reason = error.message; }
            }
        }
    } finally {
        if (created) {
            try { await (dependencies.stop ?? (async () => { stopFabric(root); stopBaseline(root); }))(); }
            catch (error) { campaign.status = 'failed'; campaign.reason = [campaign.reason, error.message].filter(Boolean).join('; '); }
            campaign.endedAt = now();
            for (const state of Object.values(states)) {
                state.index.status = campaign.status === 'completed' ? 'completed' : 'incomplete';
                state.index.endedAt = campaign.endedAt;
            }
            if (Object.keys(states).length === 2) save();
        }
        process.removeListener('SIGINT', interrupt);
        process.removeListener('SIGTERM', interrupt);
        fs.closeSync(lock);
        fs.unlinkSync(lockPath);
    }
    return {directory, campaign};
}
async function main() {
    const root = path.resolve(__dirname, '../../..');
    const result = await runPaired(root, parseArguments(process.argv.slice(2)));
    process.stdout.write('Paired campaign ' + result.campaign.status + ': ' + result.directory + '\n');
    if (result.campaign.status !== 'completed') process.exitCode = 1;
}
if (require.main === module) main().catch((error) => { process.stderr.write(error.message + '\n'); process.exitCode = 1; });
module.exports = {extensionTriggers, handoff, loadSnapshots, parseArguments, repetitionOrder, runPaired, stopFabric, stopBaseline};
