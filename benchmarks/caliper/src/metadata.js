'use strict';

const fs = require('node:fs');
const path = require('node:path');

function buildMetadata(context, observations) {
    const elapsedSeconds = Math.max(
        (new Date(observations.endedAt).getTime() - new Date(observations.startedAt).getTime()) / 1000,
        Number.EPSILON,
    );
    const document = {
        $schema: 'urn:pfi-snt:run-metadata:schema:1.0.0',
        schemaVersion: '1.0.0',
        protocol: 'measurement-protocol',
        repositoryCommit: context.repositoryCommit,
        sut: 'fabric',
        scenario: 'smoke',
        phase: 'measurement',
        repetition: 1,
        dataset: context.dataset,
        workers: 1,
        transactions: context.expectedTransactions,
        rate: {
            operation: 'query-unit',
            transactionsPerOperation: 1,
            targetOperationsPerSecond: 1,
            targetTransactionsPerSecond: 1,
            effectiveTransactionsPerSecond: observations.successfulTransactions / elapsedSeconds,
            rateController: 'fixed-rate',
        },
        startedAt: observations.startedAt,
        endedAt: observations.endedAt,
        host: context.host,
        environment: context.environment,
        notes: 'Smoke diagnóstico EVAL-1: una ronda ReadUnit; no corresponde a una corrida experimental final.',
    };
    if (observations.discardReason) {
        document.discarded = {reason: observations.discardReason};
    }
    return document;
}

function writeJSONAtomic(filePath, value) {
    fs.mkdirSync(path.dirname(filePath), {recursive: true});
    const temporaryPath = `${filePath}.tmp-${process.pid}`;
    fs.writeFileSync(temporaryPath, `${JSON.stringify(value, null, 2)}\n`, {mode: 0o600});
    fs.renameSync(temporaryPath, filePath);
}

module.exports = {buildMetadata, writeJSONAtomic};
