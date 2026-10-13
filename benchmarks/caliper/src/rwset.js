'use strict';

const protos = require('@hyperledger/fabric-protos');

function binary(message, getter, label) {
    if (!message || typeof message[getter] !== 'function') throw new Error(`transaction envelope omits ${label}`);
    const value = message[getter]();
    if (!value || value.length === 0) throw new Error(`transaction envelope omits ${label}`);
    return value;
}

function decodeTransactionRWSet(transactionBytes) {
    const prepared = protos.gateway.PreparedTransaction.deserializeBinary(transactionBytes);
    const envelope = prepared.getEnvelope();
    if (!envelope) throw new Error('prepared transaction omits its envelope');
    const payload = protos.common.Payload.deserializeBinary(binary(envelope, 'getPayload_asU8', 'payload'));
    const transaction = protos.peer.Transaction.deserializeBinary(binary(payload, 'getData_asU8', 'data'));
    const collections = [];
    for (const action of transaction.getActionsList()) {
        const actionPayload = protos.peer.ChaincodeActionPayload.deserializeBinary(
            binary(action, 'getPayload_asU8', 'action payload'),
        );
        const endorsedAction = actionPayload.getAction();
        const proposalResponse = protos.peer.ProposalResponsePayload.deserializeBinary(
            binary(endorsedAction, 'getProposalResponsePayload_asU8', 'proposal response payload'),
        );
        const chaincodeAction = protos.peer.ChaincodeAction.deserializeBinary(
            binary(proposalResponse, 'getExtension_asU8', 'chaincode action'),
        );
        const txRWSet = protos.ledger.rwset.TxReadWriteSet.deserializeBinary(
            binary(chaincodeAction, 'getResults_asU8', 'read-write set'),
        );
        for (const namespace of txRWSet.getNsRwsetList()) {
            for (const collection of namespace.getCollectionHashedRwsetList()) {
                const hashedRWSet = protos.ledger.rwset.kvrwset.HashedRWSet.deserializeBinary(
                    binary(collection, 'getHashedRwset_asU8', 'hashed read-write set'),
                );
                collections.push({
                    namespace: namespace.getNamespace(),
                    collection: collection.getCollectionName(),
                    hashedReads: hashedRWSet.getHashedReadsList().length,
                    hashedWrites: hashedRWSet.getHashedWritesList().length,
                    metadataWrites: hashedRWSet.getMetadataWritesList().length,
                });
            }
        }
    }
    return collections.sort((left, right) => left.collection.localeCompare(right.collection));
}

function summarizeCollections(collections) {
    return {
        collections,
        markerWrites: collections.filter((entry) => entry.collection.startsWith('_implicit_org_'))
            .reduce((sum, entry) => sum + entry.hashedWrites, 0),
        pairPrivateWrites: collections.filter((entry) => entry.collection.startsWith('transfer_'))
            .reduce((sum, entry) => sum + entry.hashedWrites, 0),
    };
}

function summarizePrivateWrites(transactionBytes) {
    return summarizeCollections(decodeTransactionRWSet(transactionBytes));
}

module.exports = {decodeTransactionRWSet, summarizeCollections, summarizePrivateWrites};
