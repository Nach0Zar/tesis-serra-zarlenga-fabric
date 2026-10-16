'use strict';

const READ_OPERATIONS = new Set(['ReadUnit', 'GetUnitHistory']);
const EMPTY_BODY_OPERATIONS = new Set(['ReceiveTransfer', 'Dispense']);
const PATHS = Object.freeze({
    DispatchTransfer: 'dispatch',
    ReceiveTransfer: 'receive',
    Dispense: 'dispense',
    ProhibitProduct: 'prohibit-product',
    Quarantine: 'quarantine',
    ReportDamaged: 'report-damaged',
    ReportExpired: 'report-expired',
    ReturnProduct: 'return',
    WithdrawFromMarket: 'withdraw-from-market',
});

function validateBaseURL(value) {
    const url = new URL(value);
    if (url.protocol !== 'http:' || !['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname)) {
        throw new Error('baseline API URL must use HTTP over loopback');
    }
    return url.toString().replace(/\/$/u, '');
}

function unitPath(request) {
    return `/v1/units/${encodeURIComponent(request.gtin)}/${encodeURIComponent(request.numeroSerie)}`;
}

function requestForInvocation(invocation) {
    if (!invocation?.operation || !invocation.request?.gtin || !invocation.request?.numeroSerie) {
        throw new Error('baseline invocation is incomplete');
    }
    const request = invocation.request;
    if (invocation.operation === 'RegisterUnit') {
        return {
            method: 'POST', path: '/v1/units', body: {
                gtin: request.gtin,
                numeroSerie: request.numeroSerie,
                lote: request.lote,
                fechaVencimiento: request.fechaVencimiento,
            },
        };
    }
    if (invocation.operation === 'ReadUnit') return {method: 'GET', path: unitPath(request)};
    if (invocation.operation === 'GetUnitHistory') return {method: 'GET', path: `${unitPath(request)}/history`};
    const suffix = PATHS[invocation.operation];
    if (!suffix) throw new Error(`unsupported baseline operation ${invocation.operation}`);
    if (invocation.operation === 'DispatchTransfer') {
        const destination = invocation.privateData?.destinatario?.destino;
        const commercial = invocation.privateData?.commercial;
        if (!destination || !commercial) throw new Error('DispatchTransfer recipe is missing private data');
        return {
            method: 'POST', path: `${unitPath(request)}/${suffix}`, body: {
                destino: destination,
                numeroRemito: commercial.numeroRemito,
                numeroFactura: commercial.numeroFactura,
                cantidad: commercial.cantidad,
            },
        };
    }
    if (EMPTY_BODY_OPERATIONS.has(invocation.operation)) {
        return {method: 'POST', path: `${unitPath(request)}/${suffix}`};
    }
    const body = {motivo: request.motivo};
    if (invocation.operation === 'ReturnProduct' && request.receptor) body.receptor = request.receptor;
    return {method: 'POST', path: `${unitPath(request)}/${suffix}`, body};
}

class BaselineClient {
    constructor({baseURL, credentials = new Map(), fetchImplementation = globalThis.fetch, timeoutMs = 30_000, now = Date.now}) {
        this.baseURL = validateBaseURL(baseURL);
        this.credentials = credentials;
        this.fetch = fetchImplementation;
        this.timeoutMs = timeoutMs;
        this.now = now;
    }

    async invoke(invocation) {
        const request = requestForInvocation(invocation);
        const headers = {Accept: 'application/json'};
        if (!READ_OPERATIONS.has(invocation.operation)) {
            const credential = this.credentials.get(invocation.invokerMspId);
            if (!credential) throw new Error(`missing baseline credential for ${invocation.invokerMspId}`);
            headers['X-Org-Key'] = credential.key;
        }
        const options = {method: request.method, headers, signal: AbortSignal.timeout(this.timeoutMs)};
        if (request.body !== undefined) {
            headers['Content-Type'] = 'application/json';
            options.body = JSON.stringify(request.body);
        }
        const started = this.now();
        const startedAt = new Date(started).toISOString();
        let response;
        let text;
        try {
            response = await this.fetch(`${this.baseURL}${request.path}`, options);
            text = await response.text();
        } catch (error) {
            const ended = this.now();
            return {
                ok: false,
                error,
                transaction: {
                    function: invocation.operation,
                    startedAt,
                    endedAt: new Date(ended).toISOString(),
                    latencyMs: Math.max(0, ended - started),
                    status: 'failed', retry: false,
                    transportError: error.name,
                },
            };
        }
        const ended = this.now();
        let value;
        let validJSON = false;
        if (text !== '') {
            try {
                value = JSON.parse(text);
                validJSON = true;
            } catch {
                value = undefined;
            }
        }
        const validResponse = response.ok && validJSON;
        const errorCode = response.ok ? undefined : value?.code;
        return {
            ok: validResponse,
            value,
            errorEnvelope: validResponse ? undefined : value,
            transaction: {
                function: invocation.operation,
                startedAt,
                endedAt: new Date(ended).toISOString(),
                latencyMs: Math.max(0, ended - started),
                status: validResponse ? 'success' : 'failed',
                retry: false,
                errorCode,
                httpStatus: response.status,
                responseError: response.ok && !validJSON ? 'INVALID_RESPONSE_BODY' : undefined,
            },
        };
    }
}

module.exports = {BaselineClient, READ_OPERATIONS, requestForInvocation, validateBaseURL};
