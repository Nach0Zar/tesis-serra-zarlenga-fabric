'use strict';

function mean(values) {
    if (values.length === 0) return undefined;
    return values.reduce((sum, value) => sum + value, 0) / values.length;
}

// Desvio poblacional: es el que fija la politica de extension de #46 y el que
// se informa entre repeticiones (seccion 7 del protocolo).
function populationStandardDeviation(values) {
    const average = mean(values);
    if (average === undefined) return undefined;
    return Math.sqrt(values.reduce((sum, value) => sum + ((value - average) ** 2), 0) / values.length);
}

function coefficientOfVariation(values) {
    const average = mean(values);
    if (average === undefined || average === 0) return undefined;
    return populationStandardDeviation(values) / average;
}

// Nearest-rank, igual que el agregador de las rondas.
function percentile(values, fraction) {
    if (values.length === 0) return undefined;
    const sorted = [...values].sort((left, right) => left - right);
    return sorted[Math.max(0, Math.ceil(sorted.length * fraction) - 1)];
}

function describe(values) {
    const present = values.filter((value) => Number.isFinite(value));
    return {
        n: present.length,
        mean: mean(present),
        standardDeviation: populationStandardDeviation(present),
        coefficientOfVariation: coefficientOfVariation(present),
        minimum: present.length ? Math.min(...present) : undefined,
        maximum: present.length ? Math.max(...present) : undefined,
    };
}

module.exports = {coefficientOfVariation, describe, mean, percentile, populationStandardDeviation};
