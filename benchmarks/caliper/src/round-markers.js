'use strict';

const fs = require('node:fs');
const timers = require('node:timers/promises');
const protos = require('@hyperledger/fabric-protos');
const {GatewayPool} = require('./gateway-bridge');
const {decodeEnvelopeRWSet} = require('./rwset');

const WRITES = new Set(['RegisterUnit', 'DispatchTransfer', 'ReceiveTransfer', 'Dispense']);
const READS = new Set(['ReadUnit', 'GetUnitHistory']);

function decodeBlock(block) {
    const number = String(block.getHeader().getNumber());
    const codes = block.getMetadata().getMetadataList_asU8()[2];
    const envelopes = block.getData().getDataList_asU8();
    if (!codes || codes.length !== envelopes.length) throw new Error('block validation metadata is incomplete');
    return envelopes.flatMap((bytes, index) => {
        const envelope = protos.common.Envelope.deserializeBinary(bytes);
        const payload = protos.common.Payload.deserializeBinary(envelope.getPayload_asU8());
        const header = protos.common.ChannelHeader.deserializeBinary(payload.getHeader().getChannelHeader_asU8());
        if (header.getType() !== 3) return [];
        const tx = protos.peer.Transaction.deserializeBinary(payload.getData_asU8());
        if (tx.getActionsList().length !== 1) throw new Error('unexpected transaction action count');
        const action = protos.peer.ChaincodeActionPayload.deserializeBinary(tx.getActionsList()[0].getPayload_asU8());
        const proposal = protos.peer.ChaincodeProposalPayload.deserializeBinary(action.getChaincodeProposalPayload_asU8());
        const spec = protos.peer.ChaincodeInvocationSpec.deserializeBinary(proposal.getInput_asU8()).getChaincodeSpec();
        const namespace = spec.getChaincodeId().getName();
        const name = Buffer.from(spec.getInput().getArgsList_asU8()[0]).toString('utf8').split(':').at(-1);
        const markerWrites = decodeEnvelopeRWSet(envelope)
            .filter((entry) => entry.namespace === 'snt' && entry.collection.startsWith('_implicit_org_'))
            .reduce((sum, entry) => sum + entry.hashedWrites, 0);
        return [{transactionId: header.getTxId(), blockNumber: number, function: name,
            namespace, validationCode: codes[index], markerWrites}];
    });
}

function expectations(plan, operations) {
    const transactions = new Map();
    let expected = 0;
    for (const operation of operations) {
        const planned = plan.workers.find((worker) => worker.workerIndex === operation.workerIndex)
            ?.operations.find((entry) => entry.ordinal === operation.ordinal);
        if (!planned || planned.datasetSequence !== operation.datasetSequence || planned.type !== operation.type) {
            throw new Error('raw operation does not match its work plan');
        }
        if (!planned.expectedRejection) {
            expected += planned.invocations.filter((entry) => entry.operation === 'RegisterUnit').length;
        }
        for (const tx of operation.transactions) {
            if (READS.has(tx.function)) continue;
            if (!WRITES.has(tx.function)) throw new Error('unexpected write function in round');
            if (!planned.invocations.some((entry) => entry.operation === tx.function)) {
                throw new Error('raw transaction function does not match its work plan');
            }
            if (!tx.transactionId) {
                if (tx.status === 'success') throw new Error('successful write omits transaction ID');
                continue;
            }
            if (transactions.has(tx.transactionId)) throw new Error('duplicate transaction ID in round');
            transactions.set(tx.transactionId, tx);
        }
    }
    return {transactions, expected};
}

