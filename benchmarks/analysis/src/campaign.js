'use strict';
const fs = require('node:fs');
const path = require('node:path');
const Ajv2020 = require('ajv/dist/2020');
const {fullScenarioMatrix, trialScenarioMatrix, repetitionOrder, extensionTriggers} = require('../../caliper/src/series-policy');

const SCHEMAS = path.resolve(__dirname, '../../schema');
const ajv = new Ajv2020({allErrors: true, strict: true});
const validate = ajv.compile(JSON.parse(fs.readFileSync(path.join(SCHEMAS, 'paired-campaign.schema.json'), 'utf8')));
const seriesValidator = ajv.compile(JSON.parse(fs.readFileSync(path.join(SCHEMAS, 'fabric-series-index.schema.json'), 'utf8')));
const SUTS = ['fabric', 'baseline'];
const {buildProfile} = require('../../caliper/src/profiles');
const {coefficientOfVariation} = require('../../caliper/src/series-policy');
function inside(root, relative) {
    const result = path.resolve(root, relative);
    const relation = path.relative(root, result);
    if (path.isAbsolute(relative) || relation.startsWith('..') || path.isAbsolute(relation)) throw new Error('campaign path escapes its root');
    return result;
}
function same(left, right, label) {
    if (JSON.stringify(left) !== JSON.stringify(right)) throw new Error('campaign mismatch: ' + label);
}
function validateCampaign(file, runs, options = {}) {
    const directory = path.dirname(path.resolve(file));
    const campaign = JSON.parse(fs.readFileSync(file, 'utf8'));
    if (!validate(campaign)) throw new Error('invalid campaign schema: ' + ajv.errorsText(validate.errors));
    if (campaign.status !== 'completed') throw new Error('campaign is not completed');
    if (campaign.plan === 'trial' && !options.allowNonCitable) throw new Error('trial campaign is not citable');
    const matrix = campaign.plan === 'full' ? fullScenarioMatrix() : trialScenarioMatrix();
    const states = {};
    for (const sut of SUTS) {
        const indexPath = inside(directory, campaign.series[sut]);
        const index = JSON.parse(fs.readFileSync(indexPath, 'utf8'));
        if (!seriesValidator(index)) throw new Error('invalid campaign series index');
        if (index.status !== 'completed' || index.plan !== campaign.plan || index.citable !== (campaign.plan === 'full')) {
            throw new Error('campaign series status or plan mismatch');
        }
        same(index.snapshot, campaign.snapshots[sut], 'snapshot reference');
        same(index.scenarios.map(({key, scenario, rate, family, operation}) =>
            JSON.parse(JSON.stringify({key, scenario, rate, family, operation}))), matrix, 'scenario matrix');
        states[sut] = {index, root: path.dirname(indexPath)};
    }
    let cursor = 0;
    let previous = campaign.startedAt;
    const usedRuns = new Set();
    let host;
    const extensionRows = [];
    const consume = (sut, scenario, kind, repetition, tryNumber) => {
        const attempt = scenario.attempts.find((entry) => entry.kind === kind
            && entry.repetition === repetition && entry.try === tryNumber);
        if (!attempt) throw new Error('missing paired campaign attempt');
        const ordered = campaign.attempts[cursor++];
        if (!ordered || ordered.sut !== sut || ordered.scenarioKey !== scenario.key) throw new Error('invalid campaign alternation');
        same({...ordered, sut: undefined, scenarioKey: undefined,
            directory: path.relative(states[sut].root, inside(directory, ordered.directory)).split(path.sep).join('/')},
        {...attempt, sut: undefined, scenarioKey: undefined}, 'ordered attempt');
        if (!Number.isFinite(Date.parse(attempt.startedAt)) || !Number.isFinite(Date.parse(attempt.endedAt))
            || Date.parse(attempt.startedAt) < Date.parse(previous)
            || Date.parse(attempt.endedAt) < Date.parse(attempt.startedAt)) throw new Error('campaign attempts overlap or have invalid times');
        previous = attempt.endedAt;
        if (attempt.status !== 'completed') {
            if (kind === 'measurement') throw new Error('invalid measured campaign attempt');
            return undefined;
        }
        const attemptDirectory = inside(states[sut].root, attempt.directory);
        const run = runs.find((entry) => path.resolve(entry.directory) === path.join(attemptDirectory, 'round'));
        if (!run || run.sut !== sut || run.repetition !== repetition || run.phase !== kind
            || run.repositoryCommit !== campaign.repositoryCommit) throw new Error('campaign attempt has inconsistent or missing run');
        const profile = buildProfile({...scenario, phase: kind, repetition});
        same([run.scenario, run.targetOperationsPerSecond, run.operation, run.workers, run.durationSeconds,
            run.transactionsPerOperation, run.mix ?? null],
            [profile.scenario, profile.rate, profile.operation, profile.workers, profile.durationSeconds,
                profile.transactionsPerOperation, profile.mix ?? null], 'workload profile');
        if (!run.host) throw new Error('campaign run omits host');
        if (!host) host = run.host;
        same(run.host, host, 'physical host');
        if (Date.parse(run.startedAt) < Date.parse(attempt.startedAt)
            || Date.parse(run.endedAt) > Date.parse(attempt.endedAt)
            || !Number.isFinite(Date.parse(run.startedAt)) || !Number.isFinite(Date.parse(run.endedAt))) {
            throw new Error('workload timing lies outside its campaign attempt');
        }
        if (kind === 'measurement') {
            if (!run.admission?.ok || run.discardReason || run.exclusion) throw new Error('campaign measurement is not accredited');
            if (sut === 'fabric' && (!run.participationMarkers
                || run.participationMarkers.expected !== run.participationMarkers.observed)) {
                throw new Error('campaign Fabric markers missing or inconsistent');
            }
            usedRuns.add(run);
        }
        if (!attempt.summary) throw new Error('campaign attempt omits summary');
        const summaryPath = inside(states[sut].root, attempt.summary);
        if (path.dirname(summaryPath) !== path.resolve(run.directory)) throw new Error('campaign summary path mismatch');
        const summary = JSON.parse(fs.readFileSync(summaryPath, 'utf8'));
        if (summary.discardReason) throw new Error('completed campaign attempt has discarded summary');
        return summary;
    };
    for (let index = 0; index < matrix.length; index += 1) {
        const scenarios = Object.fromEntries(SUTS.map((sut) => [sut, states[sut].index.scenarios[index]]));
        for (const sut of SUTS) {
            if (scenarios[sut].status !== 'completed') throw new Error('campaign scenario is incomplete');
            const warmups = scenarios[sut].attempts.filter((entry) => entry.kind === 'warmup');
            if (warmups.length < 1 || warmups.length > 2 || warmups.at(-1).status !== 'completed'
                || (warmups.length === 2 && warmups[0].status === 'completed')) throw new Error('invalid campaign warm-up policy');
            warmups.forEach((entry, offset) => consume(sut, scenarios[sut], 'warmup', 0, offset + 1));
        }
        const summaries = {fabric: [], baseline: []};
        const count = campaign.plan === 'full' ? 5 : 2;
        for (let repetition = 1; repetition <= count; repetition += 1) {
            for (const sut of repetitionOrder(repetition)) {
                summaries[sut].push(consume(sut, scenarios[sut], 'measurement', repetition, 1));
            }
        }
        for (const sut of SUTS) {
            same(scenarios[sut].throughputCV, coefficientOfVariation(
                summaries[sut].map((summary) => summary.rate.effectiveOperationsPerSecond)), 'throughput CV');
            same(scenarios[sut].p95CV, coefficientOfVariation(
                summaries[sut].map((summary) => summary.operationLatency.p95Ms)), 'p95 CV');
        }
        const triggers = campaign.plan === 'full' ? extensionTriggers(summaries) : [];
        const recorded = campaign.extensions.filter((entry) => entry.scenarioKey === matrix[index].key);
        same(recorded, triggers.length ? [{scenarioKey: matrix[index].key, triggers}] : [], 'extension triggers');
        const extended = triggers.length > 0;
        for (const sut of SUTS) {
            if (scenarios[sut].extended !== extended) throw new Error('campaign extension must be paired');
            const measured = scenarios[sut].attempts.filter((entry) => entry.kind === 'measurement');
            if (measured.length !== (extended ? 8 : count)) throw new Error('campaign repetition count mismatch');
        }
        if (extended) {
            for (let repetition = 6; repetition <= 8; repetition += 1) {
                for (const sut of repetitionOrder(repetition)) consume(sut, scenarios[sut], 'measurement', repetition, 1);
            }
            for (const sut of SUTS) {
                const own = triggers.filter((entry) => entry.sut === sut);
                extensionRows.push({scenarioKey: matrix[index].key, sut,
                    reason: own.length ? 'variabilidad propia' : 'mantener pares con el otro SUT', triggers});
            }
        }
    }
    if (cursor !== campaign.attempts.length || Date.parse(campaign.endedAt) < Date.parse(previous)) {
        throw new Error('campaign contains extra attempts or ends before its last attempt');
    }
    const measuredRuns = runs.filter((run) => run.phase === 'measurement' && !['smoke', 'dataset-preparation'].includes(run.scenario));
    if (measuredRuns.length !== usedRuns.size || measuredRuns.some((run) => !usedRuns.has(run))) throw new Error('inputs contain unlisted campaign measurements');
    return {campaign, extensionRows};
}
module.exports = {inside, validateCampaign};
