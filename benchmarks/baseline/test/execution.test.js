'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {performance} = require('node:perf_hooks');
const test = require('node:test');

const {aggregateResults} = require('../../caliper/src/raw-results');
const {executeOperation, runFixedRate} = require('../src/execution');
const timers = require('node:timers/promises');

function transaction(functionName, status = 'success', code) {
    return {function: functionName, status, errorCode: code, retry: false, latencyMs: 5};
}

test('transfer records both requests and end-to-end latency without Fabric retries', async () => {
    let now = 1000;
    const calls = [];
    const client = {invoke: async (entry) => {
        calls.push(entry.operation);
        now += entry.operation === 'DispatchTransfer' ? 7 : 11;
        return {ok: true, transaction: transaction(entry.operation)};
    }};
    const record = await executeOperation(client, {
        workerIndex: 0, ordinal: 0, datasetSequence: 1, type: 'transfer',
        invocations: [{operation: 'DispatchTransfer'}, {operation: 'ReceiveTransfer'}],
    }, {now: () => now});
    assert.deepEqual(calls, ['DispatchTransfer', 'ReceiveTransfer']);
    assert.equal(record.latencyMs, 18);
    assert.equal(record.transactions.length, 2);
    assert.equal(record.outcome, 'success');
    assert.equal(record.transactions.some((entry) => entry.retry), false);
});

test('expected rejection requires the exact body code and incomplete pairs fail', async () => {
    const exact = await executeOperation({invoke: async () => ({
        ok: false, transaction: transaction('RegisterUnit', 'failed', 'UNIT_ALREADY_EXISTS'),
    })}, {
        workerIndex: 0, ordinal: 0, datasetSequence: 1, type: 'register',
        invocations: [{operation: 'RegisterUnit'}],
        expectedRejection: {family: 'DUPLICATE_IDENTITY', code: 'UNIT_ALREADY_EXISTS'},
    });
    assert.equal(exact.outcome, 'expected-rejection');

    const wrong = await executeOperation({invoke: async () => ({
        ok: false, transaction: transaction('RegisterUnit', 'failed', 'INVALID_REQUEST'),
    })}, {...exact, invocations: [{operation: 'RegisterUnit'}], expectedRejection: {
        family: 'DUPLICATE_IDENTITY', code: 'UNIT_ALREADY_EXISTS',
    }});
    assert.equal(wrong.outcome, 'unexpected-failure');

    let call = 0;
    const incomplete = await executeOperation({invoke: async (entry) => {
        call += 1;
        return {ok: call === 1, transaction: transaction(entry.operation, call === 1 ? 'success' : 'failed', 'NOT_IN_TRANSIT')};
    }}, {
        workerIndex: 0, ordinal: 0, datasetSequence: 2, type: 'transfer',
        invocations: [{operation: 'DispatchTransfer'}, {operation: 'ReceiveTransfer'}],
    });
    assert.equal(incomplete.outcome, 'unexpected-failure');
});

test('fixed-rate schedules the exact aggregate operation count and emits processable parts', async (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-baseline-rate-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    const profile = {
        scenario: 'write-register', operation: 'register', workers: 2,
        durationSeconds: 2, rate: 2, transactionsPerOperation: 1,
    };
    const plan = {workers: Array.from({length: 2}, (_, workerIndex) => ({
        workerIndex,
        operations: Array.from({length: 2}, (_, ordinal) => ({
            ordinal, datasetSequence: workerIndex * 10 + ordinal, type: 'register',
            invocations: [{operation: 'RegisterUnit'}],
        })),
    }))};
    let now = 0;
    const window = await runFixedRate({
        client: {invoke: async (entry) => ({ok: true, transaction: transaction(entry.operation)})},
        plan, profile,
        partPaths: [path.join(directory, 'raw-worker-0.jsonl'), path.join(directory, 'raw-worker-1.jsonl')],
        now: () => now,
        sleep: async (delay) => { now += delay; },
    });
    assert.equal(window.startedAt, '1970-01-01T00:00:00.000Z');
    assert.equal(window.endedAt, '1970-01-01T00:00:02.000Z');
    const {operations, summary} = aggregateResults(directory, profile);
    assert.equal(operations.length, 4);
    assert.equal(summary.successfulOperations, 4);
    assert.equal(summary.discardReason, undefined);
});

test('slow responses do not reduce offered load and pending requests drain after the window', async (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-baseline-open-loop-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    const profile = {scenario: 'read-unit', operation: 'query-unit', workers: 1,
        durationSeconds: 2, rate: 10, transactionsPerOperation: 1};
    const starts = [];
    const now = () => Math.floor(performance.now());
    let inFlight = 0;
    let maximumInFlight = 0;
    const client = {invoke: async () => {
        starts.push(now());
        inFlight += 1;
        maximumInFlight = Math.max(maximumInFlight, inFlight);
        await timers.setTimeout(250);
        inFlight -= 1;
        return {ok: true, transaction: transaction('ReadUnit')};
    }};
    const plan = {workers: [{workerIndex: 0, operations: Array.from({length: 20}, (_, ordinal) => ({
        ordinal, datasetSequence: ordinal + 1, type: 'query-unit', invocations: [{operation: 'ReadUnit'}],
    }))}]};
    const window = await runFixedRate({client, plan, profile, now,
        partPaths: [path.join(directory, 'raw-worker-0.jsonl')]});
    const {summary, operations} = aggregateResults(directory, profile, window);
    assert.equal(starts.length, 20);
    assert.ok(maximumInFlight >= 3);
    assert.equal(inFlight, 0);
    assert.ok(starts.at(-1) - Date.parse(window.startedAt) < 2000);
    assert.ok(Date.parse(window.endedAt) - Date.parse(window.startedAt) < 3000);
    assert.equal(summary.rate.offeredOperationsPerSecond, 10);
    assert.ok(summary.rate.effectiveOperationsPerSecond < 10);
    assert.equal(summary.discardReason, undefined);
    assert.ok(operations.every((operation) => operation.scheduledAt && operation.submissionDelayMs >= 0));
});

test('scheduling stops at the window boundary instead of offering overdue operations', async (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-baseline-late-admission-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    let now = 0;
    let calls = 0;
    const profile = {scenario: 'read-unit', operation: 'query-unit', workers: 1,
        durationSeconds: 2, rate: 2, transactionsPerOperation: 1};
    const plan = {workers: [{workerIndex: 0, operations: Array.from({length: 4}, (_, ordinal) => ({
        ordinal, datasetSequence: ordinal + 1, type: 'query-unit', invocations: [{operation: 'ReadUnit'}],
    }))}]};
    const window = await runFixedRate({client: {invoke: async () => {
        calls += 1;
        return {ok: true, transaction: transaction('ReadUnit')};
    }}, plan, profile, partPaths: [path.join(directory, 'raw-worker-0.jsonl')],
    now: () => now, sleep: async () => { now = 2500; }});
    const {summary} = aggregateResults(directory, profile, window);
    assert.equal(calls, 1);
    assert.equal(summary.rate.offeredOperationsPerSecond, 0.5);
});
