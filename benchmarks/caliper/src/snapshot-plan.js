'use strict';

const {buildPlan, candidatePools} = require('./planner');
const {PROFILES, buildProfile} = require('./profiles');

const EXPECTED_COUNTS = Object.freeze({
    total: 50_000,
    participating: 25_183,
    unregistered: 3_080,
    prepared: 22_103,
    filler: 24_817,
    registered: 46_920,
});

const REGULATORY_MARKER_OPERATIONS = new Set([
    'ProhibitProduct',
    'Quarantine',
    'ReleaseProduct',
    'ReportDamaged',
    'ReportExpired',
    'ReturnProduct',
    'WithdrawFromMarket',
]);

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

function registrationForUnit(unit) {
    const invocation = unit?.preparation?.[0];
    if (invocation?.operation !== 'RegisterUnit') {
        throw new Error(`dataset sequence ${unit?.sequence ?? 'unknown'} does not start with RegisterUnit`);
    }
    return invocation;
}

function expectedMarkerWrites(invocation) {
    if (invocation.operation === 'RegisterUnit') return {registrations: 1, regulatoryEvents: 0, total: 1};
    if (invocation.invokerMspId === 'AnmatMSP' && REGULATORY_MARKER_OPERATIONS.has(invocation.operation)) {
        return {registrations: 0, regulatoryEvents: 1, total: 1};
    }
    return {registrations: 0, regulatoryEvents: 0, total: 0};
}

function canonicalRecipe(unit, preparation = []) {
    const registration = registrationForUnit(unit);
    const remaining = preparation[0]?.operation === 'RegisterUnit' ? preparation.slice(1) : preparation;
    return [registration, ...remaining];
}

function expectedLogicalState(unit, invocations) {
    if (invocations.length === 0) return {present: false, state: null, custodian: null};
    let state;
    let custodian;
    let pendingDestination;
    for (const invocation of invocations) {
        switch (invocation.operation) {
        case 'RegisterUnit':
            state = unit.initialState;
            custodian = unit.initialCustodian;
            break;
        case 'DispatchTransfer':
            if (!invocation.privateData?.destinatario?.destino) {
                throw new Error(`dataset sequence ${unit.sequence} dispatch omits its private destination`);
            }
            pendingDestination = invocation.privateData.destinatario.destino;
            state = 'EN_TRANSITO';
            break;
        case 'ReceiveTransfer':
            if (!pendingDestination) throw new Error(`dataset sequence ${unit.sequence} receives without a dispatch`);
            custodian = pendingDestination;
            pendingDestination = undefined;
            state = 'EN_CUSTODIA';
            break;
        default:
            if (!invocation.transitionId || !unit.expectedRejection?.blockingState) {
                throw new Error(`dataset sequence ${unit.sequence} has no logical result for ${invocation.operation}`);
            }
            state = unit.expectedRejection.blockingState;
        }
    }
    if (!state || !custodian) throw new Error(`dataset sequence ${unit.sequence} has an incomplete logical result`);
    return {present: true, state, custodian};
}

function privateIndicators(invocations) {
    let dispatches = 0;
    let receipts = 0;
    let markers = 0;
    let activeTransfers = 0;
    for (const invocation of invocations) {
        markers += expectedMarkerWrites(invocation).total;
        if (invocation.operation === 'DispatchTransfer') {
            dispatches += 1;
            activeTransfers += 1;
        }
        if (invocation.operation === 'ReceiveTransfer') {
            receipts += 1;
            activeTransfers -= 1;
        }
    }
    if (activeTransfers < 0 || activeTransfers > 1) {
        throw new Error('logical recipe has an invalid active-transfer balance');
    }
    return {
        participationMarkers: markers,
        dispatches,
        closedTransfers: receipts,
        activeTransfer: activeTransfers === 1,
    };
}

function buildGoldenSnapshotPlan(dataset, seed = 20260727, options = {}) {
    if (!Array.isArray(dataset?.units)) throw new Error('dataset must contain an units array');
    const pools = candidatePools(dataset);
    const plans = profileMatrix().map((profile) => buildPlan(dataset, profile, seed, {pools}));
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
    const unitBySequence = new Map();
    for (const unit of dataset.units) {
        if (!Number.isInteger(unit.sequence) || unitBySequence.has(unit.sequence)) {
            throw new Error('dataset contains an invalid or duplicate sequence');
        }
        unitBySequence.set(unit.sequence, unit);
    }
    const allSequences = [...unitBySequence.keys()].sort((left, right) => left - right);
    const fillerSequences = allSequences.filter((sequence) => !participantSequences.has(sequence));
    const excludedSequences = [...unregisteredSequences].sort((left, right) => left - right);
    const preparedSequences = [...participantSequences]
        .filter((sequence) => !unregisteredSequences.has(sequence))
        .sort((left, right) => left - right);
    const registeredSequences = allSequences.filter((sequence) => !unregisteredSequences.has(sequence));
    const counts = {
        total: dataset.units.length,
        participating: participantSequences.size,
        unregistered: excludedSequences.length,
        prepared: preparedSequences.length,
        filler: fillerSequences.length,
        registered: registeredSequences.length,
    };
    if (options.assertCanonical !== false) {
        for (const [name, expected] of Object.entries(EXPECTED_COUNTS)) {
            if (counts[name] !== expected) {
                throw new Error(`canonical snapshot ${name} count: expected ${expected}, got ${counts[name]}`);
            }
        }
    }
    if (fillerSequences.length === 0) throw new Error('snapshot has no filler unit for read-only smoke');

    const prepared = new Set(preparedSequences);
    const registered = new Set(registeredSequences);
    const units = allSequences.map((datasetSequence) => {
        const unit = unitBySequence.get(datasetSequence);
        const invocations = registered.has(datasetSequence)
            ? canonicalRecipe(unit, preparations.get(datasetSequence) ?? []) : [];
        return {
            datasetSequence,
            role: registered.has(datasetSequence)
                ? (prepared.has(datasetSequence) ? 'prepared' : 'filler') : 'measured-registration',
            expected: expectedLogicalState(unit, invocations),
            recipe: invocations,
            private: privateIndicators(invocations),
        };
    });
    const expectedMarkers = units.flatMap((entry) => entry.recipe).reduce((totals, invocation) => {
        const expected = expectedMarkerWrites(invocation);
        totals.registrations += expected.registrations;
        totals.regulatoryEvents += expected.regulatoryEvents;
        totals.total += expected.total;
        return totals;
    }, {registrations: 0, regulatoryEvents: 0, total: 0});

    return {
        schemaVersion: '1.0.0',
        seed,
        counts,
        excludedSequences,
        preparedSequences,
        fillerSequences,
        smokeSequence: fillerSequences[0],
        expectedMarkers,
        units,
    };
}

module.exports = {
    EXPECTED_COUNTS,
    REGULATORY_MARKER_OPERATIONS,
    buildGoldenSnapshotPlan,
    canonicalRecipe,
    expectedMarkerWrites,
    expectedLogicalState,
    privateIndicators,
    profileMatrix,
    registrationForUnit,
};
