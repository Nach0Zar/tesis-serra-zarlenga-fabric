'use strict';

const {buildPlan, normalizeSteps} = require('./planner');
const {PROFILES, buildProfile} = require('./profiles');

// Union deterministica de las veinte combinaciones de la seccion 6 sobre el
// bundle canonico. Es la misma union que usa la baseline (#139): ambos SUT
// parten del mismo snapshot logico (seccion 5).
const EXPECTED_COUNTS = Object.freeze({
    total: 50_000,
    participating: 25_183,
    unregistered: 3_080,
    prepared: 22_103,
    filler: 24_817,
});

const STATE_BY_OPERATION = Object.freeze({
    RegisterUnit: 'EN_LABORATORIO',
    DispatchTransfer: 'EN_TRANSITO',
    ReceiveTransfer: 'EN_CUSTODIA',
    Quarantine: 'EN_CUARENTENA',
    ReportExpired: 'VENCIDO',
    ReportDamaged: 'DETERIORADO',
    WithdrawFromMarket: 'RETIRADO_MERCADO',
    ProhibitProduct: 'PROHIBIDO',
    ReturnProduct: 'DEVUELTO',
});
const REGULATOR_MSP_ID = 'AnmatMSP';
const STABLE_UNIT_FIELDS = Object.freeze(['gtin', 'numeroSerie', 'lote', 'fechaVencimiento', 'custodioActual', 'estado']);

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

function registrationForUnit(unit) {
    const invocation = unit.preparation?.[0];
    if (invocation?.operation !== 'RegisterUnit') {
        throw new Error(`dataset sequence ${unit.sequence} does not start with RegisterUnit`);
    }
    return invocation;
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
            if (existing && JSON.stringify(existing) !== JSON.stringify(preparation.invocations)) {
                throw new Error(`conflicting snapshot preparations for dataset sequence ${preparation.datasetSequence}`);
            }
            preparations.set(preparation.datasetSequence, preparation.invocations);
        }
    }
    for (const sequence of unregisteredSequences) {
        if (preparations.has(sequence)) throw new Error(`unregistered sequence ${sequence} also requires preparation`);
    }

    const allSequences = dataset.units.map((unit) => unit.sequence).sort((left, right) => left - right);
    const fillerSequences = allSequences.filter((sequence) => !participantSequences.has(sequence));
    const excludedSequences = [...unregisteredSequences].sort((left, right) => left - right);
    const preparedSequences = [...participantSequences]
        .filter((sequence) => !unregisteredSequences.has(sequence))
        .sort((left, right) => left - right);
    const counts = {
        total: dataset.units.length,
        participating: participantSequences.size,
        unregistered: excludedSequences.length,
        prepared: preparedSequences.length,
        filler: fillerSequences.length,
    };
    if (options.assertCanonical !== false) {
        for (const [name, expected] of Object.entries(EXPECTED_COUNTS)) {
            if (counts[name] !== expected) {
                throw new Error(`canonical snapshot ${name} count: expected ${expected}, got ${counts[name]}`);
            }
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

// Receta que deja cada unidad en su estado previo a las rondas: nada para las
// altas medidas, solo el alta para el relleno y la preparacion completa para
// las que participan con un estado previo.
function snapshotRecipes(dataset, snapshotPlan) {
    const bySequence = new Map(dataset.units.map((unit) => [unit.sequence, unit]));
    const prepared = new Map(snapshotPlan.preparations.map((entry) => [entry.datasetSequence, entry.invocations]));
    const excluded = new Set(snapshotPlan.excludedSequences);
    const recipes = [];
    for (const sequence of [...bySequence.keys()].sort((left, right) => left - right)) {
        if (excluded.has(sequence)) continue;
        const unit = bySequence.get(sequence);
        const registration = registrationForUnit(unit);
        const steps = prepared.get(sequence) ?? [];
        recipes.push({
            datasetSequence: sequence,
            steps: steps.length > 0 ? steps : normalizeSteps([registration]),
        });
    }
    for (const recipe of recipes) {
        if (recipe.steps[0]?.operation !== 'RegisterUnit') {
            throw new Error(`snapshot recipe ${recipe.datasetSequence} does not start with RegisterUnit`);
        }
    }
    return recipes;
}

// Estado publico que la receta deja confirmado, derivado de la maquina de
// estados (ADR-001) sin leer el ledger: el custodio solo cambia en la
// recepcion (ADR-004) y nunca en un evento extraordinario.
function expectedUnitAfter(unit, steps) {
    const registration = registrationForUnit(unit);
    if (steps.length === 0) return null;
    let custodian = unit.initialCustodian;
    let state;
    let pendingDestination;
    for (const step of steps) {
        state = STATE_BY_OPERATION[step.operation];
        if (!state) throw new Error(`no expected state for ${step.operation} in sequence ${unit.sequence}`);
        if (step.operation === 'DispatchTransfer') {
            pendingDestination = step.privateData?.destinatario?.destino;
            if (!pendingDestination) throw new Error(`dispatch without destination in sequence ${unit.sequence}`);
        } else if (step.operation === 'ReceiveTransfer') {
            if (!pendingDestination) throw new Error(`receive without dispatch in sequence ${unit.sequence}`);
            custodian = pendingDestination;
            pendingDestination = undefined;
        }
    }
    return {
        gtin: registration.request.gtin,
        numeroSerie: registration.request.numeroSerie,
        lote: registration.request.lote,
        fechaVencimiento: registration.request.fechaVencimiento,
        custodioActual: custodian,
        estado: state,
    };
}

// Precondiciones en el mismo formato que la baseline: una entrada por unidad del
// bundle, `null` para las ausentes. Dos snapshots del mismo estado logico
// producen el mismo archivo byte a byte.
function expectedPreconditions(dataset, snapshotPlan) {
    const recipes = new Map(snapshotRecipes(dataset, snapshotPlan).map((recipe) => [recipe.datasetSequence, recipe.steps]));
    return [...dataset.units]
        .sort((left, right) => left.sequence - right.sequence)
        .map((unit) => ({sequence: unit.sequence, unit: expectedUnitAfter(unit, recipes.get(unit.sequence) ?? [])}));
}

// Marcadores que la preparacion debe confirmar (seccion 3.5): uno por alta en la
// coleccion implicita del laboratorio y uno por evento iniciado por el regulador.
function expectedPreparationMarkers(recipes) {
    let fromRegistrations = 0;
    let fromRegulatoryEvents = 0;
    for (const recipe of recipes) {
        for (const step of recipe.steps) {
            if (step.operation === 'RegisterUnit') fromRegistrations += 1;
            else if (step.invokerMspId === REGULATOR_MSP_ID) fromRegulatoryEvents += 1;
        }
    }
    return {expected: fromRegistrations + fromRegulatoryEvents, fromRegistrations, fromRegulatoryEvents};
}

function stableUnit(value) {
    return Object.fromEntries(STABLE_UNIT_FIELDS.map((field) => [field, value?.[field]]));
}

function equalUnit(left, right) {
    return STABLE_UNIT_FIELDS.every((field) => left?.[field] === right?.[field]);
}

module.exports = {
    EXPECTED_COUNTS,
    REGULATOR_MSP_ID,
    STABLE_UNIT_FIELDS,
    buildGoldenSnapshotPlan,
    equalUnit,
    expectedPreconditions,
    expectedPreparationMarkers,
    expectedUnitAfter,
    profileMatrix,
    registrationForUnit,
    snapshotRecipes,
    stableUnit,
};
