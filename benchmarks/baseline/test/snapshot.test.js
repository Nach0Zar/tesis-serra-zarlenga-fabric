'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const {writeJSONAtomic} = require('../../caliper/src/metadata');
const {sha256File} = require('../../caliper/src/sources');
const {prepareSnapshot, readSnapshot, snapshotPreconditions, verifyUnitPrecondition} = require('../src/snapshot');

const unit = {
    sequence: 1, initialState: 'EN_LABORATORIO', initialCustodian: 'GLN:lab',
    preparation: [{operation: 'RegisterUnit', invokerMspId: 'LabMSP',
        request: {gtin: '1', numeroSerie: 'S1', lote: 'L1', fechaVencimiento: '2099-01-01'}}],
};
const recipe = [
    ...unit.preparation,
    {operation: 'DispatchTransfer', privateData: {destinatario: {destino: 'GLN:pharmacy'}}},
    {operation: 'ReceiveTransfer'},
];
const entry = {datasetSequence: 1, role: 'prepared', recipe,
    expected: {present: true, state: 'EN_CUSTODIA', custodian: 'GLN:pharmacy'}};

test('preparation uses canonical recipes and never retries opaque internal errors', async () => {
    const calls = [];
    await assert.rejects(() => prepareSnapshot({invoke: async (invocation) => {
        calls.push(invocation.operation);
        return invocation.operation === 'DispatchTransfer'
            ? {ok: true, transaction: {}}
            : {ok: false, transaction: {errorCode: 'INTERNAL_ERROR'}};
    }}, {units: [entry]}, 1), /ReceiveTransfer failed with INTERNAL_ERROR/u);
    assert.deepEqual(calls, ['DispatchTransfer', 'ReceiveTransfer']);
});

test('successful preparation skips atomic seed registrations and preserves sequential transfers', async () => {
    const calls = [];
    const result = await prepareSnapshot({invoke: async (invocation) => {
        calls.push(invocation.operation);
        return {ok: true, transaction: {}};
    }}, {units: [entry]}, 1);
    assert.deepEqual(calls, ['DispatchTransfer', 'ReceiveTransfer']);
    assert.deepEqual(result, {recipes: 1, receiveRetries: 0});
});

test('logical preconditions preserve the complete state and custody history from the common recipe', () => {
    const [expected] = snapshotPreconditions({units: [unit]}, {units: [entry]});
    assert.equal(expected.unit.estado, 'EN_CUSTODIA');
    assert.equal(expected.unit.custodioActual, 'GLN:pharmacy');
    assert.deepEqual(expected.history.map((event) => [event.value.estado, event.value.custodioActual]), [
        ['EN_LABORATORIO', 'GLN:lab'], ['EN_TRANSITO', 'GLN:lab'], ['EN_CUSTODIA', 'GLN:pharmacy'],
    ]);
});

test('history verification ignores SUT identifiers but rejects altered history with unchanged current state', async () => {
    const [expected] = snapshotPreconditions({units: [unit]}, {units: [entry]});
    let history = expected.history.map((event, index) => ({...event,
        txId: `baseline-${index}`, timestamp: '2026-10-13T00:00:00Z'}));
    const client = {invoke: async (invocation) => ({
        ok: true, transaction: {}, value: invocation.operation === 'ReadUnit' ? expected.unit : history,
    })};
    await verifyUnitPrecondition(client, unit, expected);
    history = history.slice(1);
    await assert.rejects(() => verifyUnitPrecondition(client, unit, expected), /contaminated history/u);
});

test('absent registrations must have neither current state nor historical events', async () => {
    const expected = {sequence: 1, unit: null, history: []};
    const absent = {invoke: async () => ({ok: false, transaction: {errorCode: 'UNIT_NOT_FOUND'}})};
    await verifyUnitPrecondition(absent, unit, expected);
    await assert.rejects(() => verifyUnitPrecondition({invoke: async (invocation) =>
        invocation.operation === 'ReadUnit' ? absent.invoke() : {ok: true, value: []}
    }, unit, expected), /should have no history/u);
});

test('snapshot manifests require an intact common logical plan and matching hash', (context) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-baseline-snapshot-'));
    context.after(() => fs.rmSync(directory, {recursive: true}));
    const manifestPath = path.join(directory, 'manifest.json');
    writeJSONAtomic(manifestPath, {schemaVersion: '1.0.0', files: {}});
    assert.throws(() => readSnapshot(directory), /canonical logical plan hash/u);
    const planPath = path.join(directory, 'logical-plan.json');
    writeJSONAtomic(planPath, {units: [entry]});
    const hash = sha256File(planPath);
    const manifest = {schemaVersion: '1.0.0', logicalPlanSHA256: hash,
        files: {logicalPlan: {name: 'logical-plan.json', sha256: hash}}};
    writeJSONAtomic(manifestPath, manifest);
    assert.equal(readSnapshot(directory).manifest.logicalPlanSHA256, hash);
    writeJSONAtomic(manifestPath, {...manifest, logicalPlanSHA256: 'different'});
    assert.throws(() => readSnapshot(directory), /canonical logical plan hash/u);
    writeJSONAtomic(manifestPath, manifest);
    writeJSONAtomic(planPath, {units: []});
    assert.throws(() => readSnapshot(directory), /snapshot file hash mismatch/u);
});
