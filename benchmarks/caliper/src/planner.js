'use strict';

const {normalizeInvocation} = require('./requests');

const MIX_PATTERN = Object.freeze([
    'register', 'transfer', 'transfer', 'query-unit', 'transfer',
    'dispense', 'transfer', 'query-unit', 'transfer', 'register',
    'transfer', 'query-unit', 'transfer', 'dispense', 'transfer',
    'query-unit', 'transfer', 'transfer', 'query-unit', 'transfer',
]);

const SCENARIO_BANDS = Object.freeze({
    'write-register': 0,
    'write-transfer': 1,
    'write-dispense': 2,
    'read-unit': 3,
    'read-history': 4,
    mixed: 5,
});
const DATASET_BAND_COUNT = 7;

function normalizeSteps(steps) {
    const normalized = [];
    for (let index = 0; index < steps.length; index += 1) {
        const step = steps[index];
        if (step.operation === 'ReceiveTransfer') {
            const dispatch = steps[index - 1];
            if (dispatch?.operation !== 'DispatchTransfer') {
                throw new Error('ReceiveTransfer must immediately follow DispatchTransfer in a dataset recipe');
            }
            normalized.push(normalizeInvocation(step, [dispatch.invokerMspId, step.invokerMspId], {allowPreparation: true}));
        } else {
            normalized.push(normalizeInvocation(step, undefined, {allowPreparation: true}));
        }
    }
    return normalized;
}

function candidatePools(dataset) {
    if (!Array.isArray(dataset?.units)) {
        throw new Error('dataset must contain an units array');
    }
    const pools = {register: [], transfer: [], dispense: [], 'query-unit': [], 'query-history': [], rejection: []};
    const units = [...dataset.units].sort((left, right) => left.sequence - right.sequence);
    for (const unit of units) {
        if (!Number.isInteger(unit.sequence) || !Array.isArray(unit.preparation)) {
            throw new Error('dataset contains an invalid unit recipe');
        }
        const preparation = normalizeSteps(unit.preparation);
        const registration = preparation.find((step) => step.operation === 'RegisterUnit');
        if (unit.expectedSuccess && registration) {
            pools.register.push({
                datasetSequence: unit.sequence,
                type: 'register',
                preparation: [],
                invocations: [registration],
            });
        }
        for (let index = 0; unit.expectedSuccess && index < preparation.length - 1; index += 1) {
            if (preparation[index].operation === 'DispatchTransfer'
                && preparation[index + 1].operation === 'ReceiveTransfer') {
                pools.transfer.push({
                    datasetSequence: unit.sequence,
                    type: 'transfer',
                    preparation: preparation.slice(0, index),
                    invocations: [preparation[index], preparation[index + 1]],
                });
                break;
            }
        }
        if (unit.expectedSuccess?.operation === 'Dispense') {
            const dispense = normalizeInvocation(unit.expectedSuccess);
            const base = {
                datasetSequence: unit.sequence,
                preparation,
                unit: {gtin: dispense.request.gtin, numeroSerie: dispense.request.numeroSerie},
            };
            pools.dispense.push({...base, type: 'dispense', invocations: [dispense]});
            pools['query-unit'].push({...base, type: 'query-unit', invocations: [normalizeInvocation({
                operation: 'ReadUnit', invokerMspId: 'LabMSP', request: base.unit,
            })]});
            pools['query-history'].push({...base, type: 'query-history', invocations: [normalizeInvocation({
                operation: 'GetUnitHistory', invokerMspId: 'LabMSP', request: base.unit,
            })]});
        }
        if (unit.expectedRejection) {
            const rejection = normalizeInvocation({
                operation: unit.expectedRejection.operation,
                invokerMspId: unit.expectedRejection.invokerMspId,
                request: unit.expectedRejection.request,
                privateData: unit.expectedRejection.privateData,
            });
            pools.rejection.push({
                datasetSequence: unit.sequence,
                type: rejection.operation === 'RegisterUnit' ? 'register'
                    : rejection.operation === 'Dispense' ? 'dispense' : 'transfer',
                preparation,
                invocations: [rejection],
                expectedRejection: {
                    family: unit.expectedRejection.category,
                    code: unit.expectedRejection.expectedErrorCode,
                    blockingState: unit.expectedRejection.blockingState,
                },
            });
        }
    }
    return pools;
}

function selectPool(pools, profile) {
    if (profile.scenario === 'write-register') return pools.register;
    if (profile.scenario === 'write-transfer') return pools.transfer;
    if (profile.scenario === 'write-dispense') return pools.dispense;
    if (profile.scenario === 'read-unit') return pools['query-unit'];
    if (profile.scenario === 'read-history') return pools['query-history'];
    if (profile.scenario === 'expected-rejections') {
        const operationName = profile.operation === 'register' ? 'RegisterUnit'
            : profile.operation === 'dispense' ? 'Dispense' : 'DispatchTransfer';
        return pools.rejection.filter((candidate) =>
            candidate.expectedRejection.family === profile.rejectionFamily
            && candidate.invocations[0].operation === operationName);
    }
    throw new Error(`scenario ${profile.scenario} does not use a single pool`);
}