function summarizeObserved(plan, operations, records, durationSeconds) {
    const {transactions, expected} = expectations(plan, operations);
    const seen = new Set();
    let observed = 0;
    let fromRegistrations = 0;
    let successfulWriteTransactions = 0;
    for (const record of records) {
        if (record.namespace !== 'snt') continue;
        if (seen.has(record.transactionId)) throw new Error('duplicate transaction ID in ledger observation');
        seen.add(record.transactionId);
        const tx = transactions.get(record.transactionId);
        if (record.validationCode !== 0) {
            if (tx?.status === 'success') throw new Error('successful raw write is invalid in ledger');
            continue;
        }
        if (!tx || tx.status !== 'success' || tx.function !== record.function) {
            throw new Error('valid ledger write is absent or inconsistent in raw workload');
        }
        const expectedWrites = tx.function === 'RegisterUnit' ? 1 : 0;
        if (record.markerWrites !== expectedWrites) throw new Error('omitted or duplicate participation markers');
        observed += record.markerWrites;
        fromRegistrations += record.markerWrites;
        successfulWriteTransactions += 1;
    }
    const missing = [...transactions.values()].filter((tx) => tx.status === 'success' && !seen.has(tx.transactionId));
    if (missing.length) throw new Error('successful writes missing from ledger observation');
    if (observed !== expected) throw new Error('expected and observed participation markers differ');
    return {expected, observed, fromRegistrations, fromRegulatoryEvents: 0, successfulWriteTransactions,
        perSecond: observed / durationSeconds,
        shareOfSuccessfulWrites: successfulWriteTransactions ? observed / successfulWriteTransactions : 0};
}

function writeAudit(file, records) {
    const columns = ['transactionId', 'blockNumber', 'function', 'validationCode', 'expectedMarkerWrites', 'observedMarkerWrites'];
    const rows = records.filter((record) => record.namespace === 'snt').map((record) => [
        record.transactionId, record.blockNumber, record.function, record.validationCode,
        record.validationCode === 0 && record.function === 'RegisterUnit' ? 1 : 0,
        record.validationCode === 0 ? record.markerWrites : 0,
    ]);
    const csv = (value) => '"' + String(value).replace(/"/gu, '""') + '"';
    fs.writeFileSync(file, [columns.join(','), ...rows.map((row) => row.map(csv).join(','))].join('\n') + '\n', {mode: 0o600});
}

function createObserver(networkConfig) {
    const pool = new GatewayPool(networkConfig);
    const network = pool.connection('LabMSP').gateway.getNetwork('snt-channel');
    return {
        async height(deadline = Date.now() + 30_000) {
            const proposal = network.getContract('qscc').newProposal('GetChainInfo', {arguments: ['snt-channel']});
            const bytes = await proposal.evaluate({deadline});
            return BigInt(protos.common.BlockchainInfo.deserializeBinary(bytes).getHeight());
        },
        async readRange(start, end, deadline, append) {
            if (start >= end) return;
            const events = await network.newBlockEventsRequest({startBlock: start}).getEvents({deadline});
            let next = start;
            try {
                for await (const block of events) {
                    const number = BigInt(block.getHeader().getNumber());
                    if (number !== next) throw new Error('non-contiguous ledger observation');
                    append(decodeBlock(block));
                    next += 1n;
                    if (next >= end) return;
                }
                throw new Error('ledger stream ended before terminal block');
            } finally {
                events.close();
            }
        },
        close() { pool.close(); },
    };
}

async function observeRound(observer, startHeight, plan, operations, durationSeconds, options = {}) {
    const now = options.now ?? Date.now;
    const deadline = now() + (options.timeoutMs ?? 180_000);
    const sleep = options.sleep ?? timers.setTimeout;
    const records = [];
    const expected = expectations(plan, operations);
    let next = startHeight;
    try {
        while (true) {
            if (now() >= deadline) throw new Error('participation marker observation timed out');
            const end = await observer.height(Math.min(deadline, now() + 30_000));
            if (end < next) throw new Error('ledger height regressed');
            await observer.readRange(next, end, deadline, (entries) => records.push(...entries));
            next = end;
            const seen = new Set(records.map((entry) => entry.transactionId));
            if ([...expected.transactions.values()].every((tx) => tx.status !== 'success' || seen.has(tx.transactionId))) break;
            await sleep(100);
        }
        return {markers: summarizeObserved(plan, operations, records, durationSeconds), records};
    } catch (error) {
        error.observations = records;
        throw error;
    }
}

module.exports = {createObserver, decodeBlock, expectations, observeRound, summarizeObserved, writeAudit};
