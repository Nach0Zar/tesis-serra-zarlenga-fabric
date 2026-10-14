'use strict';

const path = require('node:path');
const timers = require('node:timers/promises');

const {appendOperation, statistics} = require('../../caliper/src/raw-results');
const {writeJSONAtomic} = require('../../caliper/src/metadata');
const {unitReference} = require('./preconditions');

const SMOKE_TRANSACTIONS = 30;

async function runReadSmoke({client, dataset, sequence, outputDirectory, now = Date.now, sleep = timers.setTimeout}) {
    const reference = unitReference(dataset, sequence);
    const partPath = path.join(outputDirectory, 'raw-worker-0.jsonl');
    const operations = [];
    const roundStarted = now();
    for (let ordinal = 0; ordinal < SMOKE_TRANSACTIONS; ordinal += 1) {
        const delay = Math.max(0, roundStarted + (ordinal * 1000) - now());
        if (delay > 0) await sleep(delay);
        const started = now();
        const result = await client.invoke({
            operation: 'ReadUnit', invokerMspId: reference.invokerMspId,
            request: {gtin: reference.gtin, numeroSerie: reference.numeroSerie},
        });
        const ended = now();
        const operation = {
            workerIndex: 0, ordinal, datasetSequence: sequence, type: 'query-unit',
            outcome: result.ok ? 'success' : 'unexpected-failure',
            retryExhausted: false,
            startedAt: new Date(started).toISOString(),
            endedAt: new Date(ended).toISOString(),
            latencyMs: Math.max(0, ended - started),
            transactions: [result.transaction],
        };
        operations.push(operation);
        appendOperation(partPath, operation);
    }
    const ended = now();
    const successfulTransactions = operations.filter((operation) => operation.outcome === 'success').length;
    const observedDurationSeconds = Math.max((ended - roundStarted) / 1000, Number.EPSILON);
    const summary = {
        transactions: operations.length,
        successfulTransactions,
        failedTransactions: operations.length - successfulTransactions,
        startedAt: new Date(roundStarted).toISOString(),
        endedAt: new Date(ended).toISOString(),
        observedDurationSeconds,
        effectiveTransactionsPerSecond: successfulTransactions / observedDurationSeconds,
        latency: statistics(operations.map((operation) => operation.latencyMs)),
        discardReason: successfulTransactions === SMOKE_TRANSACTIONS
            ? undefined : `el smoke completo ${successfulTransactions}/${SMOKE_TRANSACTIONS} lecturas exitosas`,
    };
    writeJSONAtomic(path.join(outputDirectory, 'raw.json'), operations);
    writeJSONAtomic(path.join(outputDirectory, 'summary.json'), summary);
    return summary;
}

module.exports = {SMOKE_TRANSACTIONS, runReadSmoke};
