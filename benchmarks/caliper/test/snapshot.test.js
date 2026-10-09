'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const {common, ledger, peer} = require('@hyperledger/fabric-protos');

const {addBlockToTally, emptyMarkerTally, summarizeBlock} = require('../src/block-markers');
const {compareCounts, expectedTransactionsByFunction} = require('../src/build-snapshot');
const {compareArtifacts} = require('../src/fabric-snapshot');
const {sequencesForPlan} = require('../src/run-round');
const {Series} = require('../src/run-series');
const {runRecipe, verifyPreconditions} = require('../src/snapshot-builder');
const {
    EXPECTED_COUNTS, buildGoldenSnapshotPlan, expectedPreconditions, expectedPreparationMarkers, expectedUnitAfter,
    snapshotRecipes,
} = require('../src/snapshot-plan');

const LAB = 'GLN:7791234500017';
const DROGUERIA = 'GLN:7791234500024';
const FARMACIA = 'GLN:7791234500048';

function step(operation, invokerMspId, extra = {}) {
    return {operation, invokerMspId, request: {gtin: '07790202607277', numeroSerie: 'SN1', lote: 'L1', fechaVencimiento: '2100-01-01'}, ...extra};
}

function dispatch(invokerMspId, destino) {
    return step('DispatchTransfer', invokerMspId, {privateData: {destinatario: {destino}}});
}

const UNIT = {sequence: 7, initialCustodian: LAB, preparation: [step('RegisterUnit', 'LabMSP')]};

test('expected unit state follows the state machine: custody only moves on reception', () => {
    assert.equal(expectedUnitAfter(UNIT, []), null);
    assert.deepEqual(expectedUnitAfter(UNIT, [step('RegisterUnit', 'LabMSP')]), {
        gtin: '07790202607277', numeroSerie: 'SN1', lote: 'L1', fechaVencimiento: '2100-01-01',
        custodioActual: LAB, estado: 'EN_LABORATORIO',
    });
    const chain = [
        step('RegisterUnit', 'LabMSP'), dispatch('LabMSP', DROGUERIA), step('ReceiveTransfer', 'DrogueriaMSP'),
        dispatch('DrogueriaMSP', FARMACIA),
    ];
    assert.deepEqual(
        [expectedUnitAfter(UNIT, chain).estado, expectedUnitAfter(UNIT, chain).custodioActual],
        ['EN_TRANSITO', DROGUERIA],
    );
    const received = [...chain, step('ReceiveTransfer', 'FarmaciaMSP'), step('Quarantine', 'FarmaciaMSP')];
    assert.deepEqual(
        [expectedUnitAfter(UNIT, received).estado, expectedUnitAfter(UNIT, received).custodioActual],
        ['EN_CUARENTENA', FARMACIA],
    );
    assert.throws(() => expectedUnitAfter(UNIT, [step('RegisterUnit', 'LabMSP'), step('ReceiveTransfer', 'X')]),
        /receive without dispatch/u);
});

test('expected markers count every registration and only regulator-initiated events', () => {
    const recipes = [
        {datasetSequence: 1, steps: [step('RegisterUnit', 'LabMSP')]},
        {datasetSequence: 2, steps: [step('RegisterUnit', 'LabMSP'), step('WithdrawFromMarket', 'AnmatMSP')]},
        {datasetSequence: 3, steps: [step('RegisterUnit', 'LabMSP'), step('Quarantine', 'FarmaciaMSP')]},
    ];
    assert.deepEqual(expectedPreparationMarkers(recipes), {expected: 4, fromRegistrations: 3, fromRegulatoryEvents: 1});
    assert.deepEqual(expectedTransactionsByFunction(recipes), {RegisterUnit: 3, WithdrawFromMarket: 1, Quarantine: 1});
    assert.deepEqual(compareCounts({RegisterUnit: 3}, {RegisterUnit: 3, Dispense: 1}),
        ['Dispense: esperadas 0, confirmadas 1']);
});

