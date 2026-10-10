'use strict';

const fs = require('node:fs');
const path = require('node:path');
const timers = require('node:timers/promises');

const grpc = require('@grpc/grpc-js');

const {extractContractError} = require('./contract-errors');
const {PREPARATION_GATEWAY_TIMEOUT_MS, invokePreparation} = require('./gateway-bridge');
const {writeJSONAtomic} = require('./metadata');
const {summarizePrivateWrites} = require('./rwset');
const {registrationForUnit} = require('./snapshot-plan');

const STABLE_UNIT_FIELDS = Object.freeze([
    'gtin', 'numeroSerie', 'lote', 'fechaVencimiento', 'custodioActual', 'estado',
]);
const TRANSIENT_GRPC_CODES = new Set([grpc.status.CANCELLED, grpc.status.DEADLINE_EXCEEDED, grpc.status.UNAVAILABLE]);

function stableUnit(value) {
    if (!value) return null;
    return Object.fromEntries(STABLE_UNIT_FIELDS.map((field) => [field, value[field]]));
}

function equalUnit(left, right) {
    return STABLE_UNIT_FIELDS.every((field) => left?.[field] === right?.[field]);
}

function committedUnit(result) {
    try {
        const value = JSON.parse(Buffer.from(result).toString('utf8'));
        const unit = stableUnit(value);
        return STABLE_UNIT_FIELDS.every((field) => typeof unit[field] === 'string') ? unit : null;
    } catch {
        return null;
    }
}

function stableHistory(history) {
    return history.map((entry) => ({
        txId: entry.txId,
        isDelete: entry.isDelete === true,
        value: entry.value ? stableUnit(entry.value) : null,
    }));
}

function unitReference(unit) {
    const registration = registrationForUnit(unit);
    return {
        gtin: registration.request.gtin,
        numeroSerie: registration.request.numeroSerie,
        invokerMspId: registration.invokerMspId,
    };
}

function progressReaderMspId(gatewayPool, unit) {
    const available = gatewayPool.organizations instanceof Map
        ? [...gatewayPool.organizations.keys()].sort((left, right) => left.localeCompare(right))
        : [];
    if (available.length === 0) return unitReference(unit).invokerMspId;
    return available[(unit.sequence - 1) % available.length];
}

function isTransientGrpcError(error) {
    return TRANSIENT_GRPC_CODES.has(error?.code)
        || /\b(?:CANCELLED|DEADLINE_EXCEEDED|UNAVAILABLE)\b/u.test(error?.message ?? '');
}

async function readProgress(gatewayPool, unit) {
    const reference = unitReference(unit);
    const readerMspId = progressReaderMspId(gatewayPool, unit);
    try {
        const options = {timeoutMs: PREPARATION_GATEWAY_TIMEOUT_MS};
        const current = await gatewayPool.readUnit(readerMspId, reference, options);
        const history = await gatewayPool.getUnitHistory(readerMspId, reference, options);
        return {present: true, current: stableUnit(current), history};
    } catch (error) {
        const envelope = extractContractError(error);
        if (envelope?.code === 'UNIT_NOT_FOUND') return {present: false, current: null, history: []};
        throw error;
    }
}

class PreparationJournal {
    constructor(filePath) {
        this.filePath = filePath;
        this.events = [];
        this.durableQueue = [];
        this.flushScheduled = false;
        this.flushing = false;
        if (fs.existsSync(filePath)) {
            const contents = fs.readFileSync(filePath, 'utf8').trim();
            if (contents !== '') {
                for (const line of contents.split(/\r?\n/u)) this.events.push(JSON.parse(line));
            }
        }
    }

    entry(event) {
        return {...event, recordedAt: new Date().toISOString()};
    }

    appendEntry(entry) {
        fs.mkdirSync(path.dirname(this.filePath), {recursive: true});
        const descriptor = fs.openSync(this.filePath, 'a', 0o600);
        try {
            fs.writeSync(descriptor, `${JSON.stringify(entry)}\n`);
            fs.fsyncSync(descriptor);
        } finally {
            fs.closeSync(descriptor);
        }
        this.events.push(entry);
        return entry;
    }

    append(event) {
        if (this.flushing || this.durableQueue.length > 0) {
            throw new Error('synchronous journal append cannot run during a durable group commit');
        }
        return this.appendEntry(this.entry(event));
    }

    appendDurable(event) {
        const entry = this.entry(event);
        const promise = new Promise((resolve, reject) => {
            this.durableQueue.push({entry, resolve, reject});
        });
        if (!this.flushScheduled && !this.flushing) {
            this.flushScheduled = true;
            setImmediate(() => this.flushDurable());
        }
        return promise;
    }