function plannedCalls(profile) {
    const perWorker = Math.ceil(profile.durationSeconds * profile.rate / profile.workers * 1.15) + 20;
    return profile.scenario === 'mixed' ? Math.ceil(perWorker / MIX_PATTERN.length) * MIX_PATTERN.length : perWorker;
}

function copyCandidate(candidate, ordinal) {
    return {
        ordinal,
        datasetSequence: candidate.datasetSequence,
        type: candidate.type,
        invocations: candidate.invocations,
        expectedRejection: candidate.expectedRejection,
    };
}

function scenarioBand(pool, scenario) {
    const bandIndex = SCENARIO_BANDS[scenario];
    if (bandIndex === undefined) {
        throw new Error(`scenario ${scenario} has no deterministic dataset band`);
    }
    const size = Math.floor(pool.length / DATASET_BAND_COUNT);
    if (size === 0) {
        throw new Error(`dataset has insufficient recipes to partition ${scenario}`);
    }
    return {start: bandIndex * size, size};
}

function requiredOrganizations(preparations, workers) {
    const values = new Set();
    for (const preparation of preparations) {
        for (const invocation of preparation.invocations) values.add(invocation.invokerMspId);
    }
    for (const worker of workers) {
        for (const operation of worker.operations) {
            for (const invocation of operation.invocations) values.add(invocation.invokerMspId);
        }
    }
    return [...values].sort();
}

function buildPlan(dataset, profile, seed = 20260727) {
    if (seed !== 20260727) {
        throw new Error('workload seed must be 20260727');
    }
    const pools = candidatePools(dataset);
    const count = plannedCalls(profile);
    const workers = Array.from({length: profile.workers}, (_, workerIndex) => ({workerIndex, operations: []}));

    if (profile.scenario === 'mixed') {
        const total = count * profile.workers;
        const expectedByType = {register: 0, transfer: 0, dispense: 0, 'query-unit': 0};
        for (let index = 0; index < total; index += 1) expectedByType[MIX_PATTERN[index % MIX_PATTERN.length]] += 1;
        const offsets = {};
        let offset = 0;
        for (const type of ['register', 'transfer', 'dispense', 'query-unit']) {
            const band = scenarioBand(pools[type], profile.scenario);
            offsets[type] = band.start + offset;
            offset += expectedByType[type];
            if (offset > band.size) {
                throw new Error(`dataset has insufficient disjoint ${type} recipes for mixed load`);
            }
        }
        const typeIndexes = {register: 0, transfer: 0, dispense: 0, 'query-unit': 0};
        for (const worker of workers) {
            const rotation = (seed + worker.workerIndex) % MIX_PATTERN.length;
            for (let ordinal = 0; ordinal < count; ordinal += 1) {
                const type = MIX_PATTERN[(ordinal + rotation) % MIX_PATTERN.length];
                const candidate = pools[type][offsets[type] + typeIndexes[type]];
                typeIndexes[type] += 1;
                worker.operations.push(copyCandidate(candidate, ordinal));
            }
        }
    } else {
        const pool = selectPool(pools, profile);
        if (pool.length === 0) {
            throw new Error(`dataset has no recipes for ${profile.scenario}`);
        }
        const repeatable = profile.scenario === 'expected-rejections';
        const band = repeatable ? {start: 0, size: pool.length} : scenarioBand(pool, profile.scenario);
        if (!repeatable && band.size < count * profile.workers) {
            throw new Error(`dataset has insufficient unique recipes for ${profile.scenario}`);
        }
        for (const worker of workers) {
            for (let ordinal = 0; ordinal < count; ordinal += 1) {
                const index = worker.workerIndex * count + ordinal;
                worker.operations.push(copyCandidate(pool[band.start + (index % band.size)], ordinal));
            }
        }
    }

    const preparationBySequence = new Map();
    for (const worker of workers) {
        for (const operation of worker.operations) {
            const poolsForType = operation.expectedRejection ? pools.rejection : pools[operation.type];
            const candidate = poolsForType.find((entry) => entry.datasetSequence === operation.datasetSequence
                && (!operation.expectedRejection
                    || (entry.expectedRejection.family === operation.expectedRejection.family
                        && entry.invocations[0].operation === operation.invocations[0].operation)));
            if (candidate?.preparation.length) {
                preparationBySequence.set(candidate.datasetSequence, {
                    datasetSequence: candidate.datasetSequence,
                    invocations: candidate.preparation,
                });
            }
        }
    }
    const preparations = [...preparationBySequence.values()].sort((left, right) => left.datasetSequence - right.datasetSequence);
    return {
        seed,
        scenario: profile.scenario,
        profile,
        preparations,
        workers,
        requiredMspIds: requiredOrganizations(preparations, workers),
    };
}

module.exports = {MIX_PATTERN, SCENARIO_BANDS, buildPlan, candidatePools, normalizeSteps, plannedCalls};