function endorserEnvelope(txId, functionName, implicitWrites) {
    const hashedRWSet = new ledger.rwset.kvrwset.HashedRWSet();
    hashedRWSet.setHashedWritesList(Array.from({length: implicitWrites}, () => {
        const write = new ledger.rwset.kvrwset.KVWriteHash();
        write.setKeyHash(Buffer.from('k'));
        return write;
    }));
    const namespace = new ledger.rwset.NsReadWriteSet();
    namespace.setNamespace('snt');
    if (implicitWrites > 0) {
        const collection = new ledger.rwset.CollectionHashedReadWriteSet();
        collection.setCollectionName(functionName === 'RegisterUnit' ? '_implicit_org_LabMSP' : '_implicit_org_AnmatMSP');
        collection.setHashedRwset(hashedRWSet.serializeBinary());
        namespace.setCollectionHashedRwsetList([collection]);
    }
    const readWriteSet = new ledger.rwset.TxReadWriteSet();
    readWriteSet.setNsRwsetList([namespace]);
    const chaincodeAction = new peer.ChaincodeAction();
    chaincodeAction.setResults(readWriteSet.serializeBinary());
    const responsePayload = new peer.ProposalResponsePayload();
    responsePayload.setExtension$(chaincodeAction.serializeBinary());
    const endorsedAction = new peer.ChaincodeEndorsedAction();
    endorsedAction.setProposalResponsePayload(responsePayload.serializeBinary());

    const input = new peer.ChaincodeInput();
    input.setArgsList([Buffer.from(functionName)]);
    const spec = new peer.ChaincodeSpec();
    spec.setInput(input);
    const invocation = new peer.ChaincodeInvocationSpec();
    invocation.setChaincodeSpec(spec);
    const proposalPayload = new peer.ChaincodeProposalPayload();
    proposalPayload.setInput(invocation.serializeBinary());

    const actionPayload = new peer.ChaincodeActionPayload();
    actionPayload.setChaincodeProposalPayload(proposalPayload.serializeBinary());
    actionPayload.setAction(endorsedAction);
    const action = new peer.TransactionAction();
    action.setPayload(actionPayload.serializeBinary());
    const transaction = new peer.Transaction();
    transaction.setActionsList([action]);
    return envelope(common.HeaderType.ENDORSER_TRANSACTION, txId, transaction.serializeBinary());
}

function envelope(type, txId, data) {
    const channelHeader = new common.ChannelHeader();
    channelHeader.setType(type);
    channelHeader.setTxId(txId);
    const header = new common.Header();
    header.setChannelHeader(channelHeader.serializeBinary());
    const payload = new common.Payload();
    payload.setHeader(header);
    payload.setData(data);
    const value = new common.Envelope();
    value.setPayload(payload.serializeBinary());
    return value.serializeBinary();
}

function block(number, envelopes, validationCodes) {
    const value = new common.Block();
    const header = new common.BlockHeader();
    header.setNumber(number);
    value.setHeader(header);
    const data = new common.BlockData();
    data.setDataList(envelopes);
    value.setData(data);
    const metadata = new common.BlockMetadata();
    const entries = [new Uint8Array(), new Uint8Array(), Uint8Array.from(validationCodes), new Uint8Array(), new Uint8Array()];
    metadata.setMetadataList(entries);
    value.setMetadata(metadata);
    return common.Block.deserializeBinary(value.serializeBinary());
}