    async flushDurable() {
        this.flushScheduled = false;
        if (this.flushing) return;
        this.flushing = true;
        while (this.durableQueue.length > 0) {
            const batch = this.durableQueue.splice(0);
            try {
                await fs.promises.mkdir(path.dirname(this.filePath), {recursive: true});
                const descriptor = await fs.promises.open(this.filePath, 'a', 0o600);
                try {
                    await descriptor.writeFile(batch.map(({entry}) => `${JSON.stringify(entry)}\n`).join(''));
                    await descriptor.sync();
                } finally {
                    await descriptor.close();
                }
                this.events.push(...batch.map(({entry}) => entry));
                for (const {entry, resolve} of batch) resolve(entry);
            } catch (error) {
                for (const {reject} of batch) reject(error);
            }
        }
        this.flushing = false;
        if (this.durableQueue.length > 0 && !this.flushScheduled) {
            this.flushScheduled = true;
            setImmediate(() => this.flushDurable());
        }
    }

    forSequence(sequence) {
        return this.events.filter((event) => event.datasetSequence === sequence);
    }

    committed() {
        const byTransaction = new Map();
        for (const event of this.events) {
            if (['committed', 'recovered'].includes(event.status)) byTransaction.set(event.transactionId, event);
        }
        return [...byTransaction.values()];
    }
}

function stepRecords(events) {
    const committed = new Map();
    const pending = new Map();
    const endorsed = new Map();
    for (const event of events) {
        if (event.status === 'pending') {
            const values = pending.get(event.step) ?? [];
            values.push(event);
            pending.set(event.step, values);
        }
        if (event.status === 'endorsed') endorsed.set(event.transactionId, event);
        if (['committed', 'recovered'].includes(event.status)) committed.set(event.step, event);
    }
    return {committed, pending, endorsed};
}

function assertHistoryPrefix(sequence, history, committed) {
    for (const [step, event] of [...committed.entries()].sort(([left], [right]) => left - right)) {
        if (step >= history.length || history[step]?.txId !== event.transactionId) {
            throw new Error(`dataset sequence ${sequence} history diverges at recipe step ${step}`);
        }
    }
}

async function recoverPending(sequence, records, progress, journal) {
    if (progress.history.length !== records.committed.size + 1) return false;
    const step = records.committed.size;
    const observed = progress.history[step];
    const pending = (records.pending.get(step) ?? []).find((event) => event.transactionId === observed?.txId);
    if (!pending || observed?.txId !== pending.transactionId) {
        throw new Error(`dataset sequence ${sequence} has an unrecognized committed transaction at recipe step ${step}`);
    }
    const endorsed = records.endorsed.get(pending.transactionId);
    await journal.appendDurable({
        datasetSequence: sequence,
        step,
        operation: pending.operation,
        status: 'recovered',
        transactionId: pending.transactionId,
        privateWrites: endorsed?.privateWrites ?? {collections: [], markerWrites: 0, pairPrivateWrites: 0},
        unit: stableUnit(observed.value),
    });
    return true;
}

async function waitForCommittedStep({
    gatewayPool,
    unit,
    step,
    transactionId,
    timeoutMs = 30_000,
    pollMs = 100,
    sleep = timers.setTimeout,
}) {
    const deadline = Date.now() + timeoutMs;
    while (true) {
        try {
            const progress = await readProgress(gatewayPool, unit);
            const observed = progress.history[step];
            if (observed) {
                if (observed.txId !== transactionId || progress.history.length !== step + 1) {
                    throw new Error(`dataset sequence ${unit.sequence} changed unexpectedly at recipe step ${step}`);
                }
                return progress;
            }
            if (progress.history.length !== step) {
                throw new Error(`dataset sequence ${unit.sequence} history changed unexpectedly while waiting for step ${step}`);
            }
        } catch (error) {
            if (!isTransientGrpcError(error)) throw error;
            if (Date.now() >= deadline) {
                throw new Error(`dataset sequence ${unit.sequence} could not read recipe step ${step} before the visibility deadline`, {cause: error});
            }
        }
        if (Date.now() >= deadline) {
            throw new Error(`dataset sequence ${unit.sequence} did not observe recipe step ${step} before the visibility deadline`);
        }
        await sleep(Math.min(pollMs, Math.max(0, deadline - Date.now())));
    }
}

