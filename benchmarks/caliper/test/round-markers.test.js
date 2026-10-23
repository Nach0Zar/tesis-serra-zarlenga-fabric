'use strict';
const assert = require('node:assert/strict');
const test = require('node:test');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const protos = require('@hyperledger/fabric-protos');
const {decodeBlock, summarizeObserved, observeRound, writeAudit} = require('../src/round-markers');
const {buildRoundMetadata} = require('../src/metadata');
function collectionRWSet(name, writes) {
    const hashed = new protos.ledger.rwset.kvrwset.HashedRWSet();
    for (let index = 0; index < writes; index += 1) {
        const write = new protos.ledger.rwset.kvrwset.KVWriteHash();
        write.setKeyHash(Uint8Array.from([index + 1]));
        write.setValueHash(Uint8Array.from([index + 2]));
        hashed.addHashedWrites(write);
    }
    const collection = new protos.ledger.rwset.CollectionHashedReadWriteSet();
    collection.setCollectionName(name);
    collection.setHashedRwset(hashed.serializeBinary());
    return collection;
}

function privateWriteEnvelope() {
    const namespace = new protos.ledger.rwset.NsReadWriteSet();
    namespace.setNamespace('snt');
    namespace.addCollectionHashedRwset(collectionRWSet('_implicit_org_LabMSP', 1));
    namespace.addCollectionHashedRwset(collectionRWSet('transfer_DrogueriaMSP_LabMSP', 2));
    const transactionRWSet = new protos.ledger.rwset.TxReadWriteSet();
    transactionRWSet.addNsRwset(namespace);
    const chaincodeAction = new protos.peer.ChaincodeAction();
    chaincodeAction.setResults(transactionRWSet.serializeBinary());
    const response = new protos.peer.ProposalResponsePayload();
    response.setExtension$(chaincodeAction.serializeBinary());
    const endorsed = new protos.peer.ChaincodeEndorsedAction();
    endorsed.setProposalResponsePayload(response.serializeBinary());
    const actionPayload = new protos.peer.ChaincodeActionPayload();
    actionPayload.setAction(endorsed);
    const transactionAction = new protos.peer.TransactionAction();
    transactionAction.setPayload(actionPayload.serializeBinary());
    const transaction = new protos.peer.Transaction();
    transaction.addActions(transactionAction);
    const payload = new protos.common.Payload();
    payload.setData(transaction.serializeBinary());
    const envelope = new protos.common.Envelope();
    envelope.setPayload(payload.serializeBinary());
    const prepared = new protos.gateway.PreparedTransaction();
    prepared.setTransactionId('tx-fixture');
    prepared.setEnvelope(envelope);
    return Buffer.from(prepared.serializeBinary());
}

function round(name = 'RegisterUnit', status = 'success', rejection = false) {
    const type = name === 'RegisterUnit' ? 'register' : name === 'ReadUnit' ? 'query-unit' : 'transfer';
    const operation = {workerIndex: 0, ordinal: 0, datasetSequence: 1, type,
        transactions: [{function: name, status, transactionId: 'tx-one'}]};
    const planned = {...operation, invocations: [{operation: name}], expectedRejection: rejection || undefined};
    return {operations: [operation], plan: {workers: [{workerIndex: 0, operations: [planned]}]}};
}
function record(name = 'RegisterUnit', markerWrites = 1, validationCode = 0) {
    return {transactionId: 'tx-one', function: name, namespace: 'snt', markerWrites,
        validationCode, blockNumber: '7'};
}

