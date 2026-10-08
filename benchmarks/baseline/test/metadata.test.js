'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const {buildProfile} = require('../../caliper/src/profiles');
const {buildRoundMetadata, buildSmokeMetadata} = require('../src/metadata');

function context() {
    return {
        repositoryCommit: 'a'.repeat(40),
        dataset: {seed: 20260727, sha256: 'b'.repeat(64), units: 50000},
        host: {cpu: 'CPU', cpuCores: 2, memoryGB: 4, os: 'OS', kernel: 'kernel'},
        environment: {
            docker: '29', dockerCompose: '5', baselineCommit: 'c'.repeat(40),
            baselineImage: `snt-baseline@sha256:${'d'.repeat(64)}`, postgres: '16.15',
        },
    };
}

test('round metadata preserves baseline-only environment and transfer pair rates', () => {
    const profile = buildProfile({scenario: 'write-transfer', phase: 'measurement', repetition: '1', rate: '5'});
    const metadata = buildRoundMetadata({
        ...context(), profile,
        startedAt: '2026-10-08T00:00:00.000Z', endedAt: '2026-10-08T00:02:00.000Z',
    }, {rate: {effectiveTransactionsPerSecond: 10}});
    assert.equal(metadata.rate.targetOperationsPerSecond, 5);
    assert.equal(metadata.rate.targetTransactionsPerSecond, 10);
    assert.equal(metadata.environment.contractVersion, undefined);
    assert.equal(metadata.environment.packageID, undefined);
});

test('discard and expected rejection fields are represented in metadata', () => {
    const profile = buildProfile({
        scenario: 'expected-rejections', phase: 'warmup', repetition: '0', rate: '5',
        family: 'BLOCKING_STATE', operation: 'dispense',
    });
    const metadata = buildRoundMetadata({
        ...context(), profile,
        startedAt: '2026-10-08T00:00:00.000Z', endedAt: '2026-10-08T00:01:00.000Z',
    }, {rate: {effectiveTransactionsPerSecond: 4.9}, discardReason: 'unexpected success'});
    assert.equal(metadata.rejectionFamily, 'BLOCKING_STATE');
    assert.equal(metadata.discarded.reason, 'unexpected success');
});

test('smoke metadata records exactly thirty reads', () => {
    const metadata = buildSmokeMetadata(context(), {
        startedAt: '2026-10-08T00:00:00.000Z', endedAt: '2026-10-08T00:00:30.000Z',
        observedDurationSeconds: 30, successfulTransactions: 30,
    });
    assert.equal(metadata.transactions, 30);
    assert.equal(metadata.rate.operation, 'query-unit');
    assert.equal(metadata.rate.effectiveTransactionsPerSecond, 1);
});
