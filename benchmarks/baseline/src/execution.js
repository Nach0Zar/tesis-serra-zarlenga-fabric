'use strict';

const timers = require('node:timers/promises');

const {appendOperation} = require('../../caliper/src/raw-results');

async function executeOperation(client, operation, clock = Date) {
    const started = clock.now();
    const transactions = [];
    let outcome = 'success';
    let observedErrorCode;

    if (operation.expectedRejection) {
        const result = await client.invoke(operation.invocations[0]);
        transactions.push(result.transaction);
        observedErrorCode = result.transaction.errorCode;
        outcome = !result.ok && observedErrorCode === operation.expectedRejection.code
            ? 'expected-rejection' : 'unexpected-failure';
    } else if (operation.type === 'transfer') {
        const dispatch = await client.invoke(operation.invocations[0]);
        transactions.push(dispatch.transaction);
        if (!dispatch.ok) {
            outcome = 'unexpected-failure';
        } else {
            const receive = await client.invoke(operation.invocations[1]);
            transactions.push(receive.transaction);
            if (!receive.ok) outcome = 'unexpected-failure';
        }
    } else {
        const result = await client.invoke(operation.invocations[0]);
        transactions.push(result.transaction);
        if (!result.ok) outcome = 'unexpected-failure';
    }

    const ended = clock.now();
    return {
        workerIndex: operation.workerIndex,
        ordinal: operation.ordinal,
        datasetSequence: operation.datasetSequence,
        type: operation.type,
        outcome,
        expectedRejection: operation.expectedRejection,
        observedErrorCode,
        retryExhausted: false,
        startedAt: new Date(started).toISOString(),
        endedAt: new Date(ended).toISOString(),
        latencyMs: Math.max(0, ended - started),
        transactions,
    };
}

async function runFixedRate({client, plan, profile, partPaths, now = Date.now, sleep = timers.setTimeout}) {
    const roundStarted = now();
    const intervalMs = 1000 / (profile.rate / profile.workers);
    const durationMs = profile.durationSeconds * 1000;
    const operationsPerWorker = profile.durationSeconds * profile.rate / profile.workers;
    if (!Number.isInteger(operationsPerWorker)) {
        throw new Error('profile does not divide into an exact per-worker operation count');
    }
    await Promise.all(plan.workers.map(async (worker) => {
        for (let ordinal = 0; ordinal < operationsPerWorker; ordinal += 1) {
            const operation = worker.operations[ordinal];
            if (!operation) throw new Error(`worker ${worker.workerIndex} exhausted its deterministic work plan`);
            const delay = Math.max(0, roundStarted + intervalMs * ordinal - now());
            if (delay > 0) await sleep(delay);
            const record = await executeOperation(client, {...operation, workerIndex: worker.workerIndex}, {now});
            appendOperation(partPaths[worker.workerIndex], record);
        }
    }));
    let remaining = Math.max(0, roundStarted + durationMs - now());
    while (remaining > 0) {
        await sleep(remaining);
        remaining = Math.max(0, roundStarted + durationMs - now());
    }
    return {
        startedAt: new Date(roundStarted).toISOString(),
        endedAt: new Date(now()).toISOString(),
    };
}

module.exports = {executeOperation, runFixedRate};
