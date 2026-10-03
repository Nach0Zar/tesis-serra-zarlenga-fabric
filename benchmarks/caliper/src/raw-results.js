'use strict';

const fs = require('node:fs');
const path = require('node:path');

const {writeJSONAtomic} = require('./metadata');

function appendOperation(partPath, operation) {
    fs.appendFileSync(partPath, `${JSON.stringify(operation)}\n`, {encoding: 'utf8', mode: 0o600});
}

function percentile(sorted, percentage) {
    if (sorted.length === 0) return undefined;
    return sorted[Math.max(0, Math.ceil(sorted.length * percentage) - 1)];
}

function statistics(values) {
    if (values.length === 0) return undefined;
    const sorted = [...values].sort((left, right) => left - right);
    const mean = values.reduce((sum, value) => sum + value, 0) / values.length;
    const variance = values.reduce((sum, value) => sum + ((value - mean) ** 2), 0) / values.length;
    return {
        count: values.length,
        minimumMs: sorted[0],
        maximumMs: sorted.at(-1),
        meanMs: mean,
        standardDeviationMs: Math.sqrt(variance),
        p50Ms: percentile(sorted, 0.50),
        p95Ms: percentile(sorted, 0.95),
        p99Ms: percentile(sorted, 0.99),
    };
}

function durationSeconds(operations, configuredDurationSeconds, observedWindow = {}) {
    const startedAt = observedWindow.startedAt
        ?? operations.map((operation) => operation.startedAt).sort().at(0);
    const endedAt = observedWindow.endedAt
        ?? operations.map((operation) => operation.endedAt).sort().at(-1);
    const elapsedSeconds = startedAt && endedAt
        ? Math.max(0, Date.parse(endedAt) - Date.parse(startedAt)) / 1000
        : 0;
    return {startedAt, endedAt, seconds: Math.max(configuredDurationSeconds, elapsedSeconds)};
}

function readParts(runDirectory) {
    const files = fs.readdirSync(runDirectory)
        .filter((name) => /^raw-worker-\d+\.jsonl$/u.test(name))
        .sort((left, right) => left.localeCompare(right));
    const operations = [];
    for (const file of files) {
        const contents = fs.readFileSync(path.join(runDirectory, file), 'utf8').trim();
        if (contents === '') continue;
        for (const line of contents.split(/\r?\n/u)) operations.push(JSON.parse(line));
    }
    return operations.sort((left, right) => left.workerIndex - right.workerIndex || left.ordinal - right.ordinal);
}

function aggregateResults(runDirectory, profile, observedWindow = {}) {
    const operations = readParts(runDirectory);
    const transactions = operations.flatMap((operation) => operation.transactions);
    const measuredWindow = durationSeconds(operations, profile.durationSeconds, observedWindow);
    const byFunction = {};
    for (const transaction of transactions) {
        const list = byFunction[transaction.function] ?? [];
        list.push(transaction.latencyMs);
        byFunction[transaction.function] = list;
    }
    const unexpected = operations.filter((operation) => operation.outcome === 'unexpected-failure');
    const expectedRejections = operations.filter((operation) => operation.outcome === 'expected-rejection');
    const successful = operations.filter((operation) => operation.outcome === 'success');
    const observedErrorCodes = {};
    for (const operation of operations) {
        if (operation.observedErrorCode) {
            observedErrorCodes[operation.observedErrorCode] = (observedErrorCodes[operation.observedErrorCode] ?? 0) + 1;
        }
    }
    const incompletePairs = operations.filter((operation) => operation.type === 'transfer'
        && operation.outcome === 'success'
        && (operation.transactions.filter((transaction) => transaction.status === 'success').length !== 2
            || operation.transactions[0]?.function !== 'DispatchTransfer'
            || operation.transactions.at(-1)?.function !== 'ReceiveTransfer'));
    let discardReason;
    if (operations.length === 0) discardReason = 'la ronda no produjo operaciones';
    else if (unexpected.length > 0) discardReason = `${unexpected.length} operaciones tuvieron fallos inesperados`;
    else if (incompletePairs.length > 0) discardReason = `${incompletePairs.length} pares de transferencia quedaron incompletos`;
    else if (profile.scenario === 'expected-rejections' && expectedRejections.length !== operations.length) {
        discardReason = 'la ronda de rechazo incluyó resultados que no fueron rechazos esperados';
    } else if (profile.scenario !== 'expected-rejections' && successful.length !== operations.length) {
        discardReason = 'la ronda de camino feliz incluyó operaciones no exitosas';
    }
    const transferPairs = operations.filter((operation) => operation.type === 'transfer');
    const retriedTransferPairs = transferPairs.filter((operation) => operation.transactions.some(
        (transaction) => transaction.retry === true,
    ));
    const summary = {
        scenario: profile.scenario,
        operationCount: operations.length,
        successfulOperations: successful.length,
        expectedRejections: expectedRejections.length,
        unexpectedFailures: unexpected.length,
        transactionCount: transactions.length,
        successfulTransactions: transactions.filter((transaction) => transaction.status === 'success').length,
        failedTransactions: transactions.filter((transaction) => transaction.status === 'failed').length,
        retryAttempts: transactions.filter((transaction) => transaction.retry === true).length,
        transferPairs: transferPairs.length,
        retriedTransferPairs: retriedTransferPairs.length,
        retriedTransferPairRate: transferPairs.length > 0 ? retriedTransferPairs.length / transferPairs.length : 0,
        observedErrorCodes,
        rate: {
            operation: profile.operation,
            targetOperationsPerSecond: profile.rate,
            targetTransactionsPerSecond: profile.rate * profile.transactionsPerOperation,
            effectiveOperationsPerSecond: operations.length / measuredWindow.seconds,
            effectiveTransactionsPerSecond: transactions.length / measuredWindow.seconds,
        },
        operationLatency: statistics(operations.map((operation) => operation.latencyMs)),
        transactionLatencyByFunction: Object.fromEntries(
            Object.entries(byFunction).map(([functionName, values]) => [functionName, statistics(values)]),
        ),
        startedAt: measuredWindow.startedAt,
        endedAt: measuredWindow.endedAt,
        observedDurationSeconds: measuredWindow.seconds,
        discardReason,
    };
    writeJSONAtomic(path.join(runDirectory, 'raw.json'), operations);
    writeJSONAtomic(path.join(runDirectory, 'summary.json'), summary);
    return {operations, summary};
}

module.exports = {aggregateResults, appendOperation, durationSeconds, percentile, readParts, statistics};
