'use strict';

const fs = require('node:fs');
const path = require('node:path');
const timers = require('node:timers/promises');

const {chainHeight, tallyBlocks} = require('./block-markers');
const {extractContractError} = require('./contract-errors');
const {CHANNEL} = require('./fabric-snapshot');
const {equalUnit, registrationForUnit, stableUnit} = require('./snapshot-plan');

const MAX_RECOVERIES_PER_RECIPE = 12;
const READER_MSP_ID = 'LabMSP';

function unitReference(unit) {
    const registration = registrationForUnit(unit);
    return {gtin: registration.request.gtin, numeroSerie: registration.request.numeroSerie};
}

async function parallel(items, concurrency, operation) {
    let next = 0;
    let firstError;
    await Promise.all(Array.from({length: Math.min(concurrency, Math.max(1, items.length))}, async () => {
        while (!firstError) {
            const index = next;
            next += 1;
            if (index >= items.length) return;
            try {
                await operation(items[index], index);
            } catch (error) {
                firstError ??= error;
            }
        }
    }));
    if (firstError) throw firstError;
}

async function readUnit(pool, reference) {
    try {
        const bytes = await pool.connection(READER_MSP_ID).contract.evaluateTransaction(
            'ReadUnit', reference.gtin, reference.numeroSerie,
        );
        return {present: true, unit: JSON.parse(Buffer.from(bytes).toString('utf8'))};
    } catch (error) {
        if (extractContractError(error)?.code === 'UNIT_NOT_FOUND') return {present: false};
        throw error;
    }
}

// Pasos de la receta ya confirmados: cada paso escribe la clave publica de la
// unidad exactamente una vez, asi que es el largo de su historial.
async function confirmedSteps(pool, reference, steps) {
    let history;
    try {
        const bytes = await pool.connection(READER_MSP_ID).contract.evaluateTransaction(
            'GetUnitHistory', reference.gtin, reference.numeroSerie,
        );
        history = JSON.parse(Buffer.from(bytes).toString('utf8'));
    } catch (error) {
        if (extractContractError(error)?.code === 'UNIT_NOT_FOUND') return 0;
        throw error;
    }
    if (!Array.isArray(history) || history.length > steps.length) {
        throw new Error(`unit ${reference.gtin}/${reference.numeroSerie} has ${history?.length} writes for a ${steps.length}-step recipe`);
    }
    return history.length;
}

function failureCause(error) {
    const envelope = extractContractError(error);
    if (envelope) return envelope.details?.causa ?? envelope.code;
    if (typeof error?.code === 'number') return `GRPC_${error.code}`;
    return error?.message?.match(/committed with status (\w+)/u)?.[1] ?? 'UNCLASSIFIED';
}

// Ejecuta una receta y la reanuda tras cualquier fallo consultando lo que el
// ledger ya confirmo; nunca reenvia un paso sin saber si el anterior quedo.
async function runRecipe(pool, recipe, reference, options = {}) {
    const sleep = options.sleep ?? timers.setTimeout;
    const recoveries = [];
    let next = options.startAt ?? 0;
    while (next < recipe.steps.length) {
        try {
            await pool.invoke(recipe.steps[next]);
            next += 1;
        } catch (error) {
            recoveries.push(failureCause(error));
            if (recoveries.length > MAX_RECOVERIES_PER_RECIPE) {
                throw new Error(`recipe ${recipe.datasetSequence} failed ${recoveries.length} times: ${recoveries.join(', ')}`, {cause: error});
            }
            await sleep(Math.min(5000, 250 * (2 ** Math.min(recoveries.length, 5))));
            next = await confirmedSteps(pool, reference, recipe.steps);
        }
    }
    return recoveries;
}

class ProgressJournal {
    constructor(filePath) {
        this.filePath = filePath;
        this.completed = new Set();
        if (fs.existsSync(filePath)) {
            for (const line of fs.readFileSync(filePath, 'utf8').split('\n')) {
                if (line.trim()) this.completed.add(JSON.parse(line).sequence);
            }
        }
    }

    record(sequence) {
        fs.appendFileSync(this.filePath, `${JSON.stringify({sequence})}\n`, {mode: 0o600});
        this.completed.add(sequence);
    }
}

async function executeRecipes({pool, dataset, recipes, directory, concurrency, log}) {
    const units = new Map(dataset.units.map((unit) => [unit.sequence, unit]));
    const journal = new ProgressJournal(path.join(directory, 'progress.jsonl'));
    const pending = recipes.filter((recipe) => !journal.completed.has(recipe.datasetSequence));
    const recoveriesByCause = {};
    const resumed = journal.completed.size > 0;
    let recoveredRecipes = 0;
    let done = recipes.length - pending.length;
    log(`snapshot: ${done}/${recipes.length} recetas ya confirmadas, ${pending.length} pendientes, concurrencia ${concurrency}`);
    await parallel(pending, concurrency, async (recipe) => {
        const reference = unitReference(units.get(recipe.datasetSequence));
        // Una receta interrumpida en una sesion anterior retoma desde el ledger.
        const startAt = resumed ? await confirmedSteps(pool, reference, recipe.steps) : 0;
        const recoveries = await runRecipe(pool, recipe, reference, {startAt});
        if (recoveries.length > 0) recoveredRecipes += 1;
        for (const cause of recoveries) recoveriesByCause[cause] = (recoveriesByCause[cause] ?? 0) + 1;
        journal.record(recipe.datasetSequence);
        done += 1;
        if (done % 1000 === 0 || done === recipes.length) log(`snapshot: ${done}/${recipes.length} recetas confirmadas`);
    });
    return {recipes: recipes.length, executedThisSession: pending.length, recoveredRecipes, recoveriesByCause};
}

