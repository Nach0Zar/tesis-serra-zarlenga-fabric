'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {runPaired, parseArguments, extensionTriggers} = require('../src/run-paired-series');
const {coefficientOfVariation} = require('../src/series-policy');

const actualRoot = path.resolve(__dirname, '../../..');
function fixture(t, plan = 'full', variable = false, failure) {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'paired-series-'));
    t.after(() => fs.rmSync(root, {recursive: true, force: true}));
    fs.mkdirSync(path.join(root, 'benchmarks/schema'), {recursive: true});
    for (const name of ['paired-campaign.schema.json', 'fabric-series-index.schema.json']) {
        fs.copyFileSync(path.join(actualRoot, 'benchmarks/schema', name), path.join(root, 'benchmarks/schema', name));
    }
    const snapshots = {};
    for (const sut of ['fabric', 'baseline']) {
        const directory = path.join(root, 'build/snapshots', sut);
        fs.mkdirSync(directory, {recursive: true});
        const manifestPath = path.join(directory, 'manifest.json');
        fs.writeFileSync(manifestPath, '{}');
        snapshots[sut] = {directory, manifestPath, manifest: {snapshotId: (sut === 'fabric' ? 'a' : 'b').repeat(64)}};
    }
    const events = [];
    let tick = 0;
    const options = {plan, campaignToken: 'test', fabricSnapshotDirectory: 'unused', baselineSnapshotDirectory: 'unused'};
    const executor = (sut) => async (repoRoot, attempt) => {
        events.push([sut, attempt.kind, attempt.repetition, attempt.tryNumber]);
        fs.mkdirSync(path.join(attempt.attemptDirectory, 'round'), {recursive: true});
        const status = failure?.(sut, attempt) ?? 'completed';
        const summary = {rate: {effectiveOperationsPerSecond: variable && sut === 'baseline' && attempt.repetition === 5 ? 50 : 10},
            operationLatency: {p95Ms: 10}};
        const summaryPath = path.join(attempt.attemptDirectory, 'round/summary.json');
        fs.writeFileSync(summaryPath, JSON.stringify(summary));
        return {status, summary, summaryPath, ...(status === 'completed' ? {} : {reason: 'injected failure'})};
    };
    const dependencies = {commit: () => 'c'.repeat(40), snapshots: () => snapshots,
        matrix: [{key: 'read-unit-50', scenario: 'read-unit', rate: 50}],
        now: () => new Date(1_800_000_000_000 + tick++ * 1000).toISOString(),
        guard: async () => {}, handoff: async () => {}, stop: async () => {},
        executors: {fabric: executor('fabric'), baseline: executor('baseline')}};
    return {root, options, dependencies, events};
}
test('five paired repetitions alternate and do not extend stable systems', async (t) => {
    const fixture_ = fixture(t);
    const result = await runPaired(fixture_.root, fixture_.options, fixture_.dependencies);
    assert.equal(result.campaign.status, 'completed');
    assert.deepEqual(fixture_.events, [
        ['fabric', 'warmup', 0, 1], ['baseline', 'warmup', 0, 1],
        ...[1, 2, 3, 4, 5].flatMap((number) => (number % 2 ? ['fabric', 'baseline'] : ['baseline', 'fabric'])
            .map((sut) => [sut, 'measurement', number, 1])),
    ]);
    assert.deepEqual(result.campaign.extensions, []);
    await assert.rejects(runPaired(fixture_.root, fixture_.options, fixture_.dependencies), /exist/u);
});
test('one variable system extends both to eight and records the actual trigger', async (t) => {
    const fixture_ = fixture(t, 'full', true);
    const result = await runPaired(fixture_.root, fixture_.options, fixture_.dependencies);
    assert.equal(result.campaign.status, 'completed');
    assert.equal(fixture_.events.length, 18);
    assert.deepEqual(result.campaign.extensions[0].triggers.map(({sut, metric}) => [sut, metric]), [['baseline', 'throughput']]);
    for (const sut of ['fabric', 'baseline']) {
        const index = JSON.parse(fs.readFileSync(path.join(result.directory, sut, 'series-index.json')));
        assert.equal(index.scenarios[0].extended, true);
        assert.equal(index.scenarios[0].attempts.filter((attempt) => attempt.kind === 'measurement').length, 8);
    }
});
test('trial remains noncitable and does not extend', async (t) => {
    const fixture_ = fixture(t, 'trial', true);
    const result = await runPaired(fixture_.root, fixture_.options, fixture_.dependencies);
    assert.equal(result.campaign.status, 'completed');
    assert.equal(result.campaign.attempts.length, 6);
    assert.equal(JSON.parse(fs.readFileSync(path.join(result.directory, 'fabric/series-index.json'))).citable, false);
});
test('warm-up retries once and second failure blocks all measurements', async (t) => {
    const fixture_ = fixture(t, 'full', false, (sut, attempt) => sut === 'fabric' ? 'failed' : 'completed');
    const result = await runPaired(fixture_.root, fixture_.options, fixture_.dependencies);
    assert.equal(result.campaign.status, 'failed');
    assert.equal(fixture_.events.length, 2);
    const index = JSON.parse(fs.readFileSync(path.join(result.directory, 'fabric/series-index.json')));
    assert.equal(index.scenarios[0].status, 'blocked');
});
test('a failed first warm-up can recover on its only retry', async (t) => {
    const fixture_ = fixture(t, 'trial', false, (sut, attempt) =>
        sut === 'fabric' && attempt.kind === 'warmup' && attempt.tryNumber === 1 ? 'failed' : 'completed');
    const result = await runPaired(fixture_.root, fixture_.options, fixture_.dependencies);
    assert.equal(result.campaign.status, 'completed');
    assert.equal(fixture_.events.length, 7);
});
test('discarded measurement is preserved and stops without replacement', async (t) => {
    const fixture_ = fixture(t, 'full', false, (sut, attempt) => attempt.kind === 'measurement' ? 'discarded' : 'completed');
    const result = await runPaired(fixture_.root, fixture_.options, fixture_.dependencies);
    assert.equal(result.campaign.status, 'failed');
    assert.equal(fixture_.events.length, 3);
    assert.equal(result.campaign.attempts.at(-1).status, 'discarded');
});
test('global lock excludes a second controller without stealing the lock', async (t) => {
    const fixture_ = fixture(t);
    const parent = path.join(fixture_.root, 'build/benchmarks/paired');
    fs.mkdirSync(parent, {recursive: true});
    fs.writeFileSync(path.join(parent, '.active.lock'), 'other controller');
    await assert.rejects(runPaired(fixture_.root, fixture_.options, fixture_.dependencies), /exist/u);
    assert.equal(fs.readFileSync(path.join(parent, '.active.lock'), 'utf8'), 'other controller');
});
test('strict threshold and argument validation preserve protocol', () => {
    assert.equal(coefficientOfVariation([85, 115]), 0.15);
    const summary = (value) => ({rate: {effectiveOperationsPerSecond: value}, operationLatency: {p95Ms: value}});
    assert.deepEqual(extensionTriggers({fabric: [summary(85), summary(115)], baseline: [summary(100), summary(100)]}), []);
    assert.throws(() => parseArguments(['--plan', 'availability']), /required/u);
    assert.throws(() => parseArguments(['--plan', 'trial', '--plan', 'full']), /invalid/u);
    assert.throws(() => parseArguments(['--plan', 'full', '--fabric-snapshot-dir', 'x',
        '--baseline-snapshot-dir', 'y', '--campaign-token', '../escape']), /required/u);
});
