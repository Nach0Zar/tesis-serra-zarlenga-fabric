'use strict';

const fs = require('node:fs');
const path = require('node:path');

const MEASURED_PHASE = 'measurement';
const NON_COMPARABLE_SCENARIOS = new Set(['smoke', 'dataset-preparation']);
const AVAILABILITY_SCENARIOS = new Set(['raft-1', 'raft-2', 'peer-1a', 'peer-1b', 'peer-1c', 'peer-1d', 'db-1', 'api-1']);
// Ciclo de la seccion 6.4: 2 altas, 11 transferencias, 2 dispensaciones, 5 lecturas.
const MIX_PATTERN = Object.freeze([
    'register', 'transfer', 'transfer', 'query-unit', 'transfer',
    'dispense', 'transfer', 'query-unit', 'transfer', 'register',
    'transfer', 'query-unit', 'transfer', 'dispense', 'transfer',
    'query-unit', 'transfer', 'transfer', 'query-unit', 'transfer',
]);

function readJSON(filePath) {
    return JSON.parse(fs.readFileSync(filePath, 'utf8'));
}

function findMetadataFiles(root) {
    const found = [];
    const visit = (directory) => {
        for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
            const candidate = path.join(directory, entry.name);
            if (entry.isDirectory()) visit(candidate);
            else if (entry.isFile() && entry.name === 'metadata.json') found.push(candidate);
        }
    };
    visit(root);
    return found.sort();
}

// Artefacto medido de cada lado: lo que la salida debe identificar (#47).
function artifactOf(metadata) {
    const environment = metadata.environment ?? {};
    if (metadata.sut === 'fabric') {
        return {
            sut: 'fabric', contractVersion: environment.contractVersion, packageID: environment.packageID,
            fabric: environment.fabric, label: `contrato ${environment.contractVersion}, ${environment.packageID}`,
        };
    }
    return {
        sut: 'baseline', baselineCommit: environment.baselineCommit, baselineImage: environment.baselineImage,
        label: `baseline ${environment.baselineCommit}, ${environment.baselineImage}`,
    };
}

// Variante comparable: familia de rechazo y operacion conceptual sobre la que recae.
function variantOf(metadata) {
    if (metadata.scenario !== 'expected-rejections') return '';
    return [metadata.rejectionFamily, metadata.rate?.operation].filter(Boolean).join('/');
}

function groupKey(run) {
    return [run.sut, run.scenario, run.variant, run.targetOperationsPerSecond].join('|');
}

function comparisonKey(run) {
    return [run.scenario, run.variant, run.targetOperationsPerSecond].join('|');
}

// Mezcla exacta: cada worker recorre el ciclo en orden, con una rotacion fija.
function verifyMixComposition(operations) {
    const byWorker = new Map();
    for (const operation of operations) {
        const list = byWorker.get(operation.workerIndex) ?? [];
        list.push(operation);
        byWorker.set(operation.workerIndex, list);
    }
    const counts = {};
    for (const operation of operations) counts[operation.type] = (counts[operation.type] ?? 0) + 1;
    for (const [workerIndex, list] of byWorker) {
        list.sort((left, right) => left.ordinal - right.ordinal);
        const first = list[0];
        const rotation = MIX_PATTERN.findIndex((type, index) => list.every((operation) =>
            MIX_PATTERN[(operation.ordinal + index) % MIX_PATTERN.length] === operation.type));
        if (rotation < 0) {
            return {valid: false, counts, reason: `el worker ${workerIndex} no sigue el ciclo 2/11/2/5 desde el ordinal ${first.ordinal}`};
        }
    }
    return {valid: true, counts};
}

function timeoutLike(transaction) {
    return transaction.transportError === 'TimeoutError'
        || /DEADLINE|TIMEOUT/iu.test(transaction.errorCode ?? '')
        || /DEADLINE|TIMEOUT/iu.test(transaction.transportError ?? '');
}

function failureListing(operations) {
    const unexpected = [];
    let timeouts = 0;
    for (const operation of operations) {
        for (const transaction of operation.transactions ?? []) {
            if (transaction.status === 'failed' && timeoutLike(transaction)) timeouts += 1;
        }
        if (operation.outcome !== 'unexpected-failure') continue;
        const failed = (operation.transactions ?? []).filter((transaction) => transaction.status === 'failed').at(-1);
        unexpected.push({
            datasetSequence: operation.datasetSequence,
            type: operation.type,
            code: failed?.errorCode ?? failed?.transportError ?? failed?.responseError ?? operation.observedErrorCode ?? 'sin código',
            retryExhausted: operation.retryExhausted === true,
        });
    }
    return {unexpected, timeouts};
}

function loadRun(metadataPath, options = {}) {
    const directory = path.dirname(metadataPath);
    const metadata = readJSON(metadataPath);
    const summaryPath = path.join(directory, 'summary.json');
    const rawPath = path.join(directory, 'raw.json');
    const summary = fs.existsSync(summaryPath) ? readJSON(summaryPath) : undefined;
    const raw = options.readRaw !== false && fs.existsSync(rawPath) ? readJSON(rawPath) : undefined;
    const run = {
        directory,
        metadataPath,
        sut: metadata.sut,
        scenario: metadata.scenario,
        phase: metadata.phase,
        repetition: metadata.repetition,
        variant: variantOf(metadata),
        operation: metadata.rate?.operation,
        targetOperationsPerSecond: metadata.rate?.targetOperationsPerSecond,
        transactionsPerOperation: metadata.rate?.transactionsPerOperation,
        workers: metadata.workers,
        durationSeconds: metadata.durationSeconds,
        mix: metadata.rate?.mix,
        datasetSHA256: metadata.dataset?.sha256,
        artifact: artifactOf(metadata),
        startedAt: metadata.startedAt,
        endedAt: metadata.endedAt,
        faultInjection: metadata.faultInjection,
        notes: metadata.notes,
        discardReason: metadata.discarded?.reason ?? summary?.discardReason,
        summary,
        raw,
    };
    run.exclusion = exclusionReason(run);
    return run;
}

// Lo que no entra en las estadisticas de rendimiento estable. Se conserva y se
// lista con su motivo (seccion 7: no eliminar sin justificacion documentada).
function exclusionReason(run) {
    if (NON_COMPARABLE_SCENARIOS.has(run.scenario)) return `escenario ${run.scenario}`;
    // Las rondas de disponibilidad se procesan por ventanas y no entran en el
    // rendimiento estable (#44).
    if (AVAILABILITY_SCENARIOS.has(run.scenario)) return 'escenario de disponibilidad';
    if (run.phase !== MEASURED_PHASE) return `fase ${run.phase}`;
    if (run.discardReason) return `descartada: ${run.discardReason}`;
    if (!run.summary) return 'falta summary.json';
    if (run.scenario === 'mixed' && run.raw) {
        const composition = verifyMixComposition(run.raw);
        if (!composition.valid) return `mezcla inválida: ${composition.reason}`;
    }
    return undefined;
}

function loadRuns(roots, options = {}) {
    const runs = [];
    for (const root of roots) {
        for (const metadataPath of findMetadataFiles(root)) runs.push(loadRun(metadataPath, options));
    }
    return runs;
}

module.exports = {
    AVAILABILITY_SCENARIOS,
    MIX_PATTERN,
    artifactOf,
    comparisonKey,
    exclusionReason,
    failureListing,
    findMetadataFiles,
    groupKey,
    loadRun,
    loadRuns,
    variantOf,
    verifyMixComposition,
};
