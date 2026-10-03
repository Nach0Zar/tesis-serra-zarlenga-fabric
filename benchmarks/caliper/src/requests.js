'use strict';

const {identityNameForMsp} = require('./network-config');

const ALLOWED_OPERATIONS = new Set([
    'RegisterUnit', 'DispatchTransfer', 'ReceiveTransfer', 'Dispense', 'ReadUnit', 'GetUnitHistory',
]);
const PREPARATION_OPERATIONS = new Set([
    ...ALLOWED_OPERATIONS,
    'ProhibitProduct', 'Quarantine', 'ReportDamaged', 'ReportExpired', 'ReturnProduct', 'WithdrawFromMarket',
]);

function normalizeInvocation(invocation, targetMspIds, options = {}) {
    const allowed = options.allowPreparation ? PREPARATION_OPERATIONS : ALLOWED_OPERATIONS;
    if (!invocation || !allowed.has(invocation.operation)) {
        throw new Error(`unsupported workload operation ${invocation?.operation}`);
    }
    if (!invocation.invokerMspId || !invocation.request?.gtin || !invocation.request?.numeroSerie) {
        throw new Error(`${invocation.operation} recipe is incomplete`);
    }
    const targets = targetMspIds ?? (ALLOWED_OPERATIONS.has(invocation.operation) ? [invocation.invokerMspId] : []);
    return {...invocation, targetMspIds: targets};
}

function transientMap(invocation) {
    if (invocation.operation !== 'DispatchTransfer') {
        return undefined;
    }
    if (!invocation.privateData?.destinatario || !invocation.privateData?.commercial) {
        throw new Error('DispatchTransfer recipe is missing private data');
    }
    return {
        destinatario: JSON.stringify(invocation.privateData.destinatario),
        commercial: JSON.stringify(invocation.privateData.commercial),
    };
}

function contractArguments(invocation) {
    const request = invocation.request;
    if (['ReadUnit', 'GetUnitHistory'].includes(invocation.operation)) {
        return [request.gtin, request.numeroSerie];
    }
    return [JSON.stringify(request)];
}

function buildCaliperRequest(invocation) {
    const normalized = normalizeInvocation(invocation, invocation.targetMspIds);
    const request = {
        contractId: 'snt',
        contractFunction: normalized.operation,
        contractArguments: contractArguments(normalized),
        readOnly: ['ReadUnit', 'GetUnitHistory'].includes(normalized.operation),
        invokerIdentity: identityNameForMsp(normalized.invokerMspId),
        invokerMspId: normalized.invokerMspId,
    };
    if (!request.readOnly) {
        request.targetOrganizations = normalized.targetMspIds;
    }
    const transient = transientMap(normalized);
    if (transient) {
        request.transientMap = transient;
    }
    return request;
}

module.exports = {
    ALLOWED_OPERATIONS,
    PREPARATION_OPERATIONS,
    buildCaliperRequest,
    contractArguments,
    normalizeInvocation,
    transientMap,
};
