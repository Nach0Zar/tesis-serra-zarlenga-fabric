#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const {availabilityWindows, successTimeline} = require('./availability');
const {validateCampaign} = require('./campaign');
const {CLASS_SERIES, availabilityChart, latencyChart, throughputChart} = require('./charts');
const {accreditRun, loadSnapshotManifests} = require('./evidence');
const {aggregate, compare, runMetrics} = require('./metrics');
const {buildReport} = require('./report');
const {failureListing, loadRuns} = require('./runs');

function parseArguments(arguments_) {
    const options = {inputs: [], snapshots: [], validate: true, allowNonCitable: false};
    for (let index = 0; index < arguments_.length; index += 1) {
        const name = arguments_[index];
        if (name === '--no-validate') {
            options.validate = false;
            continue;
        }
        if (name === '--allow-noncitable') {
            options.allowNonCitable = true;
            continue;
        }
        const value = arguments_[index + 1];
        if (value === undefined || value.startsWith('--')) throw new Error(`${name} requires a value`);
        index += 1;
        if (name === '--input') options.inputs.push(value);
        else if (name === '--snapshot') options.snapshots.push(value);
        else if (name === '--output') options.output = value;
        else if (name === '--campaign') options.campaign = value;
        else throw new Error(`unsupported argument ${name}`);
    }
    if (options.inputs.length === 0) throw new Error('at least one --input directory is required');
    if (!options.output) throw new Error('--output is required');
    return options;
}