test('block tally counts implicit-collection markers of valid transactions by origin', () => {
    const {VALID, MVCC_READ_CONFLICT} = peer.TxValidationCode;
    const summary = summarizeBlock(block(9, [
        endorserEnvelope('t1', 'RegisterUnit', 1),
        endorserEnvelope('t2', 'WithdrawFromMarket', 1),
        endorserEnvelope('t3', 'DispatchTransfer', 0),
        endorserEnvelope('t4', 'RegisterUnit', 1),
        envelope(common.HeaderType.CONFIG, 'cfg', Buffer.alloc(0)),
    ], [VALID, VALID, VALID, MVCC_READ_CONFLICT, VALID]));
    assert.equal(summary.number, 9);
    assert.equal(summary.transactions.length, 4);
    const tally = addBlockToTally(emptyMarkerTally(), summary);
    assert.deepEqual(tally.byFunction, {RegisterUnit: 1, WithdrawFromMarket: 1, DispatchTransfer: 1});
    assert.equal(tally.invalidTransactions, 1);
    assert.deepEqual(tally.markers, {
        total: 2, fromRegistrations: 1, fromRegulatoryEvents: 1,
        byCollection: {_implicit_org_LabMSP: 1, _implicit_org_AnmatMSP: 1},
    });
});

function fakeLedgerPool({failOnce = {}, failAlways} = {}) {
    const history = [];
    const sent = [];
    return {
        sent,
        history,
        async invoke(invocation) {
            sent.push(invocation.operation);
            if (failAlways === invocation.operation) throw new Error('{"code":"INVALID_STATE_TRANSITION","message":"no"}');
            history.push(invocation.operation);
            if (failOnce[invocation.operation]) {
                failOnce[invocation.operation] = false;
                // El paso quedo confirmado aunque el cliente vio un timeout.
                throw Object.assign(new Error('deadline exceeded'), {code: 4});
            }
        },
        connection() {
            return {contract: {evaluateTransaction: async (name) => {
                assert.equal(name, 'GetUnitHistory');
                if (history.length === 0) throw new Error('{"code":"UNIT_NOT_FOUND","message":"no existe"}');
                return Buffer.from(JSON.stringify(history.map((operation) => ({operation}))));
            }}};
        },
    };
}

test('a recipe resumes from the confirmed history and never resends a committed step', async () => {
    const recipe = {
        datasetSequence: 7,
        steps: [step('RegisterUnit', 'LabMSP'), dispatch('LabMSP', DROGUERIA), step('ReceiveTransfer', 'DrogueriaMSP')],
    };
    const pool = fakeLedgerPool({failOnce: {DispatchTransfer: true}});
    const recoveries = await runRecipe(pool, recipe, {gtin: 'g', numeroSerie: 's'}, {sleep: async () => {}});
    assert.deepEqual(recoveries, ['GRPC_4']);
    assert.deepEqual(pool.sent, ['RegisterUnit', 'DispatchTransfer', 'ReceiveTransfer']);
    assert.deepEqual(pool.history, ['RegisterUnit', 'DispatchTransfer', 'ReceiveTransfer']);

    const stuck = fakeLedgerPool({failAlways: 'DispatchTransfer'});
    await assert.rejects(runRecipe(stuck, recipe, {gtin: 'g', numeroSerie: 's'}, {sleep: async () => {}}),
        /recipe 7 failed 13 times/u);
});

test('precondition verification reports contaminated and unexpectedly present units', async () => {
    const dataset = {units: [1, 2, 3].map((sequence) => ({
        sequence, preparation: [{operation: 'RegisterUnit', request: {gtin: `g${sequence}`, numeroSerie: 's'}}],
    }))};
    const expectedUnit = {gtin: 'g1', numeroSerie: 's', lote: 'L', fechaVencimiento: 'F', custodioActual: LAB, estado: 'EN_LABORATORIO'};
    const ledgerState = {
        g1: expectedUnit,
        g2: {...expectedUnit, gtin: 'g2', estado: 'EN_TRANSITO'},
        g3: {...expectedUnit, gtin: 'g3'},
    };
    const pool = {connection: () => ({contract: {evaluateTransaction: async (_name, gtin) => Buffer.from(JSON.stringify(ledgerState[gtin]))}})};
    const result = await verifyPreconditions({
        pool, dataset, concurrency: 2,
        preconditions: [
            {sequence: 1, unit: expectedUnit},
            {sequence: 2, unit: {...expectedUnit, gtin: 'g2'}},
            {sequence: 3, unit: null},
        ],
    });
    assert.equal(result.present, 1);
    assert.deepEqual(result.mismatches.map((entry) => entry.sequence), [2, 3]);
});

