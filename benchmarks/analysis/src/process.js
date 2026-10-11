#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const {availabilityWindows, successTimeline} = require('./availability');
const {availabilityChart, latencyChart, throughputChart} = require('./charts');
const {aggregate, compare, runMetrics} = require('./metrics');
const {buildReport} = require('./report');
const {loadRuns} = require('./runs');

function parseArguments(arguments_) {
    const options = {inputs: [], validate: true};
    for (let index = 0; index < arguments_.length; index += 1) {
        const name = arguments_[index];
        if (name === '--no-validate') {
            options.validate = false;
            continue;
        }
        const value = arguments_[index + 1];
        if (value === undefined || value.startsWith('--')) throw new Error(`${name} requires a value`);
        index += 1;
        if (name === '--input') options.inputs.push(value);
        else if (name === '--output') options.output = value;
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
    'sut', 'scenario', 'variant', 'operation', 'targetOperationsPerSecond', 'repetition', 'operations', 'completed',
    'throughput', 'transactionThroughput', 'offeredOperationsPerSecond', 'observedDurationSeconds', 'latencyMeanMs',
    'latencyStandardDeviationMs', 'p50Ms', 'p95Ms', 'p99Ms', 'dispatchP95Ms', 'receiveP95Ms', 'transferPairP95Ms', 'retryAttempts',
    'retriedTransferPairRate', 'retryTimeMs', 'retryByCause', 'expectedRejections', 'unexpectedFailures',
    'observedErrorCodes', 'timeouts', 'runDirectory',
];

function aggregateRow(entry) {
    const row = {
        sut: entry.sut, scenario: entry.scenario, variant: entry.variant, operation: entry.operation,
        targetOperationsPerSecond: entry.targetOperationsPerSecond, repetitions: entry.repetitions.join(' '),
        extended: entry.extended, extendedBy: entry.extendedBy.join(' '), datasetSHA256: entry.datasetSHA256,
        artifact: entry.artifact.label, problems: entry.problems.join('; '),
    };
    for (const metric of ['throughput', 'transactionThroughput', 'latencyMeanMs', 'p50Ms', 'p95Ms', 'p99Ms', 'dispatchP95Ms', 'receiveP95Ms', 'transferPairP95Ms']) {
        row[`${metric}Mean`] = entry[metric].mean;
        row[`${metric}StandardDeviation`] = entry[metric].standardDeviation;
        row[`${metric}CV`] = entry[metric].coefficientOfVariation;
    }
    return row;
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

function processResults(options) {
    const runs = loadRuns(options.inputs.map((input) => path.resolve(input)));
    if (runs.length === 0) throw new Error('no metadata.json found under the inputs');
    if (options.validate) validateMetadata(runs);
    const included = runs.filter((run) => !run.exclusion);
    const excluded = runs.filter((run) => run.exclusion);
    const aggregates = aggregate(included);
    const {comparisons, unpaired} = compare(aggregates);
    const repetitions = included.map(runMetrics);
    const unexpected = repetitions.filter((metric) => metric.unexpected.length > 0 || metric.timeouts > 0);
    const availabilityRuns = runs.filter((run) => run.faultInjection && run.phase === 'measurement' && !run.discardReason);
    const availability = availabilityRuns.map((run) => ({
        sut: run.sut, scenario: run.scenario, repetition: run.repetition, directory: run.directory,
        ...availabilityWindows(run),
    })).filter((entry) => entry.windows);

    const output = path.resolve(options.output);
    fs.mkdirSync(path.join(output, 'figures'), {recursive: true});
    const write = (name, contents) => fs.writeFileSync(path.join(output, name), contents);
    write('repetitions.csv', toCSV(repetitions, REPETITION_COLUMNS));
    const aggregateRows = aggregates.map(aggregateRow);
    write('aggregate.csv', toCSV(aggregateRows, Object.keys(aggregateRows[0] ?? {sut: ''})));
    write('comparison.csv', toCSV(comparisons.map((entry) => ({
        scenario: entry.scenario, variant: entry.variant, targetOperationsPerSecond: entry.targetOperationsPerSecond,
        fabricThroughput: entry.fabric.throughput.mean, baselineThroughput: entry.baseline.throughput.mean,
        throughputRatio: entry.throughputRatio, fabricP50Ms: entry.fabric.p50Ms.mean, baselineP50Ms: entry.baseline.p50Ms.mean,
        p50Ratio: entry.p50Ratio, fabricP95Ms: entry.fabric.p95Ms.mean, baselineP95Ms: entry.baseline.p95Ms.mean,
        p95Ratio: entry.p95Ratio, fabricP99Ms: entry.fabric.p99Ms.mean, baselineP99Ms: entry.baseline.p99Ms.mean,
        p99Ratio: entry.p99Ratio, fabricArtifact: entry.fabric.artifact.label, baselineArtifact: entry.baseline.artifact.label,
        datasetSHA256: entry.fabric.datasetSHA256,
    })), ['scenario', 'variant', 'targetOperationsPerSecond', 'fabricThroughput', 'baselineThroughput', 'throughputRatio',
        'fabricP50Ms', 'baselineP50Ms', 'p50Ratio', 'fabricP95Ms', 'baselineP95Ms', 'p95Ratio', 'fabricP99Ms',
        'baselineP99Ms', 'p99Ratio', 'fabricArtifact', 'baselineArtifact', 'datasetSHA256']));
    write('excluded.csv', toCSV(excluded.map((run) => ({
        sut: run.sut, scenario: run.scenario, phase: run.phase, repetition: run.repetition, reason: run.exclusion,
        directory: run.directory,
    })), ['sut', 'scenario', 'phase', 'repetition', 'reason', 'directory']));
    write('unexpected.csv', toCSV(unexpected.flatMap((metric) => (metric.unexpected.length ? metric.unexpected : [{}]).map((failure) => ({
        sut: metric.sut, scenario: metric.scenario, variant: metric.variant, targetOperationsPerSecond: metric.targetOperationsPerSecond,
        repetition: metric.repetition, timeouts: metric.timeouts, ...failure,
    }))), ['sut', 'scenario', 'variant', 'targetOperationsPerSecond', 'repetition', 'timeouts', 'datasetSequence', 'type', 'code', 'retryExhausted']));
    if (availability.length) write('availability.json', `${JSON.stringify(availability, null, 2)}\n`);

    const figures = [];
    const addFigure = (name, svg) => {
        if (!svg) return;
        write(path.join('figures', name), svg);
        figures.push(name);
    };
    addFigure('throughput.svg', throughputChart(aggregates));
    addFigure('latency.svg', latencyChart(aggregates));
    const byScenario = new Map();
    for (const run of availabilityRuns) {
        const list = byScenario.get(run.scenario) ?? [];
        list.push(run);
        byScenario.set(run.scenario, list);
    }
    for (const [scenario, list] of byScenario) {
        for (const repetition of [...new Set(list.map((run) => run.repetition))]) {
            const timelines = list.filter((run) => run.repetition === repetition).map((run) => ({
                sut: run.sut,
                points: successTimeline(run),
                recoveredOffset: run.faultInjection.recoveredAt
                    ? (Date.parse(run.faultInjection.recoveredAt) - Date.parse(run.faultInjection.injectedAt)) / 1000
                    : undefined,
            }));
            addFigure(`availability-${scenario}-run-${String(repetition).padStart(2, '0')}.svg`,
                availabilityChart(`Disponibilidad ${scenario}, repetición ${repetition}`, timelines));
        }
    }
    write('report.md', buildReport({
        aggregates, comparisons, unpaired, excluded, unexpected, availability, inputs: options.inputs,
    }));
    return {runs: runs.length, included: included.length, excluded: excluded.length, aggregates: aggregates.length,
        comparisons: comparisons.length, unpaired: unpaired.length, figures, output};
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

module.exports = {parseArguments, processResults, toCSV};
