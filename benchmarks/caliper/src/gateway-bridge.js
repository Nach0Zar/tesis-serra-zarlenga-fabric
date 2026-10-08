'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const timers = require('node:timers/promises');

const grpc = require('@grpc/grpc-js');
const {Constants, TxStatus} = require('@hyperledger/caliper-core');
const {connect, hash, signers} = require('@hyperledger/fabric-gateway');

const {extractContractError, isPrivateDataNotDisseminated} = require('./contract-errors');
const {contractArguments, transientMap} = require('./requests');

const SUBMITTED_EVENT = Constants.Events.Connector.TxsSubmitted;
const FINISHED_EVENT = Constants.Events.Connector.TxsFinished;

function networkOrganizations(networkConfig) {
    const values = new Map();
    for (const organization of networkConfig.organizations ?? []) {
        const identity = organization.identities?.certificates?.[0];
        const peer = organization.peers?.[0];
        if (!organization.mspid || !identity?.clientPrivateKey?.path || !identity?.clientSignedCert?.path
            || !peer?.endpoint || !peer?.tlsCACerts?.path) {
            throw new Error(`network config is incomplete for ${organization.mspid ?? 'unknown MSP'}`);
        }
        values.set(organization.mspid, {organization, identity, peer});
    }
    return values;
}

class GatewayPool {
    constructor(networkConfig) {
        this.organizations = networkOrganizations(networkConfig);
        this.connections = new Map();
    }

    connection(mspId) {
        if (this.connections.has(mspId)) return this.connections.get(mspId);
        const configuration = this.organizations.get(mspId);
        if (!configuration) throw new Error(`network config has no organization ${mspId}`);
        const {identity, organization, peer} = configuration;
        const client = new grpc.Client(
            peer.endpoint,
            grpc.credentials.createSsl(fs.readFileSync(peer.tlsCACerts.path)),
            {'grpc.ssl_target_name_override': peer.grpcOptions?.['ssl-target-name-override']},
        );
        const privateKey = crypto.createPrivateKey(fs.readFileSync(identity.clientPrivateKey.path));
        const gateway = connect({
            client,
            identity: {mspId: organization.mspid, credentials: fs.readFileSync(identity.clientSignedCert.path)},
            signer: signers.newPrivateKeySigner(privateKey),
            hash: hash.sha256,
        });
        const value = {client, gateway, contract: gateway.getNetwork('snt-channel').getContract('snt')};
        this.connections.set(mspId, value);
        return value;
    }

    async invoke(invocation) {
        const {contract} = this.connection(invocation.invokerMspId);
        const deadline = Date.now() + 30_000;
        const options = {arguments: contractArguments(invocation)};
        const transient = transientMap(invocation);
        if (transient) {
            options.transientData = Object.fromEntries(
                Object.entries(transient).map(([key, value]) => [key, Buffer.from(value)]),
            );
        }
        if (invocation.targetMspIds?.length) options.endorsingOrganizations = invocation.targetMspIds;
        const proposal = contract.newProposal(invocation.operation, options);
        const transactionId = proposal.getTransactionId();
        const transaction = await proposal.endorse({deadline});
        const submitted = await transaction.submit({deadline});
        const status = await submitted.getStatus({deadline});
        if (!status.successful) {
            const error = new Error(`transaction ${transactionId} committed with status ${status.code}`);
            error.transactionId = transactionId;
            throw error;
        }
        return {transactionId, result: submitted.getResult()};
    }

    async waitForUnitState(mspIds, request, expectedState, timeoutMs = 30_000) {
        if (!Array.isArray(mspIds) || mspIds.length === 0) {
            throw new Error('state synchronization requires at least one MSP');
        }
        const deadline = Date.now() + timeoutMs;
        const pending = new Set(mspIds);
        while (pending.size > 0) {
            for (const mspId of [...pending]) {
                const {contract} = this.connection(mspId);
                try {
                    const proposal = contract.newProposal('ReadUnit', {
                        arguments: [request.gtin, request.numeroSerie],
                    });
                    const bytes = await proposal.evaluate({deadline: Math.min(deadline, Date.now() + 5_000)});
                    const unit = JSON.parse(Buffer.from(bytes).toString('utf8'));
                    if (unit.estado === expectedState) pending.delete(mspId);
                } catch (error) {
                    if (Date.now() >= deadline) throw error;
                }
            }
            if (pending.size === 0) return;
            if (Date.now() >= deadline) {
                throw new Error(
                    `peers ${[...pending].join(', ')} did not observe ${expectedState} before the deadline`,
                );
            }
            await timers.setTimeout(100);
        }
    }