async function invokeRecipeStep({
    gatewayPool,
    journal,
    sequence,
    step,
    invocation,
    maxAttempts,
    retryDelayMs,
    sleep,
    invoke = invokePreparation,
    summarize = summarizePrivateWrites,
}) {
    for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
        let transactionId;
        try {
            const result = await invoke(gatewayPool, invocation, {
                onTransactionId: async (id) => {
                    transactionId = id;
                    await journal.appendDurable({
                        datasetSequence: sequence, step, operation: invocation.operation,
                        status: 'pending', transactionId: id, attempt,
                    });
                },
                onEndorsed: async ({transactionId: id, transactionBytes}) => {
                    await journal.appendDurable({
                        datasetSequence: sequence, step, operation: invocation.operation,
                        status: 'endorsed', transactionId: id, attempt,
                        privateWrites: summarize(transactionBytes),
                    });
                },
            });
            const endorsed = stepRecords(journal.forSequence(sequence)).endorsed.get(result.transactionId);
            return {
                transactionId: result.transactionId,
                privateWrites: endorsed?.privateWrites ?? summarize(result.transactionBytes),
                result: result.result,
                recovered: false,
            };
        } catch (error) {
            if (!isTransientGrpcError(error)) throw error;
            const reconciliationUnit = {sequence, preparation: [
                {operation: 'RegisterUnit', invokerMspId: invocation.invokerMspId, request: invocation.request},
            ]};
            const records = stepRecords(journal.forSequence(sequence));
            const ambiguousTransactionId = error.transactionId ?? transactionId;
            const endorsed = records.endorsed.get(ambiguousTransactionId);
            let progress;
            if (endorsed) {
                progress = await waitForCommittedStep({
                    gatewayPool,
                    unit: reconciliationUnit,
                    step,
                    transactionId: ambiguousTransactionId,
                    timeoutMs: 30_000,
                    pollMs: retryDelayMs,
                    sleep,
                });
            } else {
                progress = await readProgress(gatewayPool, reconciliationUnit);
            }
            if (progress.history.length === step + 1 && progress.history[step]?.txId === ambiguousTransactionId) {
                return {
                    transactionId: ambiguousTransactionId,
                    privateWrites: endorsed?.privateWrites ?? {collections: [], markerWrites: 0, pairPrivateWrites: 0},
                    result: Buffer.from(JSON.stringify(progress.current)),
                    recovered: true,
                };
            }
            if (progress.history.length !== step) {
                throw new Error(`dataset sequence ${sequence} changed unexpectedly after transient ${invocation.operation}`);
            }
            if (attempt === maxAttempts) {
                throw new Error(`dataset sequence ${sequence}/${invocation.operation} exceeded ${maxAttempts} transient attempts`, {cause: error});
            }
            await sleep(Math.min(retryDelayMs * attempt, 10_000));
        }
    }
    throw new Error('unreachable recipe retry state');
}

