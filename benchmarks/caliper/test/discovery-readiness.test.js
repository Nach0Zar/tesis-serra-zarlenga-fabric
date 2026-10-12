'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const protos = require('@hyperledger/fabric-protos');

const {endorsementOrganizations, probeDiscovery, waitForDiscovery} = require('../src/discovery-readiness');

const organizations = ['AnmatMSP', 'LabMSP', 'DrogueriaMSP', 'DistribuidorMSP',
    'FarmaciaMSP', 'CentroMedicoMSP', 'FinanciadorMSP'].sort();
const credentials = organizations.map((mspId) => ({mspId}));

function prepared(identities = organizations) {
    const endorsed = new protos.peer.ChaincodeEndorsedAction();
    for (const mspId of identities) {
        const identity = new protos.msp.SerializedIdentity();
        identity.setMspid(mspId);
        const endorsement = new protos.peer.Endorsement();
        endorsement.setEndorser(identity.serializeBinary());
        endorsed.addEndorsements(endorsement);
    }
    const actionPayload = new protos.peer.ChaincodeActionPayload();
    actionPayload.setAction(endorsed);
    const action = new protos.peer.TransactionAction();
    action.setPayload(actionPayload.serializeBinary());
    const transaction = new protos.peer.Transaction();
    transaction.addActions(action);
    const payload = new protos.common.Payload();
    payload.setData(transaction.serializeBinary());
    const envelope = new protos.common.Envelope();
    envelope.setPayload(payload.serializeBinary());
    const result = new protos.gateway.PreparedTransaction();
    result.setEnvelope(envelope);
    return result.serializeBinary();
}

function clock() {
    let time = 0;
    return {now: () => time, delay: async (milliseconds) => {time += milliseconds;}};
}

test('readiness uses system metadata, verifies all seven endorsements, and never submits', async () => {
    let endorsed = 0;
    const pool = {connection(mspId) {
        assert.equal(mspId, 'CentroMedicoMSP');
        return {contract: {newProposal(operation, options) {
            assert.equal(operation, 'org.hyperledger.fabric:GetMetadata');
            assert.deepEqual(options, {endorsingOrganizations: organizations});
            return {async endorse({deadline}) {
                assert.equal(deadline, 123);
                endorsed += 1;
                return {getBytes: () => prepared(), submit: () => assert.fail('readiness must never submit')};
            }};
        }}};
    }};
    assert.deepEqual(await probeDiscovery(pool, 'CentroMedicoMSP', organizations, 123), organizations);
    assert.equal(endorsed, 1);
    assert.deepEqual(endorsementOrganizations(prepared()), organizations);
});

test('readiness rejects omitted, duplicated and unexpected endorsers', async () => {
    for (const identities of [organizations.slice(1), [...organizations.slice(1), organizations[1]],
        [...organizations, 'UnexpectedMSP']]) {
        const pool = {connection: () => ({contract: {newProposal: () => ({
            endorse: async () => ({getBytes: () => prepared(identities)}),
        })}})};
        await assert.rejects(probeDiscovery(pool, 'CentroMedicoMSP', organizations, 123), /endorsements differ/u);
    }
});

test('local availability does not bypass remote discovery; two complete sweeps are required', async () => {
    const attempts = [];
    let calls = 0;
    const result = await waitForDiscovery({}, credentials, {...clock(), onAttempt: async (attempt) => attempts.push(attempt),
        probe: async (_pool, mspId, targets) => {
            calls += 1;
            assert.deepEqual(targets, organizations);
            if (calls <= 7 && mspId === 'CentroMedicoMSP') {
                throw Object.assign(new Error('failed to find any endorsing peers for org(s): DrogueriaMSP'), {code: 14});
            }
            return organizations;
        }});
    assert.equal(result.sweeps, 3);
    assert.equal(result.successfulSweeps, 2);
    assert.equal(calls, 21);
    assert.equal(attempts.filter((attempt) => !attempt.ready).length, 1);
    assert.equal(attempts.find((attempt) => !attempt.ready).error.phase, 'endorsement');
});

test('readiness timeout blocks preparation instead of exhausting writing retries', async () => {
    let calls = 0;
    await assert.rejects(waitForDiscovery({}, credentials, {...clock(), timeoutMs: 2_000,
        probe: async () => {calls += 1; throw Object.assign(new Error('unavailable'), {code: 14});}}),
    /preparation\/measurement must not start/u);
    assert.equal(calls, 14);
});

test('readiness rejects permanent, mixed and operator cancellation errors without retry', async () => {
    const errors = [Object.assign(new Error('permission denied'), {code: 7}),
        Object.assign(new Error('aborted'), {code: 10, details: [
            {message: 'rpc error: code = DeadlineExceeded desc = context deadline exceeded'},
            {message: 'rpc error: code = PermissionDenied desc = access denied'},
        ]}), Object.assign(new Error('operator cancelled'), {code: 1, operatorCancelled: true}),
        Object.assign(new Error('aborted'), {name: 'AbortError'})];
    for (const error of errors) {
        let calls = 0;
        await assert.rejects(waitForDiscovery({}, credentials, {...clock(),
            probe: async () => {calls += 1; throw error;}}), (observed) => observed === error);
        assert.equal(calls, 1);
    }
});

test('readiness rejects invalid topology and timing before contacting peers', async () => {
    await assert.rejects(waitForDiscovery({}, credentials.slice(1)), /seven distinct/u);
    await assert.rejects(waitForDiscovery({}, [...credentials.slice(1), credentials[1]]), /seven distinct/u);
    await assert.rejects(waitForDiscovery({}, credentials, {timeoutMs: 0}), /positive integer/u);
    await assert.rejects(waitForDiscovery({}, credentials, {intervalMs: -1}), /positive integer/u);
});
