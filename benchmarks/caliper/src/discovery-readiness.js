'use strict';

const timers = require('node:timers/promises');
const protos = require('@hyperledger/fabric-protos');

const {isTransientGrpcError, preparationErrorDiagnostics} = require('./snapshot-state');

function endorsementOrganizations(bytes) {
    const prepared = protos.gateway.PreparedTransaction.deserializeBinary(bytes);
    const envelope = prepared.getEnvelope();
    if (!envelope) throw new Error('readiness proposal omits its envelope');
    const payload = protos.common.Payload.deserializeBinary(envelope.getPayload_asU8());
    const transaction = protos.peer.Transaction.deserializeBinary(payload.getData_asU8());
    const actions = transaction.getActionsList();
    if (actions.length !== 1) throw new Error('readiness proposal must contain exactly one action');
    const action = protos.peer.ChaincodeActionPayload.deserializeBinary(actions[0].getPayload_asU8()).getAction();
    if (!action) throw new Error('readiness proposal omits its endorsed action');
    return action.getEndorsementsList().map((endorsement) =>
        protos.msp.SerializedIdentity.deserializeBinary(endorsement.getEndorser_asU8()).getMspid()).sort();
}

async function probeDiscovery(gatewayPool, mspId, organizations, deadline) {
    // Contract API's system metadata function only reads in-memory metadata.
    // Endorse exercises Gateway's discovery plan; never submit this proposal.
    const proposal = gatewayPool.connection(mspId).contract.newProposal('org.hyperledger.fabric:GetMetadata', {
        endorsingOrganizations: organizations,
    });
    const transaction = await proposal.endorse({deadline});
    const observed = endorsementOrganizations(transaction.getBytes());
    if (JSON.stringify(observed) !== JSON.stringify([...organizations].sort())) {
        throw new Error(`discovery readiness endorsements differ for ${mspId}`);
    }
    return observed;
}

async function waitForDiscovery(gatewayPool, credentials, options = {}) {
    const organizations = credentials.map((entry) => entry.mspId).sort();
    if (organizations.length !== 7 || new Set(organizations).size !== 7) {
        throw new Error('discovery readiness requires seven distinct organizations');
    }
    const timeoutMs = options.timeoutMs ?? 180_000;
    const intervalMs = options.intervalMs ?? 1_000;
    const probeTimeoutMs = options.probeTimeoutMs ?? 10_000;
    if (![timeoutMs, intervalMs, probeTimeoutMs].every((value) => Number.isInteger(value) && value > 0)) {
        throw new Error('discovery readiness timeouts must be positive integer milliseconds');
    }
    const now = options.now ?? Date.now;
    const delay = options.delay ?? timers.setTimeout;
    const probe = options.probe ?? probeDiscovery;
    const startedAt = now();
    const deadline = startedAt + timeoutMs;
    let successfulSweeps = 0;
    let sweeps = 0;
    let lastFailure;
    while (now() < deadline) {
        sweeps += 1;
        let successful = true;
        for (const mspId of organizations) {
            if (now() >= deadline) {successful = false; break;}
            const started = now();
            try {
                const endorsers = await probe(gatewayPool, mspId, organizations,
                    Math.min(deadline, now() + probeTimeoutMs));
                await options.onAttempt?.({sweep: sweeps, gateway: mspId, ready: true,
                    endorsers, elapsedMs: now() - started});
            } catch (error) {
                error.phase = 'endorsement';
                if (!isTransientGrpcError(error)) throw error;
                lastFailure = error;
                successful = false;
                await options.onAttempt?.({sweep: sweeps, gateway: mspId, ready: false,
                    elapsedMs: now() - started, error: preparationErrorDiagnostics(error)});
            }
        }
        successfulSweeps = successful ? successfulSweeps + 1 : 0;
        if (successfulSweeps === 2 && now() <= deadline) {
            return {gateways: organizations, endorsersPerGateway: organizations.length,
                successfulSweeps, sweeps, durationSeconds: (now() - startedAt) / 1_000};
        }
        if (now() < deadline) await delay(Math.min(intervalMs, deadline - now()));
    }
    throw new Error(`discovery readiness exceeded ${timeoutMs} ms; preparation/measurement must not start`, {
        cause: lastFailure,
    });
}

module.exports = {endorsementOrganizations, probeDiscovery, waitForDiscovery};