async function prepareRecipe({
    gatewayPool,
    journal,
    unit,
    recipe,
    maxAttempts = 5,
    retryDelayMs = 1000,
    visibilityTimeoutMs = 30_000,
    visibilityPollMs = 100,
    sleep = timers.setTimeout,
    invoke = invokePreparation,
    summarize = summarizePrivateWrites,
}) {
    let progress = await readProgress(gatewayPool, unit);
    let records = stepRecords(journal.forSequence(unit.sequence));
    assertHistoryPrefix(unit.sequence, progress.history, records.committed);
    if (await recoverPending(unit.sequence, records, progress, journal)) {
        records = stepRecords(journal.forSequence(unit.sequence));
    }
    if (progress.history.length !== records.committed.size) {
        throw new Error(`dataset sequence ${unit.sequence} ledger history does not match its durable journal`);
    }
    for (let step = records.committed.size; step < recipe.invocations.length; step += 1) {
        const invocation = recipe.invocations[step];
        const previousInvocation = recipe.invocations[step - 1];
        if (previousInvocation && previousInvocation.invokerMspId !== invocation.invokerMspId && progress.current) {
            await gatewayPool.waitForUnitState(
                [invocation.invokerMspId],
                invocation.request,
                progress.current.estado,
                PREPARATION_GATEWAY_TIMEOUT_MS,
            );
        }
        const result = await invokeRecipeStep({
            gatewayPool, journal, sequence: unit.sequence, step, invocation, maxAttempts, retryDelayMs, sleep,
            invoke, summarize,
        });
        const resultUnit = committedUnit(result.result);
        progress = resultUnit
            ? {present: true, current: resultUnit, history: []}
            : await waitForCommittedStep({
                gatewayPool,
                unit,
                step,
                transactionId: result.transactionId,
                timeoutMs: visibilityTimeoutMs,
                pollMs: visibilityPollMs,
                sleep,
            });
        await journal.appendDurable({
            datasetSequence: unit.sequence,
            step,
            operation: invocation.operation,
            status: result.recovered ? 'recovered' : 'committed',
            transactionId: result.transactionId,
            privateWrites: result.privateWrites,
            unit: progress.current,
        });
    }
    return progress;
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

function journalStatistics(journal) {
    const committed = journal.committed();
    const registrations = committed.filter((entry) => entry.operation === 'RegisterUnit')
        .reduce((sum, entry) => sum + (entry.privateWrites?.markerWrites ?? 0), 0);
    const regulatoryEvents = committed.filter((entry) => entry.operation !== 'RegisterUnit')
        .reduce((sum, entry) => sum + (entry.privateWrites?.markerWrites ?? 0), 0);
    return {
        successfulWriteTransactions: committed.length,
        markers: {
            observed: registrations + regulatoryEvents,
            fromRegistrations: registrations,
            fromRegulatoryEvents: regulatoryEvents,
        },
        pairPrivateWrites: committed.reduce((sum, entry) => sum + (entry.privateWrites?.pairPrivateWrites ?? 0), 0),
    };
}

function activeTransferProbe(unit) {
    for (let index = 0; index < unit.preparation.length - 1; index += 1) {
        if (unit.preparation[index].operation === 'DispatchTransfer'
            && unit.preparation[index + 1].operation === 'ReceiveTransfer') {
            return unit.preparation[index + 1];
        }
    }
    return undefined;
}

async function verifySnapshotState({gatewayPool, dataset, preconditions, logicalPlan, concurrency = 64}) {
    const expected = new Map(preconditions.map((entry) => [entry.sequence, entry]));
    if (expected.size !== dataset.units.length) throw new Error('snapshot preconditions do not cover the complete dataset');
    const logical = new Map((logicalPlan?.units ?? []).map((entry) => [entry.datasetSequence, entry]));
    if (logical.size !== dataset.units.length) throw new Error('logical plan does not cover the complete dataset');
    const result = {present: 0, absent: 0, historyEntries: 0, activePrivateDataProbes: 0};
    await parallel(dataset.units, concurrency, async (unit) => {
        const precondition = expected.get(unit.sequence);
        const logicalUnit = logical.get(unit.sequence);
        if (!precondition) throw new Error(`snapshot omits precondition for sequence ${unit.sequence}`);
        if (!logicalUnit) throw new Error(`logical plan omits sequence ${unit.sequence}`);
        if (logicalUnit.expected.present !== (precondition.unit !== null)) {
            throw new Error(`dataset sequence ${unit.sequence} precondition contradicts the logical plan`);
        }
        const progress = await readProgress(gatewayPool, unit);
        if (!logicalUnit.expected.present) {
            if (progress.present) throw new Error(`dataset sequence ${unit.sequence} should be absent from the snapshot`);
            result.absent += 1;
            return;
        }
        if (!progress.present || !equalUnit(progress.current, precondition.unit)) {
            throw new Error(`dataset sequence ${unit.sequence} public state differs from its snapshot precondition`);
        }
        if (progress.current.estado !== logicalUnit.expected.state
            || progress.current.custodioActual !== logicalUnit.expected.custodian) {
            throw new Error(`dataset sequence ${unit.sequence} state or custodian differs from the logical plan`);
        }
        if (JSON.stringify(stableHistory(progress.history)) !== JSON.stringify(precondition.history)) {
            throw new Error(`dataset sequence ${unit.sequence} history differs from its snapshot precondition`);
        }
        if (precondition.unit.estado === 'EN_TRANSITO') {
            const probe = activeTransferProbe(unit);
            if (!probe) throw new Error(`dataset sequence ${unit.sequence} is in transit without a receiver probe`);
            await gatewayPool.evaluate(probe, {mspId: probe.invokerMspId});
            result.activePrivateDataProbes += 1;
        }
        result.present += 1;
        result.historyEntries += progress.history.length;
    });
    return result;
}

async function buildPreconditions({gatewayPool, dataset, excludedSequences, concurrency = 64}) {
    const excluded = new Set(excludedSequences);
    const entries = new Array(dataset.units.length);
    await parallel(dataset.units, concurrency, async (unit, index) => {
        const progress = await readProgress(gatewayPool, unit);
        if (excluded.has(unit.sequence)) {
            if (progress.present) throw new Error(`excluded dataset sequence ${unit.sequence} is registered`);
            entries[index] = {sequence: unit.sequence, unit: null, history: []};
            return;
        }
        if (!progress.present) throw new Error(`registered dataset sequence ${unit.sequence} is absent`);
        entries[index] = {
            sequence: unit.sequence,
            unit: progress.current,
            history: stableHistory(progress.history),
        };
    });
    return entries.sort((left, right) => left.sequence - right.sequence);
}

function writePreconditions(filePath, preconditions) {
    writeJSONAtomic(filePath, preconditions);
}

module.exports = {
    PreparationJournal,
    STABLE_UNIT_FIELDS,
    activeTransferProbe,
    assertHistoryPrefix,
    buildPreconditions,
    committedUnit,
    equalUnit,
    isTransientGrpcError,
    invokeRecipeStep,
    journalStatistics,
    parallel,
    prepareRecipe,
    progressReaderMspId,
    readProgress,
    stableHistory,
    stableUnit,
    stepRecords,
    unitReference,
    verifySnapshotState,
    waitForCommittedStep,
    writePreconditions,
};
