'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const {
    coefficientOfVariation, defaultPlan, needsExtension, parseArguments, roundMetrics, scenarioKey,
} = require('../src/run-series');

test('default series plan covers the 20 scenario-rate combinations of protocol section 6', () => {
    const plan = defaultPlan();
    assert.equal(plan.length, 20);
    assert.equal(new Set(plan.map(scenarioKey)).size, 20);
    assert.deepEqual(plan.filter((spec) => spec.scenario === 'expected-rejections').map(scenarioKey), [
        'expected-rejections-5-UNAUTHORIZED_TRANSFER',
        'expected-rejections-5-DUPLICATE_IDENTITY',
        'expected-rejections-5-BLOCKING_STATE-transfer',
        'expected-rejections-5-BLOCKING_STATE-dispense',
    ]);
});

test('series arguments validate scenario specs against the round profiles', () => {
    const options = parseArguments(['--only', 'write-register:5,expected-rejections:5:DUPLICATE_IDENTITY', '--repetitions', '1']);
    assert.deepEqual(options.specs, [
        {scenario: 'write-register', rate: 5},
        {scenario: 'expected-rejections', rate: 5, family: 'DUPLICATE_IDENTITY'},
    ]);
    assert.equal(options.repetitions, 1);
    assert.throws(() => parseArguments(['--only', 'read-unit:5']), /rate must be one of/u);
    assert.throws(() => parseArguments(['--repetitions', '6']), /between 1 and 5/u);
});

test('coefficient of variation uses the sample deviation and needs two repetitions', () => {
    assert.equal(coefficientOfVariation([10]), undefined);
    assert.ok(Math.abs(coefficientOfVariation([9, 10, 11]) - 0.1) < 1e-12);
    assert.equal(needsExtension({throughputCV: 0.1, p95CV: 0.15}), false);
    assert.equal(needsExtension({throughputCV: 0.1, p95CV: 0.151}), true);
    assert.equal(needsExtension({throughputCV: undefined, p95CV: undefined}), false);
});

test('round metrics count expected rejections instead of successes in rejection rounds', () => {
    const base = {observedDurationSeconds: 60, operationLatency: {p95Ms: 42}};
    assert.deepEqual(roundMetrics({...base, scenario: 'expected-rejections', expectedRejections: 300, successfulOperations: 0}),
        {throughput: 5, p95Ms: 42});
    assert.deepEqual(roundMetrics({...base, scenario: 'write-register', expectedRejections: 0, successfulOperations: 120}),
        {throughput: 2, p95Ms: 42});
});