// Verificacion semantica completa (D2): las 50.000 unidades contra el estado
// derivado de la maquina de estados, no contra lo que devolvio el ledger.
async function verifyPreconditions({pool, dataset, preconditions, concurrency, sequences}) {
    const units = new Map(dataset.units.map((unit) => [unit.sequence, unit]));
    const selected = sequences ?? preconditions.map((entry) => entry.sequence);
    const expectedBySequence = new Map(preconditions.map((entry) => [entry.sequence, entry.unit]));
    const mismatches = [];
    let present = 0;
    let absent = 0;
    await parallel(selected, concurrency, async (sequence) => {
        if (!expectedBySequence.has(sequence)) throw new Error(`snapshot omits precondition for sequence ${sequence}`);
        const expected = expectedBySequence.get(sequence);
        const observed = await readUnit(pool, unitReference(units.get(sequence)));
        if (expected === null) {
            if (observed.present) mismatches.push({sequence, expected: null, observed: stableUnit(observed.unit)});
            else absent += 1;
        } else if (!observed.present || !equalUnit(observed.unit, expected)) {
            mismatches.push({sequence, expected, observed: observed.present ? stableUnit(observed.unit) : null});
        } else {
            present += 1;
        }
    });
    mismatches.sort((left, right) => left.sequence - right.sequence);
    return {checked: selected.length, present, absent, mismatches};
}

async function currentHeight(pool) {
    return chainHeight(pool.connection(READER_MSP_ID).gateway.getNetwork(CHANNEL));
}

async function countMarkers(pool, startBlock, endBlock) {
    return tallyBlocks(pool.connection(READER_MSP_ID).gateway.getNetwork(CHANNEL), startBlock, endBlock);
}

// Metadata de la fase dataset-preparation (secciones 3.5, 4 y 9.6). DES-20 fija
// la operacion `register` con una transaccion por operacion; con el snapshot
// de D1 la preparacion tambien despacha y recibe, asi que la tasa se expresa en
// transacciones confirmadas y el desglose por funcion queda en la nota. La
// construccion no esta regulada por tasa: la objetivo declarada es la observada.
function buildPreparationMetadata({
    context, recipes, transactions, transactionsByFunction, durationSeconds, startedAt, endedAt, markers, concurrency,
}) {
    const transactionsPerSecond = transactions / durationSeconds;
    const breakdown = Object.entries(transactionsByFunction)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([name, count]) => `${name} ${count}`)
        .join(', ');
    return {
        $schema: 'urn:pfi-snt:run-metadata:schema:1.0.0',
        schemaVersion: '1.0.0',
        protocol: 'measurement-protocol',
        repositoryCommit: context.repositoryCommit,
        sut: 'fabric',
        scenario: 'dataset-preparation',
        phase: 'preparation',
        repetition: 1,
        dataset: context.dataset,
        workers: concurrency,
        durationSeconds,
        rate: {
            operation: 'register',
            transactionsPerOperation: 1,
            targetOperationsPerSecond: transactionsPerSecond,
            targetTransactionsPerSecond: transactionsPerSecond,
            effectiveTransactionsPerSecond: transactionsPerSecond,
        },
        participationMarkers: {
            expected: markers.expected,
            observed: markers.observed,
            fromRegistrations: markers.fromRegistrations,
            fromRegulatoryEvents: markers.fromRegulatoryEvents,
            successfulWriteTransactions: markers.successfulWriteTransactions,
            perSecond: markers.observed / durationSeconds,
            shareOfSuccessfulWrites: markers.observed / markers.successfulWriteTransactions,
        },
        startedAt,
        endedAt,
        host: context.host,
        environment: context.environment,
        notes: `Construccion del snapshot logico inicial: ${recipes} recetas y ${transactions} transacciones confirmadas (${breakdown}) con ${concurrency} recetas concurrentes. La tasa se expresa en transacciones: DES-20 modela la preparacion como altas de una transaccion. No esta regulada por tasa; la objetivo declarada es la observada. No se mezcla con las rondas medidas.`.slice(0, 2000),
    };
}

module.exports = {
    READER_MSP_ID,
    buildPreparationMetadata,
    confirmedSteps,
    countMarkers,
    currentHeight,
    executeRecipes,
    failureCause,
    parallel,
    readUnit,
    runRecipe,
    unitReference,
    verifyPreconditions,
};