test('a snapshot is rejected when contract, package, Fabric or the network identities change', () => {
    const artifacts = {
        contractVersion: '2.11.2', packageID: 'snt_1.0:abc', fabric: '2.5.16', fabricCA: '1.5.17',
        network: {files: {'network/configtx.yaml': 'h1'}, crypto: 'c1'},
    };
    assert.deepEqual(compareArtifacts(artifacts, structuredClone(artifacts)), []);
    const changed = structuredClone(artifacts);
    changed.packageID = 'snt_1.0:def';
    changed.network.files['network/configtx.yaml'] = 'h2';
    changed.network.crypto = 'c2';
    assert.deepEqual(compareArtifacts(artifacts, changed), [
        'packageID: snt_1.0:abc != snt_1.0:def',
        'network/configtx.yaml changed',
        'network/organizations identities changed',
    ]);
});

test('round verification covers the smoke unit, preparations and every planned operation', () => {
    const plan = {
        preparations: [{datasetSequence: 5}],
        workers: [{operations: [{datasetSequence: 9}, {datasetSequence: 5}]}, {operations: [{datasetSequence: 2}]}],
    };
    assert.deepEqual(sequencesForPlan(plan, 7), [2, 5, 7, 9]);
});

test('a warm-up that never validates blocks the measured repetitions of its scenario', async () => {
    const repoRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-series-'));
    const series = new Series(repoRoot, {repetitions: 5, maxAttempts: 2, specs: [], seriesToken: 'test'});
    const phases = [];
    series.repetition = async (_spec, phase) => {
        phases.push(phase);
        return undefined;
    };
    await series.scenario({scenario: 'write-register', rate: 5});
    assert.deepEqual(phases, ['warmup']);
    assert.equal(series.report.scenarios[0].status, 'not-executed');
    assert.match(series.report.scenarios[0].reason, /warm-up descartado en 2 intentos/u);
});

const datasetDirectory = path.resolve(__dirname, '..', '..', '..', 'build', 'dataset');
test('the canonical bundle yields the shared golden snapshot', {skip: !fs.existsSync(path.join(datasetDirectory, 'manifest.json'))}, () => {
    const {loadDatasetBundle} = require('../src/sources');
    const {dataset} = loadDatasetBundle(datasetDirectory);
    const plan = buildGoldenSnapshotPlan(dataset);
    assert.deepEqual(plan.counts, EXPECTED_COUNTS);
    assert.equal(plan.smokeSequence, 2824);
    const recipes = snapshotRecipes(dataset, plan);
    assert.equal(recipes.length, EXPECTED_COUNTS.total - EXPECTED_COUNTS.unregistered);
    assert.equal(recipes.reduce((sum, recipe) => sum + recipe.steps.length, 0), 129_878);
    assert.deepEqual(expectedPreparationMarkers(recipes), {expected: 46_924, fromRegistrations: 46_920, fromRegulatoryEvents: 4});
    const states = {};
    for (const entry of expectedPreconditions(dataset, plan)) {
        const key = entry.unit?.estado ?? 'AUSENTE';
        states[key] = (states[key] ?? 0) + 1;
    }
    assert.deepEqual(states, {
        AUSENTE: 3080, EN_LABORATORIO: 29_158, EN_CUSTODIA: 17_750, EN_CUARENTENA: 2, VENCIDO: 2,
        DETERIORADO: 2, RETIRADO_MERCADO: 2, PROHIBIDO: 2, DEVUELTO: 2,
    });
});