test('actual block decoder separates implicit markers from bilateral writes', () => {
    const prepared = protos.gateway.PreparedTransaction.deserializeBinary(privateWriteEnvelope());
    const envelope = prepared.getEnvelope();
    const payload = protos.common.Payload.deserializeBinary(envelope.getPayload_asU8());
    const channelHeader = new protos.common.ChannelHeader();
    channelHeader.setType(3);
    channelHeader.setTxId('tx-one');
    const header = new protos.common.Header();
    header.setChannelHeader(channelHeader.serializeBinary());
    payload.setHeader(header);
    const tx = protos.peer.Transaction.deserializeBinary(payload.getData_asU8());
    const action = tx.getActionsList()[0];
    const actionPayload = protos.peer.ChaincodeActionPayload.deserializeBinary(action.getPayload_asU8());
    const id = new protos.peer.ChaincodeID();
    id.setName('snt');
    const input = new protos.peer.ChaincodeInput();
    input.setArgsList([Buffer.from('RegisterUnit')]);
    const spec = new protos.peer.ChaincodeSpec();
    spec.setChaincodeId(id);
    spec.setInput(input);
    const invocation = new protos.peer.ChaincodeInvocationSpec();
    invocation.setChaincodeSpec(spec);
    const proposal = new protos.peer.ChaincodeProposalPayload();
    proposal.setInput(invocation.serializeBinary());
    actionPayload.setChaincodeProposalPayload(proposal.serializeBinary());
    action.setPayload(actionPayload.serializeBinary());
    payload.setData(tx.serializeBinary());
    envelope.setPayload(payload.serializeBinary());
    const block = new protos.common.Block();
    const blockHeader = new protos.common.BlockHeader();
    blockHeader.setNumber(7);
    block.setHeader(blockHeader);
    const data = new protos.common.BlockData();
    data.setDataList([envelope.serializeBinary()]);
    block.setData(data);
    const metadata = new protos.common.BlockMetadata();
    metadata.setMetadataList([new Uint8Array(), new Uint8Array(), Uint8Array.from([0])]);
    block.setMetadata(metadata);
    assert.deepEqual(decodeBlock(block), [record()]);
    metadata.setMetadataList([new Uint8Array(), new Uint8Array(), Uint8Array.from([11])]);
    assert.equal(decodeBlock(block)[0].validationCode, 11);
});

test('markers are attributed to committed writes and zero rounds remain explicit', () => {
    const registration = round();
    assert.deepEqual(summarizeObserved(registration.plan, registration.operations, [record()], 120),
        {expected: 1, observed: 1, fromRegistrations: 1, fromRegulatoryEvents: 0,
            successfulWriteTransactions: 1, perSecond: 1 / 120, shareOfSuccessfulWrites: 1});
    for (const [name, status, rejection, records] of [
        ['ReadUnit', 'success', false, []],
        ['RegisterUnit', 'failed', true, [record('RegisterUnit', 1, 11)]],
        ['DispatchTransfer', 'success', false, [record('DispatchTransfer', 0)]],
    ]) {
        const item = round(name, status, rejection);
        const result = summarizeObserved(item.plan, item.operations, records, 120);
        assert.equal(result.observed, 0);
        assert.equal(result.expected, 0);
        assert.equal(result.shareOfSuccessfulWrites, 0);
    }
});

test('omissions, duplicates, invalid success and foreign writes invalidate evidence', () => {
    const {plan, operations} = round();
    for (const records of [[], [record('RegisterUnit', 0)], [record('RegisterUnit', 2)],
        [record(), record()], [record('RegisterUnit', 1, 11)],
        [{...record(), transactionId: 'foreign'}]]) {
        assert.throws(() => summarizeObserved(plan, operations, records, 120));
    }
    assert.throws(() => summarizeObserved(plan,
        [{...operations[0], transactions: [operations[0].transactions[0], operations[0].transactions[0]]}],
        [record()], 120), /duplicate/u);
    assert.throws(() => summarizeObserved(plan, [{...operations[0], datasetSequence: 2}], [record()], 120),
        /work plan/u);
});

test('post-round observation waits for lagging peer and retains partial timeout evidence', async () => {
    const {plan, operations} = round();
    let time = 0;
    let reads = 0;
    const observer = {height: async () => reads++ ? 8n : 7n,
        readRange: async (start, end, deadline, append) => { if (end > start) append([record()]); }};
    const options = {now: () => time, sleep: async () => { time += 100; }};
    const before = JSON.stringify(operations);
    const result = await observeRound(observer, 7n, plan, operations, 120, options);
    assert.equal(result.markers.observed, 1);
    assert.equal(JSON.stringify(operations), before);
    time = 0;
    await assert.rejects(observeRound({...observer, height: async () => 7n}, 7n, plan, operations, 120,
        {...options, timeoutMs: 200}), (error) => /timed out/u.test(error.message) && error.observations.length === 0);
});

test('audit contains only selected public fields and metadata keeps original times', () => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-markers-'));
    try {
        writeAudit(path.join(directory, 'markers.csv'), [record()]);
        assert.match(fs.readFileSync(path.join(directory, 'markers.csv'), 'utf8'), /tx-one/u);
        const item = round();
        const markers = summarizeObserved(item.plan, item.operations, [record()], 120);
        const context = {profile: {durationSeconds: 120, rate: 5, transactionsPerOperation: 1},
            startedAt: '2026-10-20T10:00:00Z', endedAt: '2026-10-20T10:02:00Z'};
        const before = buildRoundMetadata(context, {transactionCount: 1});
        const after = buildRoundMetadata({...context, participationMarkers: markers}, {transactionCount: 1});
        assert.deepEqual(after, {...before, participationMarkers: markers});
    } finally { fs.rmSync(directory, {recursive: true, force: true}); }
});