    close() {
        for (const {client, gateway} of this.connections.values()) {
            gateway.close();
            client.close();
        }
        this.connections.clear();
    }
}

function errorTransactionId(error) {
    return error?.transactionId ?? error?.transactionID ?? error?.transactionIdString ?? '';
}

function receiveRetryCause(envelope) {
    if (isPrivateDataNotDisseminated(envelope)) return 'PRIVATE_DATA_NOT_DISSEMINATED';
    if (envelope?.code === 'NOT_IN_TRANSIT') return 'NOT_IN_TRANSIT';
    return undefined;
}

function isRetryableReceiveVisibility(envelope) {
    return receiveRetryCause(envelope) !== undefined;
}

async function measuredGatewayInvocation({
    gatewayPool,
    sutAdapter,
    invocation,
    retryTransientReceive = false,
    retryDelayMs = 500,
    maxAttempts = 60,
}) {
    if (!Number.isInteger(maxAttempts) || maxAttempts <= 0) {
        throw new Error('maxAttempts must be a positive integer');
    }
    const attempts = [];
    for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
        const status = new TxStatus();
        status.SetTimeCreate(Date.now());
        sutAdapter.emit(SUBMITTED_EVENT, 1);
        let envelope;
        try {
            const result = await gatewayPool.invoke(invocation);
            status.SetID(result.transactionId);
            status.SetResult(result.result);
            status.SetVerification(true);
            status.SetStatusSuccess();
        } catch (error) {
            envelope = extractContractError(error);
            status.SetID(errorTransactionId(error));
            status.SetResult('');
            status.SetVerification(true);
            status.SetStatusFail();
            status.SetErrMsg(0, JSON.stringify(envelope ?? {code: 'UNCLASSIFIED_GATEWAY_ERROR'}));
        } finally {
            sutAdapter.emit(FINISHED_EVENT, status);
        }
        const retryCause = retryTransientReceive
            && invocation.operation === 'ReceiveTransfer'
            ? receiveRetryCause(envelope) : undefined;
        const attemptRecord = {status, envelope, retryCause, retryWaitMs: 0};
        attempts.push(attemptRecord);
        if (status.GetStatus() === 'success' || retryCause === undefined) {
            return {attempts, status, envelope, retryCount: attempts.length - 1, exhausted: false};
        }
        if (attempt === maxAttempts) {
            return {attempts, status, envelope, retryCount: attempts.length - 1, exhausted: true};
        }
        const waitStartedAt = Date.now();
        await timers.setTimeout(retryDelayMs);
        attemptRecord.retryWaitMs = Math.max(0, Date.now() - waitStartedAt);
    }
}

async function invokePreparation(gatewayPool, invocation) {
    if (invocation.operation === 'ReceiveTransfer') {
        await gatewayPool.waitForUnitState(invocation.targetMspIds, invocation.request, 'EN_TRANSITO');
    }
    for (let attempt = 1; attempt <= 60; attempt += 1) {
        try {
            return await gatewayPool.invoke(invocation);
        } catch (error) {
            const envelope = extractContractError(error);
            if (!isPrivateDataNotDisseminated(envelope) || invocation.operation !== 'ReceiveTransfer') throw error;
            await timers.setTimeout(500);
        }
    }
    throw new Error('preparation ReceiveTransfer exceeded 60 private-data dissemination attempts');
}

module.exports = {
    GatewayPool,
    invokePreparation,
    isRetryableReceiveVisibility,
    measuredGatewayInvocation,
    networkOrganizations,
    receiveRetryCause,
};
