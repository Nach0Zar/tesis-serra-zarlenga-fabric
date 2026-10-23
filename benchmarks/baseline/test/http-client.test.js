'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const {BaselineClient, requestForInvocation, validateBaseURL} = require('../src/http-client');

function invocation(operation) {
    return {
        operation, invokerMspId: 'LabMSP',
        request: {
            gtin: '07791234567898', numeroSerie: 'SN / 1', lote: 'LOT-1', fechaVencimiento: '2099-01-01',
        },
    };
}

test('requests translate the five core operations without changing their semantics', () => {
    assert.deepEqual(requestForInvocation(invocation('RegisterUnit')), {
        method: 'POST', path: '/v1/units',
        body: {gtin: '07791234567898', numeroSerie: 'SN / 1', lote: 'LOT-1', fechaVencimiento: '2099-01-01'},
    });
    const dispatch = invocation('DispatchTransfer');
    dispatch.privateData = {
        destinatario: {destino: 'GLN:7791234500024'},
        commercial: {numeroRemito: 'R-1', numeroFactura: 'F-1', cantidad: 1},
    };
    assert.deepEqual(requestForInvocation(dispatch), {
        method: 'POST', path: '/v1/units/07791234567898/SN%20%2F%201/dispatch',
        body: {destino: 'GLN:7791234500024', numeroRemito: 'R-1', numeroFactura: 'F-1', cantidad: 1},
    });
    assert.deepEqual(requestForInvocation(invocation('ReceiveTransfer')), {
        method: 'POST', path: '/v1/units/07791234567898/SN%20%2F%201/receive',
    });
    assert.deepEqual(requestForInvocation(invocation('Dispense')), {
        method: 'POST', path: '/v1/units/07791234567898/SN%20%2F%201/dispense',
    });
    assert.deepEqual(requestForInvocation(invocation('ReadUnit')), {
        method: 'GET', path: '/v1/units/07791234567898/SN%20%2F%201',
    });
    assert.deepEqual(requestForInvocation(invocation('GetUnitHistory')), {
        method: 'GET', path: '/v1/units/07791234567898/SN%20%2F%201/history',
    });
});

test('client keeps credentials in memory and classifies only the contractual body code', async () => {
    let captured;
    let now = 1000;
    const client = new BaselineClient({
        baseURL: 'http://127.0.0.1:18080',
        credentials: new Map([['LabMSP', {key: 'super-secret', role: 'operator'}]]),
        now: () => { const value = now; now += 7; return value; },
        fetchImplementation: async (url, options) => {
            captured = {url, options};
            return {ok: false, status: 409, text: async () => '{"code":"UNIT_ALREADY_EXISTS","message":"duplicate"}'};
        },
    });
    const result = await client.invoke(invocation('RegisterUnit'));
    assert.equal(captured.options.headers['X-Org-Key'], 'super-secret');
    assert.equal(result.transaction.errorCode, 'UNIT_ALREADY_EXISTS');
    assert.equal(result.transaction.latencyMs, 7);
    assert.doesNotMatch(JSON.stringify(result), /super-secret/u);
});

test('reads need no key, transport failures are processable and remote URLs are rejected', async () => {
    const client = new BaselineClient({
        baseURL: 'http://localhost:18080',
        fetchImplementation: async (_url, options) => {
            assert.equal(options.headers['X-Org-Key'], undefined);
            const error = new Error('timeout');
            error.name = 'TimeoutError';
            throw error;
        },
    });
    const result = await client.invoke(invocation('ReadUnit'));
    assert.equal(result.ok, false);
    assert.equal(result.transaction.transportError, 'TimeoutError');
    assert.throws(() => validateBaseURL('https://example.com'), /loopback/u);
});

test('successful HTTP responses with invalid JSON remain unexpected failures', async () => {
    const client = new BaselineClient({
        baseURL: 'http://127.0.0.1:18080',
        fetchImplementation: async () => ({ok: true, status: 200, text: async () => ''}),
    });
    const result = await client.invoke(invocation('ReadUnit'));
    assert.equal(result.ok, false);
    assert.equal(result.transaction.status, 'failed');
    assert.equal(result.transaction.responseError, 'INVALID_RESPONSE_BODY');
});
