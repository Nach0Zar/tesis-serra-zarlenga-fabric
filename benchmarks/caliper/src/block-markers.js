'use strict';

const {common, ledger, peer} = require('@hyperledger/fabric-protos');

const CHAINCODE_NAMESPACE = 'snt';
const IMPLICIT_COLLECTION_PREFIX = '_implicit_org_';
const ENDORSER_TRANSACTION = common.HeaderType.ENDORSER_TRANSACTION;
const VALID = peer.TxValidationCode.VALID;

// Funcion invocada por la propuesta: primer argumento del ChaincodeInvocationSpec.
function invokedFunction(actionPayload) {
    const proposalPayload = peer.ChaincodeProposalPayload.deserializeBinary(actionPayload.getChaincodeProposalPayload_asU8());
    const invocation = peer.ChaincodeInvocationSpec.deserializeBinary(proposalPayload.getInput_asU8());
    const args = invocation.getChaincodeSpec()?.getInput()?.getArgsList_asU8() ?? [];
    return args.length > 0 ? Buffer.from(args[0]).toString('utf8') : '';
}

// Escrituras con hash en colecciones implicitas: son los marcadores de participacion
// de ADR-007 punto 6, la unica escritura del chaincode en esas colecciones.
function implicitCollectionWrites(actionPayload) {
    const responsePayload = peer.ProposalResponsePayload.deserializeBinary(
        actionPayload.getAction().getProposalResponsePayload_asU8(),
    );
    const chaincodeAction = peer.ChaincodeAction.deserializeBinary(responsePayload.getExtension_asU8());
    const readWriteSet = ledger.rwset.TxReadWriteSet.deserializeBinary(chaincodeAction.getResults_asU8());
    const byCollection = {};
    for (const namespace of readWriteSet.getNsRwsetList()) {
        if (namespace.getNamespace() !== CHAINCODE_NAMESPACE) continue;
        for (const collection of namespace.getCollectionHashedRwsetList()) {
            const name = collection.getCollectionName();
            if (!name.startsWith(IMPLICIT_COLLECTION_PREFIX)) continue;
            const hashed = ledger.rwset.kvrwset.HashedRWSet.deserializeBinary(collection.getHashedRwset_asU8());
            const writes = hashed.getHashedWritesList().filter((write) => !write.getIsDelete()).length;
            if (writes > 0) byCollection[name] = (byCollection[name] ?? 0) + writes;
        }
    }
    return byCollection;
}

// Resume un bloque confirmado: transacciones de endoso validas sobre el chaincode
// y marcadores escritos por cada una, con la funcion que los produjo.
function summarizeBlock(block) {
    const number = Number(block.getHeader().getNumber());
    const filter = block.getMetadata().getMetadataList_asU8()[common.BlockMetadataIndex.TRANSACTIONS_FILTER] ?? new Uint8Array();
    const transactions = [];
    block.getData().getDataList_asU8().forEach((envelopeBytes, index) => {
        const envelope = common.Envelope.deserializeBinary(envelopeBytes);
        const payload = common.Payload.deserializeBinary(envelope.getPayload_asU8());
        const channelHeader = common.ChannelHeader.deserializeBinary(payload.getHeader().getChannelHeader_asU8());
        if (channelHeader.getType() !== ENDORSER_TRANSACTION) return;
        const validationCode = filter[index];
        const transaction = peer.Transaction.deserializeBinary(payload.getData_asU8());
        for (const action of transaction.getActionsList()) {
            const actionPayload = peer.ChaincodeActionPayload.deserializeBinary(action.getPayload_asU8());
            const markers = implicitCollectionWrites(actionPayload);
            transactions.push({
                transactionId: channelHeader.getTxId(),
                valid: validationCode === VALID,
                validationCode,
                function: invokedFunction(actionPayload),
                markers: Object.values(markers).reduce((sum, count) => sum + count, 0),
                markersByCollection: markers,
            });
        }
    });
    return {number, transactions};
}

function emptyMarkerTally() {
    return {
        blocks: 0,
        validTransactions: 0,
        invalidTransactions: 0,
        byFunction: {},
        markers: {total: 0, fromRegistrations: 0, fromRegulatoryEvents: 0, byCollection: {}},
    };
}

// Las altas aportan el marcador del laboratorio (ADR-007 6.g); cualquier otra
// escritura con marcador es un evento iniciado por la organizacion regulatoria.
function addBlockToTally(tally, summary) {
    tally.blocks += 1;
    for (const transaction of summary.transactions) {
        if (!transaction.valid) {
            tally.invalidTransactions += 1;
            continue;
        }
        tally.validTransactions += 1;
        tally.byFunction[transaction.function] = (tally.byFunction[transaction.function] ?? 0) + 1;
        if (transaction.markers === 0) continue;
        tally.markers.total += transaction.markers;
        if (transaction.function === 'RegisterUnit') tally.markers.fromRegistrations += transaction.markers;
        else tally.markers.fromRegulatoryEvents += transaction.markers;
        for (const [collection, count] of Object.entries(transaction.markersByCollection)) {
            tally.markers.byCollection[collection] = (tally.markers.byCollection[collection] ?? 0) + count;
        }
    }
    return tally;
}

async function chainHeight(network) {
    const bytes = await network.getContract('qscc').evaluateTransaction('GetChainInfo', network.getName());
    return Number(common.BlockchainInfo.deserializeBinary(bytes).getHeight());
}

// Recorre los bloques [startBlock, endBlock) desde el peer del gateway.
async function tallyBlocks(network, startBlock, endBlock) {
    const tally = emptyMarkerTally();
    if (endBlock <= startBlock) return tally;
    const events = await network.getBlockEvents({startBlock: BigInt(startBlock)});
    try {
        for await (const block of events) {
            const summary = summarizeBlock(block);
            if (summary.number >= endBlock) break;
            addBlockToTally(tally, summary);
            if (summary.number === endBlock - 1) break;
        }
    } finally {
        events.close();
    }
    return tally;
}

module.exports = {
    IMPLICIT_COLLECTION_PREFIX,
    addBlockToTally,
    chainHeight,
    emptyMarkerTally,
    summarizeBlock,
    tallyBlocks,
};
