'use strict';

const {buildPlan} = require('../../caliper/src/planner');
const {PROFILES, buildProfile} = require('../../caliper/src/profiles');

const EXPECTED_COUNTS = Object.freeze({
    total: 50_000,
    participating: 25_183,
    unregistered: 3_080,
    prepared: 22_103,
    filler: 24_817,
});

function profileMatrix() {
    const profiles = [];
    for (const scenario of ['write-register', 'write-transfer', 'write-dispense', 'read-unit', 'read-history', 'mixed']) {
        for (const rate of PROFILES[scenario].rates) {
            profiles.push(buildProfile({scenario, phase: 'measurement', repetition: '1', rate: String(rate)}));
        }
    }
    for (const [family, operation] of [
        ['UNAUTHORIZED_TRANSFER', undefined],
        ['DUPLICATE_IDENTITY', undefined],
        ['BLOCKING_STATE', 'transfer'],
        ['BLOCKING_STATE', 'dispense'],
    ]) {
        profiles.push(buildProfile({
            scenario: 'expected-rejections', phase: 'measurement', repetition: '1', rate: '5', family, operation,
        }));
    }
    if (profiles.length !== 20) throw new Error(`expected 20 profile combinations, got ${profiles.length}`);
    return profiles;
}

function sameInvocations(left, right) {
    return JSON.stringify(left) === JSON.stringify(right);
}

function buildGoldenSnapshotPlan(dataset, seed = 20260727, options = {}) {
    if (!Array.isArray(dataset?.units)) throw new Error('dataset must contain an units array');
    const plans = profileMatrix().map((profile) => buildPlan(dataset, profile, seed));
    const participantSequences = new Set();
    const unregisteredSequences = new Set();
    const preparations = new Map();

    for (const plan of plans) {
        for (const worker of plan.workers) {
            for (const operation of worker.operations) {
                participantSequences.add(operation.datasetSequence);
                if (operation.type === 'register' && !operation.expectedRejection) {
                    unregisteredSequences.add(operation.datasetSequence);
                }
            }
        }
        for (const preparation of plan.preparations) {
            const existing = preparations.get(preparation.datasetSequence);
            if (existing && !sameInvocations(existing, preparation.invocations)) {
                throw new Error(`conflicting snapshot preparations for dataset sequence ${preparation.datasetSequence}`);
            }
            preparations.set(preparation.datasetSequence, preparation.invocations);
        }
    }

    for (const sequence of unregisteredSequences) {
        if (preparations.has(sequence)) throw new Error(`unregistered sequence ${sequence} also requires preparation`);
    }
    const allSequences = new Set(dataset.units.map((unit) => unit.sequence));
    const fillerSequences = [...allSequences].filter((sequence) => !participantSequences.has(sequence)).sort((a, b) => a - b);
    const excludedSequences = [...unregisteredSequences].sort((a, b) => a - b);
    const preparedSequences = [...participantSequences].filter((sequence) => !unregisteredSequences.has(sequence)).sort((a, b) => a - b);
    const counts = {
        total: dataset.units.length,
        participating: participantSequences.size,
        unregistered: excludedSequences.length,
        prepared: preparedSequences.length,
        filler: fillerSequences.length,
    };
    if (options.assertCanonical !== false) {
        for (const [name, expected] of Object.entries(EXPECTED_COUNTS)) {
            if (counts[name] !== expected) throw new Error(`canonical snapshot ${name} count: expected ${expected}, got ${counts[name]}`);
        }
    }
    if (fillerSequences.length === 0) throw new Error('snapshot has no filler unit for read-only smoke');

    return {
        seed,
        counts,
        excludedSequences,
        preparedSequences,
        fillerSequences,
        smokeSequence: fillerSequences[0],
        preparations: preparedSequences.map((datasetSequence) => ({
            datasetSequence,
            invocations: preparations.get(datasetSequence) ?? [],
        })),
    };
}

function registrationForUnit(unit) {
    const invocation = unit.preparation?.[0];
    if (invocation?.operation !== 'RegisterUnit') throw new Error(`dataset sequence ${unit.sequence} does not start with RegisterUnit`);
    return invocation;
}

module.exports = {EXPECTED_COUNTS, buildGoldenSnapshotPlan, profileMatrix, registrationForUnit};
