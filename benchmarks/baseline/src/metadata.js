'use strict';

const os = require('node:os');
const fs = require('node:fs');

function readOSName() {
    try {
        const contents = fs.readFileSync('/etc/os-release', 'utf8');
        return contents.match(/^PRETTY_NAME=(?:"([^"]+)"|(.*))$/mu)?.slice(1).find(Boolean) ?? os.type();
    } catch {
        return os.type();
    }
}

function buildHostMetadata() {
    const cpu = os.cpus()[0];
    if (!cpu) throw new Error('cannot identify benchmark host CPU');
    const host = {
        cpu: cpu.model.trim(), cpuCores: os.cpus().length,
        memoryGB: Number((os.totalmem() / (1024 ** 3)).toFixed(3)),
        os: readOSName(), kernel: os.release(),
    };
    if (/microsoft/iu.test(os.release())) host.wsl = process.env.WSL_DISTRO_NAME ? `WSL2 ${process.env.WSL_DISTRO_NAME}` : 'WSL2';
    return host;
}

function baseMetadata(context) {
    return {
        $schema: 'urn:pfi-snt:run-metadata:schema:1.0.0',
        schemaVersion: '1.0.0',
        protocol: 'measurement-protocol',
        repositoryCommit: context.repositoryCommit,
        sut: 'baseline',
        dataset: context.dataset,
        host: context.host,
        environment: context.environment,
    };
}

function buildRoundMetadata(context, summary) {
    const profile = context.profile;
    const document = {
        ...baseMetadata(context),
        scenario: profile.scenario,
        phase: profile.phase,
        repetition: profile.repetition,
        workers: profile.workers,
        durationSeconds: profile.durationSeconds,
        rate: {
            operation: profile.operation,
            transactionsPerOperation: profile.transactionsPerOperation,
            targetOperationsPerSecond: profile.rate,
            targetTransactionsPerSecond: profile.rate * profile.transactionsPerOperation,
            effectiveTransactionsPerSecond: summary.rate.effectiveTransactionsPerSecond,
            rateController: 'fixed-rate',
        },
        startedAt: context.startedAt,
        endedAt: context.endedAt,
        notes: 'Ronda individual de la baseline REST; la serie estadistica y el procesamiento comparativo permanecen fuera de este runner.',
    };
    if (profile.mix) document.rate.mix = profile.mix;
    if (profile.rejectionFamily) document.rejectionFamily = profile.rejectionFamily;
    if (summary.discardReason) document.discarded = {reason: summary.discardReason};
    return document;
}

function buildSmokeMetadata(context, summary) {
    const document = {
        ...baseMetadata(context),
        scenario: 'smoke', phase: 'measurement', repetition: 1,
        workers: 1, transactions: 30,
        rate: {
            operation: 'query-unit', transactionsPerOperation: 1,
            targetOperationsPerSecond: 1, targetTransactionsPerSecond: 1,
            effectiveTransactionsPerSecond: summary.successfulTransactions / summary.observedDurationSeconds,
            rateController: 'fixed-rate',
        },
        startedAt: summary.startedAt, endedAt: summary.endedAt,
        notes: 'Smoke diagnostico de solo lectura sobre una unidad de relleno reservada.',
    };
    if (summary.discardReason) document.discarded = {reason: summary.discardReason};
    return document;
}

module.exports = {buildHostMetadata, buildRoundMetadata, buildSmokeMetadata};
