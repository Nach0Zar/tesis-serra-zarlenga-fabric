'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const {prepareSnapshot} = require('../src/snapshot');

test('snapshot preparation retries only a transient internal reception outside the measured window', async () => {
    let receiveCalls = 0;
    const delays = [];
    const result = await prepareSnapshot({invoke: async (invocation) => {
        if (invocation.operation === 'DispatchTransfer') {
            return {ok: true, value: {gtin: '1'}, transaction: {}};
        }
        receiveCalls += 1;
        if (receiveCalls === 1) {
            return {ok: false, errorEnvelope: {code: 'INTERNAL_ERROR'}, transaction: {errorCode: 'INTERNAL_ERROR'}};
        }
        return {ok: true, value: {gtin: '1'}, transaction: {}};
    }}, {
        preparations: [{datasetSequence: 1, invocations: [
            {operation: 'DispatchTransfer'}, {operation: 'ReceiveTransfer'},
        ]}],
    }, new Map([[1, {}]]), 1, {
        retryDelayMs: 1100,
        sleep: async (delay) => delays.push(delay),
    });
    assert.deepEqual(result, {recipes: 1, receiveRetries: 1});
    assert.deepEqual(delays, [1100]);
    assert.equal(receiveCalls, 2);
});

test('snapshot preparation does not retry unrelated failures', async () => {
    await assert.rejects(() => prepareSnapshot({invoke: async () => ({
        ok: false, errorEnvelope: {code: 'INTERNAL_ERROR'}, transaction: {errorCode: 'INTERNAL_ERROR'},
    })}, {
        preparations: [{datasetSequence: 2, invocations: [{operation: 'Dispense'}]}],
    }, new Map([[2, {}]]), 1, {sleep: async () => assert.fail('must not sleep')}), /Dispense failed/u);
});
