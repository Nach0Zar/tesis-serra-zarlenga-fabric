'use strict';
const {PROFILES} = require('./profiles');
const SUTS = ['fabric', 'baseline'];

function fullScenarioMatrix() {
    const matrix = [];
    for (const scenario of ['write-register', 'write-transfer', 'write-dispense', 'read-unit', 'read-history', 'mixed']) {
        for (const rate of PROFILES[scenario].rates) matrix.push({key: `${scenario}-${rate}`, scenario, rate});
    }
    for (const [family, operation] of [
        ['UNAUTHORIZED_TRANSFER', undefined],
        ['DUPLICATE_IDENTITY', undefined],
        ['BLOCKING_STATE', 'transfer'],
        ['BLOCKING_STATE', 'dispense'],
    ]) {
        const suffix = operation ? `${family.toLowerCase()}-${operation}` : family.toLowerCase();
        matrix.push({key: `expected-rejections-${suffix}-5`, scenario: 'expected-rejections', rate: 5, family, operation});
    }
    if (matrix.length !== 20) throw new Error(`expected 20 full-series scenarios, got ${matrix.length}`);
    return matrix;
}

function trialScenarioMatrix() {
    return [
        {key: 'read-unit-50', scenario: 'read-unit', rate: 50},
        {key: 'write-transfer-20', scenario: 'write-transfer', rate: 20},
        {key: 'mixed-20', scenario: 'mixed', rate: 20},
    ];
}

function coefficientOfVariation(values) {
    if (!Array.isArray(values) || values.length === 0) throw new Error('CV requires at least one value');
    if (values.some((value) => !Number.isFinite(value) || value < 0)) throw new Error('CV values must be finite and non-negative');
    const mean = values.reduce((sum, value) => sum + value, 0) / values.length;
    if (mean === 0) return 0;
    const variance = values.reduce((sum, value) => sum + ((value - mean) ** 2), 0) / values.length;
    return Math.sqrt(variance) / mean;
}

function repetitionOrder(repetition) {
    return repetition % 2 ? ['fabric', 'baseline'] : ['baseline', 'fabric'];
}
function extensionTriggers(summaries) {
    return SUTS.flatMap((sut) => [
        ['throughput', summaries[sut].map((summary) => summary.rate.effectiveOperationsPerSecond)],
        ['p95', summaries[sut].map((summary) => summary.operationLatency.p95Ms)],
    ].flatMap(([metric, values]) => {
        const cv = coefficientOfVariation(values);
        return cv > 0.15 ? [{sut, metric, cv}] : [];
    }));
}

module.exports = {coefficientOfVariation, extensionTriggers, fullScenarioMatrix, repetitionOrder, trialScenarioMatrix};