function csvValue(value) {
    if (value === undefined || value === null) return '';
    const text = typeof value === 'object' ? JSON.stringify(value) : String(value);
    return /[",\n]/u.test(text) ? `"${text.replace(/"/gu, '""')}"` : text;
}

function toCSV(rows, columns) {
    return `${[columns.join(','), ...rows.map((row) => columns.map((column) => csvValue(row[column])).join(','))].join('\n')}\n`;
}

const REPETITION_COLUMNS = [
    'sut', 'scenario', 'variant', 'operation', 'targetOperationsPerSecond', 'repetition', 'nonCitable', 'snapshotId',
    'seriesToken', 'operations', 'completed', 'throughput', 'confirmedTransactionThroughput',
    'attemptedTransactionThroughput', 'confirmedTransactions', 'transactionAttempts', 'offeredOperationsPerSecond',
    'observedDurationSeconds', 'latencyMeanMs', 'latencyStandardDeviationMs', 'p50Ms', 'p95Ms', 'p99Ms',
    'dispatchP95Ms', 'receiveP95Ms', 'transferPairP95Ms', 'retryAttempts', 'retriedTransferPairRate', 'retryTimeMs',
    'retryByCause', 'expectedRejections', 'unexpectedFailures', 'observedErrorCodes', 'timeouts',
    'markersExpected', 'markersObserved', 'markersFromRegistrations', 'markersFromRegulatoryEvents',
    'markersPerSecond', 'markersShareOfSuccessfulWrites', 'runDirectory',
];

const STATISTICS = [
    'throughput', 'confirmedTransactionThroughput', 'attemptedTransactionThroughput', 'latencyMeanMs', 'p50Ms',
    'p95Ms', 'p99Ms', 'dispatchP95Ms', 'receiveP95Ms', 'transferPairP95Ms',
];

function repetitionRow(metric) {
    return {
        ...metric,
        markersExpected: metric.markers?.expected,
        markersObserved: metric.markers?.observed,
        markersFromRegistrations: metric.markers?.fromRegistrations,
        markersFromRegulatoryEvents: metric.markers?.fromRegulatoryEvents,
        markersPerSecond: metric.markers?.perSecond,
        markersShareOfSuccessfulWrites: metric.markers?.shareOfSuccessfulWrites,
    };
}

// Una fila por serie: la original de cinco siempre y la extendida si existe
// (seccion 7), para que el motivo de la extension quede a la vista.
function aggregateRows(entry) {
    const series = [['original', entry.original]];
    if (entry.extendedSeries) series.push(['extendida', entry.extendedSeries]);
    return series.map(([label, statistics]) => {
        const row = {
            sut: entry.sut, scenario: entry.scenario, variant: entry.variant, operation: entry.operation,
            targetOperationsPerSecond: entry.targetOperationsPerSecond, workers: entry.workers,
            durationSeconds: entry.durationSeconds, series: label, repetitions: statistics.repetitions.join(' '),
            extendedBy: entry.extendedBy.join(' '), nonCitable: entry.nonCitable, snapshotId: entry.snapshotId,
            datasetSHA256: entry.datasetSHA256, artifact: entry.artifact.label, problems: entry.problems.join('; '),
            markersDeclaredRounds: entry.markers.declaredRounds, markersObservedMean: entry.markers.observed?.mean,
            markersExpectedMean: entry.markers.expected?.mean, markersPerSecondMean: entry.markers.perSecond?.mean,
        };
        for (const metric of STATISTICS) {
            row[`${metric}Mean`] = statistics[metric].mean;
            row[`${metric}StandardDeviation`] = statistics[metric].standardDeviation;
            row[`${metric}CV`] = statistics[metric].coefficientOfVariation;
        }
        return row;
    });
}

function validateMetadata(runs) {
    const repoRoot = path.resolve(__dirname, '..', '..', '..');
    const files = runs.map((run) => run.metadataPath);
    if (files.length === 0) return;
    const result = spawnSync('go', ['run', './cmd/runmeta', ...files], {
        cwd: path.join(repoRoot, 'client'), encoding: 'utf8', maxBuffer: 64 * 1024 * 1024,
    });
    if (result.error) throw result.error;
    if (result.status !== 0) {
        throw new Error(`metadata.json no valida contra DES-20:\n${(result.stdout + result.stderr).trim()}`);
    }
}

const NON_MEASURED = new Set(['smoke', 'dataset-preparation']);

// Acreditacion y citabilidad, separadas de la exclusion del rendimiento estable:
// valen igual para rendimiento y disponibilidad. Ninguna medicion entra sin
// estado inicial acreditado y sin serie citable; con --allow-noncitable se
// admiten las no citables y todas las salidas que las usan se rotulan.
function accredit(runs, options) {
    const manifests = loadSnapshotManifests(options.snapshots.map((directory) => path.resolve(directory)));
    if (manifests.problems.length) throw new Error(`snapshots invalidos: ${manifests.problems.join('; ')}`);
    const roots = options.inputs.map((input) => path.resolve(input));
    for (const run of runs) {
        if (run.phase !== 'measurement' || NON_MEASURED.has(run.scenario) || run.discardReason || !run.raw) continue;
        const accreditation = accreditRun(run, {manifests, roots});
        run.accreditation = accreditation;
        if (accreditation.reason) run.admission = {ok: false, reason: `no acreditada: ${accreditation.reason}`};
        else if (accreditation.nonCitable && !options.allowNonCitable) {
            run.admission = {ok: false, reason: `no citable: ${accreditation.nonCitable}`};
        } else {
            run.admission = {ok: true};
            if (accreditation.nonCitable) run.nonCitable = accreditation.nonCitable;
        }
    }
}

function admitted(run) {
    return run.admission?.ok === true;
}

// Motivo por el que una ronda no entra al rendimiento estable.
function stableExclusion(run) {
    if (run.exclusion) return run.admission && !run.admission.ok ? `${run.exclusion}; ${run.admission.reason}` : run.exclusion;
    if (!admitted(run)) return run.admission?.reason ?? 'no acreditada';
    return undefined;
}

// Errores inesperados y timeouts de TODAS las rondas con crudo, incluidas las
// descartadas: son justamente las que los tienen (seccion 10, #47).
function failureRows(runs) {
    const rows = [];
    for (const run of runs) {
        if (!run.raw || ['smoke', 'dataset-preparation'].includes(run.scenario)) continue;
        const {unexpected, timeouts} = failureListing(run.raw);
        if (unexpected.length === 0 && !timeouts) continue;
        const base = {
            sut: run.sut, scenario: run.scenario, variant: run.variant, targetOperationsPerSecond: run.targetOperationsPerSecond,
            phase: run.phase, repetition: run.repetition, status: run.exclusion ? 'excluida' : 'incluida',
            exclusion: run.exclusion, timeouts, directory: run.directory,
        };
        if (unexpected.length === 0) rows.push(base);
        for (const failure of unexpected) rows.push({...base, ...failure});
    }
    return rows;
}

function preparationRows(runs) {
    return runs.filter((run) => run.scenario === 'dataset-preparation').map((run) => ({
        sut: run.sut, startedAt: run.startedAt, endedAt: run.endedAt, durationSeconds: run.durationSeconds,
        markersExpected: run.participationMarkers?.expected, markersObserved: run.participationMarkers?.observed,
        markersFromRegistrations: run.participationMarkers?.fromRegistrations,
        markersFromRegulatoryEvents: run.participationMarkers?.fromRegulatoryEvents,
        markersPerSecond: run.participationMarkers?.perSecond,
        markersShareOfSuccessfulWrites: run.participationMarkers?.shareOfSuccessfulWrites,
        successfulWriteTransactions: run.participationMarkers?.successfulWriteTransactions,
        directory: run.directory,
    }));
}

function availabilityTimelines(entry, run) {
    const recoveredOffset = run.faultInjection.recoveredAt
        ? (Date.parse(run.faultInjection.recoveredAt) - Date.parse(run.faultInjection.injectedAt)) / 1000
        : undefined;
    // En caidas de peer la figura muestra las clases, nunca solo el global.
    if (entry.byClass) {
        return [
            {series: CLASS_SERIES.target, points: successTimeline({...run, raw: run.raw.filter((operation) => operation.availabilityClass === 'target')}), recoveredOffset},
            {series: CLASS_SERIES.control, points: successTimeline({...run, raw: run.raw.filter((operation) => operation.availabilityClass === 'control')}), recoveredOffset},
        ];
    }
    return [{sut: run.sut, points: successTimeline(run), recoveredOffset}];
}

function processResults(options) {
    const runs = loadRuns(options.inputs.map((input) => path.resolve(input)));
    if (runs.length === 0) throw new Error('no metadata.json found under the inputs');
    if (options.validate) validateMetadata(runs);
    accredit(runs, {snapshots: [], allowNonCitable: false, ...options});
    const campaign = options.campaign ? validateCampaign(options.campaign, runs, options) : undefined;
    const included = runs.filter((run) => !stableExclusion(run));
    const excluded = runs.filter((run) => stableExclusion(run)).map((run) => ({...run, exclusion: stableExclusion(run)}));
    const aggregates = aggregate(included);
    if (campaign) {
        for (const entry of aggregates) {
            const matrix = require('../../caliper/src/series-policy').fullScenarioMatrix();
            const scenario = matrix.find((item) => item.scenario === entry.scenario && item.rate === entry.targetOperationsPerSecond
                && (item.family ?? '') === (entry.variant.split('/')[0] ?? '')
                && (!item.operation || entry.operation === item.operation));
            const extension = campaign.extensionRows.find((row) => row.sut === entry.sut && row.scenarioKey === scenario?.key);
            if (extension && extension.reason === 'mantener pares con el otro SUT') entry.extendedBy = [extension.reason];
        }
    }
    const {comparisons, unpaired} = compare(aggregates);
    const repetitions = included.map(runMetrics);
    const failures = failureRows(runs);
    const preparation = preparationRows(runs);
    const availabilityRuns = runs.filter((run) => run.faultInjection && admitted(run));
    const availability = availabilityRuns.map((run) => ({
        run, entry: {sut: run.sut, scenario: run.scenario, repetition: run.repetition, directory: run.directory,
            nonCitable: run.nonCitable, ...availabilityWindows(run)},
    })).filter(({entry}) => entry.windows);
    const nonCitable = [...included, ...availabilityRuns].some((run) => run.nonCitable);

    const output = path.resolve(options.output);
    fs.mkdirSync(path.join(output, 'figures'), {recursive: true});
    const write = (name, contents) => fs.writeFileSync(path.join(output, name), contents);
    write('repetitions.csv', toCSV(repetitions.map(repetitionRow), REPETITION_COLUMNS));
    const rows = aggregates.flatMap(aggregateRows);
    write('aggregate.csv', toCSV(rows, Object.keys(rows[0] ?? {sut: ''})));
    write('comparison.csv', toCSV(comparisons.map((entry) => ({
        scenario: entry.scenario, variant: entry.variant, targetOperationsPerSecond: entry.targetOperationsPerSecond,
        series: entry.series, pairedRepetitions: entry.pairedRepetitions.join(' '),
        missingFabric: entry.missingRepetitions.fabric.join(' '), missingBaseline: entry.missingRepetitions.baseline.join(' '),
        nonCitable: entry.nonCitable,
        fabricThroughput: entry.fabric.throughput.mean, baselineThroughput: entry.baseline.throughput.mean,
        throughputRatio: entry.throughputRatio, fabricP50Ms: entry.fabric.p50Ms.mean, baselineP50Ms: entry.baseline.p50Ms.mean,
        p50Ratio: entry.p50Ratio, fabricP95Ms: entry.fabric.p95Ms.mean, baselineP95Ms: entry.baseline.p95Ms.mean,
        p95Ratio: entry.p95Ratio, fabricP99Ms: entry.fabric.p99Ms.mean, baselineP99Ms: entry.baseline.p99Ms.mean,
        p99Ratio: entry.p99Ratio, fabricAttemptedTxPerSecond: entry.fabric.attemptedTransactionThroughput.mean,
        baselineAttemptedTxPerSecond: entry.baseline.attemptedTransactionThroughput.mean,
        fabricArtifact: entry.fabric.artifact.label, baselineArtifact: entry.baseline.artifact.label,
        fabricSnapshot: entry.fabric.snapshotId, baselineSnapshot: entry.baseline.snapshotId,
        datasetSHA256: entry.fabric.datasetSHA256, logicalPlanSHA256: entry.logicalPlanSHA256,
    })), ['scenario', 'variant', 'targetOperationsPerSecond', 'series', 'pairedRepetitions', 'missingFabric',
        'missingBaseline', 'nonCitable', 'fabricThroughput', 'baselineThroughput', 'throughputRatio', 'fabricP50Ms',
        'baselineP50Ms', 'p50Ratio', 'fabricP95Ms', 'baselineP95Ms', 'p95Ratio', 'fabricP99Ms', 'baselineP99Ms',
        'p99Ratio', 'fabricAttemptedTxPerSecond', 'baselineAttemptedTxPerSecond', 'fabricArtifact',
        'baselineArtifact', 'fabricSnapshot', 'baselineSnapshot', 'datasetSHA256', 'logicalPlanSHA256']));
    write('excluded.csv', toCSV(excluded.map((run) => ({
        sut: run.sut, scenario: run.scenario, phase: run.phase, repetition: run.repetition, reason: run.exclusion,
        directory: run.directory,
    })), ['sut', 'scenario', 'phase', 'repetition', 'reason', 'directory']));
    write('unexpected.csv', toCSV(failures, ['sut', 'scenario', 'variant', 'targetOperationsPerSecond', 'phase',
        'repetition', 'status', 'exclusion', 'timeouts', 'datasetSequence', 'type', 'code', 'retryExhausted', 'directory']));
    if (preparation.length) {
        write('preparation.csv', toCSV(preparation, Object.keys(preparation[0])));
    }
    if (availability.length) {
        write('availability.json', `${JSON.stringify(availability.map(({entry}) => entry), null, 2)}\n`);
    }

    const figures = [];
    const addFigure = (name, svg) => {
        if (!svg) return;
        write(path.join('figures', name), svg);
        figures.push(name);
    };
    addFigure('throughput.svg', throughputChart(aggregates, {nonCitable}));
    addFigure('latency.svg', latencyChart(aggregates, {nonCitable}));
    // Figuras de disponibilidad sólo desde resultados admitidos y no bloqueados.
    for (const {run, entry} of availability) {
        if (entry.blocked) continue;
        addFigure(`availability-${entry.sut}-${entry.scenario}-run-${String(entry.repetition).padStart(2, '0')}.svg`,
            availabilityChart(`Disponibilidad ${entry.scenario}, repetición ${entry.repetition}`, availabilityTimelines(entry, run),
                {nonCitable: Boolean(entry.nonCitable), sut: entry.byClass ? entry.sut : undefined}));
    }
    const campaignReport = campaign ? '\n## Campaña alternada verificada\n\nCommit experimental: ' + campaign.campaign.repositoryCommit
        + '. Orden y repeticiones pareadas validados.\n\n'
        + campaign.extensionRows.map((row) => '- ' + row.scenarioKey + ', ' + row.sut + ': ' + row.reason
            + '; activadores: ' + row.triggers.map((trigger) => trigger.sut + '/' + trigger.metric + ' CV=' + trigger.cv).join(', ')).join('\n')
        + '\n\nMismo host, sin presupuesto agregado equivalente demostrado; se conservan las cuotas originales y los recursos efectivos de cada SUT.\n' : '';
    write('report.md', buildReport({
        aggregates, comparisons, unpaired, excluded, failures, preparation,
        availability: availability.map(({entry}) => entry), inputs: options.inputs,
        nonCitable,
    }) + campaignReport);
    return {runs: runs.length, included: included.length, excluded: excluded.length, aggregates: aggregates.length,
        comparisons: comparisons.length, unpaired: unpaired.length, nonCitable, figures, output};
}

if (require.main === module) {
    try {
        const result = processResults(parseArguments(process.argv.slice(2)));
        process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
    } catch (error) {
        process.stderr.write(`Processing failed: ${error.message}\n`);
        process.exitCode = 1;
    }
}

module.exports = {accredit, failureRows, parseArguments, processResults, toCSV};
