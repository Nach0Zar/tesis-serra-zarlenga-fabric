'use strict';

const fs = require('node:fs');

const {WorkloadModuleBase} = require('@hyperledger/caliper-core');

const {GatewayPool, measuredGatewayInvocation} = require('../src/gateway-bridge');
const {appendOperation} = require('../src/raw-results');
const {buildCaliperRequest} = require('../src/requests');

function transactionRecord(status, functionName, retry = false, errorCode) {
    const started = status.GetTimeCreate();
    const ended = status.GetTimeFinal();
    return {
        function: functionName,
        transactionId: status.GetID() || undefined,
        startedAt: new Date(started).toISOString(),
        endedAt: new Date(ended).toISOString(),
        latencyMs: Math.max(0, ended - started),
        status: status.GetStatus(),
        retry,
        errorCode,
    };
}

async function standardInvocation(sutAdapter, invocation) {
    const response = await sutAdapter.sendRequests(buildCaliperRequest(invocation));
    const status = Array.isArray(response) ? response[0] : response;
    if (!status?.GetStatus) throw new Error(`${invocation.operation} returned no Caliper transaction status`);
    return {status, transaction: transactionRecord(status, invocation.operation)};
}

async function executeOperation({
    operation,
    sutAdapter,
    gatewayPool,
    measuredInvocation = measuredGatewayInvocation,
}) {
    const started = Date.now();
    const transactions = [];
    let outcome = 'success';
    let observedErrorCode;

    if (operation.expectedRejection) {
        const invocation = operation.invocations[0];
        const measured = await measuredInvocation({gatewayPool, sutAdapter, invocation});
        observedErrorCode = measured.envelope?.code;
        transactions.push(transactionRecord(measured.status, invocation.operation, false, observedErrorCode));
        outcome = measured.status.GetStatus() === 'failed'
            && observedErrorCode === operation.expectedRejection.code
            ? 'expected-rejection' : 'unexpected-failure';
    } else if (operation.type === 'transfer') {
        const dispatch = await standardInvocation(sutAdapter, operation.invocations[0]);
        transactions.push(dispatch.transaction);
        if (dispatch.status.GetStatus() !== 'success') {
            outcome = 'unexpected-failure';
        } else {
            await gatewayPool.waitForUnitState(
                operation.invocations[1].targetMspIds,
                operation.invocations[1].request,
                'EN_TRANSITO',
            );
            const receive = await measuredInvocation({
                gatewayPool,
                sutAdapter,
                invocation: operation.invocations[1],
                retryPrivateData: true,
            });
            receive.attempts.forEach((attempt, index) => transactions.push(transactionRecord(
                attempt.status,
                operation.invocations[1].operation,
                index > 0,
                attempt.envelope?.code,
            )));
            if (receive.status.GetStatus() !== 'success') outcome = 'unexpected-failure';
        }
    } else {
        const invocation = operation.invocations[0];
        const measured = await standardInvocation(sutAdapter, invocation);
        transactions.push(measured.transaction);
        if (measured.status.GetStatus() !== 'success') outcome = 'unexpected-failure';
    }

    const ended = Date.now();
    return {
        workerIndex: operation.workerIndex,
        ordinal: operation.ordinal,
        datasetSequence: operation.datasetSequence,
        type: operation.type,
        outcome,
        expectedRejection: operation.expectedRejection,
        observedErrorCode,
        startedAt: new Date(started).toISOString(),
        endedAt: new Date(ended).toISOString(),
        latencyMs: ended - started,
        transactions,
    };
}

class CoreWorkload extends WorkloadModuleBase {
    constructor(expectedType) {
        super();
        this.expectedType = expectedType;
        this.nextOperation = 0;
        this.gatewayPool = undefined;
    }

    async initializeWorkloadModule(workerIndex, totalWorkers, roundIndex, roundArguments, sutAdapter, sutContext) {
        await super.initializeWorkloadModule(workerIndex, totalWorkers, roundIndex, roundArguments, sutAdapter, sutContext);
        const plan = JSON.parse(fs.readFileSync(roundArguments.workPlanPath, 'utf8'));
        const worker = plan.workers?.find((entry) => entry.workerIndex === workerIndex);
        if (!worker || plan.profile.workers !== totalWorkers) {
            throw new Error('work plan does not match Caliper workers');
        }
        if (plan.profile.module !== roundArguments.moduleName) {
            throw new Error('work plan module does not match benchmark configuration');
        }
        this.operations = worker.operations.map((operation) => ({...operation, workerIndex}));
        this.partPath = roundArguments.partPaths[workerIndex];
        if (!this.partPath) throw new Error(`missing raw part path for worker ${workerIndex}`);
        if (['transfer', 'mixed', 'expected-rejection'].includes(this.expectedType)) {
            const networkConfig = JSON.parse(fs.readFileSync(roundArguments.networkConfigPath, 'utf8'));
            this.gatewayPool = new GatewayPool(networkConfig);
        }
    }

    async submitTransaction() {
        const operation = this.operations[this.nextOperation];
        if (!operation) throw new Error(`worker ${this.workerIndex} exhausted its deterministic work plan`);
        this.nextOperation += 1;
        if (this.expectedType !== 'mixed' && this.expectedType !== 'expected-rejection'
            && operation.type !== this.expectedType) {
            throw new Error(`expected ${this.expectedType} operation, got ${operation.type}`);
        }
        const record = await executeOperation({operation, sutAdapter: this.sutAdapter, gatewayPool: this.gatewayPool});
        appendOperation(this.partPath, record);
    }

    async cleanupWorkloadModule() {
        this.gatewayPool?.close();
    }
}

function createCoreWorkload(expectedType) {
    return new CoreWorkload(expectedType);
}

module.exports = {CoreWorkload, createCoreWorkload, executeOperation, standardInvocation, transactionRecord};
