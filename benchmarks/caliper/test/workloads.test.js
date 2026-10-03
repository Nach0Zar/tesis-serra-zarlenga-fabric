'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const {TxStatus} = require('@hyperledger/caliper-core');

const {extractContractError, isPrivateDataNotDisseminated} = require('../src/contract-errors');
const {invokePreparation, measuredGatewayInvocation} = require('../src/gateway-bridge');
const {buildRoundMetadata} = require('../src/metadata');
const {MIX_PATTERN, buildPlan, candidatePools} = require('../src/planner');
const {buildProfile, weightedTransactionsPerOperation} = require('../src/profiles');
const {aggregateResults, appendOperation} = require('../src/raw-results');
const {buildCaliperRequest} = require('../src/requests');
const {buildBenchmarkConfig, preparePlan} = require('../src/run-round');
const {executeOperation} = require('../workloads/core-workload');

function invocation(operation, invokerMspId, sequence) {
    const value = {
        operation,
        invokerMspId,
        request: {gtin: '07791234567898', numeroSerie: `SN-${sequence}`},
    };
    if (operation === 'RegisterUnit') {
        value.request.lote = `LOT-${sequence}`;
        value.request.fechaVencimiento = '2099-01-01';
    }
    if (operation === 'DispatchTransfer') {
        value.privateData = {
            destinatario: {destino: 'GLN:7791234500024'},
            commercial: {numeroRemito: `R-${sequence}`, numeroFactura: `F-${sequence}`, cantidad: 1},
        };
    }
    return value;
}

function happyUnit(sequence) {
    return {
        sequence,
        preparation: [
            invocation('RegisterUnit', 'LabMSP', sequence),
            invocation('DispatchTransfer', 'LabMSP', sequence),
            invocation('ReceiveTransfer', 'DrogueriaMSP', sequence),
        ],
        expectedSuccess: invocation('Dispense', 'DrogueriaMSP', sequence),
    };
}

function status(state, start = 1000, end = 1010) {
    const value = new TxStatus('tx-id');
    value.SetTimeCreate(start);
    value.SetVerification(true);
    if (state === 'success') value.SetStatusSuccess(end);
    else value.SetStatusFail();
    return value;
}

test('measurement profiles accept only their exact rates and phase/repetition pairs', () => {
    const transfer = buildProfile({scenario: 'write-transfer', phase: 'warmup', repetition: '0', rate: '10'});
    assert.equal(transfer.transactionsPerOperation, 2);
    assert.equal(transfer.workers, 2);
    assert.throws(() => buildProfile({scenario: 'write-transfer', phase: 'warmup', repetition: '1', rate: '10'}));
    assert.throws(() => buildProfile({scenario: 'read-unit', phase: 'measurement', repetition: '1', rate: '20'}));

    const blocking = buildProfile({
        scenario: 'expected-rejections', phase: 'measurement', repetition: '1', rate: '5',
        family: 'BLOCKING_STATE', operation: 'dispense',
    });
    assert.equal(blocking.operation, 'dispense');
    assert.equal(blocking.transactionsPerOperation, 1);
    assert.equal(weightedTransactionsPerOperation({register: 10, transfer: 55, dispense: 10, query: 25}), 1.55);
    assert.throws(
        () => weightedTransactionsPerOperation({register: 10, transfer: 50, dispense: 10, query: 25}),
        /must sum to 100/u,
    );
});

test('Caliper controller converts conceptual rates to transaction rates', () => {
    const paths = {
        workPlanPath: '/tmp/work-plan.json', networkConfigPath: '/tmp/network-config.json',
        partPaths: ['/tmp/raw-0.jsonl', '/tmp/raw-1.jsonl'],
    };
    const transfer = buildProfile({scenario: 'write-transfer', phase: 'warmup', repetition: '0', rate: '5'});
    const mixed = buildProfile({scenario: 'mixed', phase: 'warmup', repetition: '0', rate: '20'});
    assert.equal(buildBenchmarkConfig('/repo', transfer, paths).test.rounds[0].rateControl.opts.tps, 10);
    assert.equal(buildBenchmarkConfig('/repo', mixed, paths).test.rounds[0].rateControl.opts.tps, 31);
});

test('planner pairs transfers and excludes every M3 operation', () => {
    const pools = candidatePools({units: [happyUnit(1)]});
    assert.equal(pools.transfer.length, 1);
    assert.deepEqual(pools.transfer[0].invocations.map((entry) => entry.operation), ['DispatchTransfer', 'ReceiveTransfer']);
    assert.deepEqual(pools.transfer[0].invocations[1].targetMspIds, ['LabMSP', 'DrogueriaMSP']);
    const serialized = JSON.stringify(pools);
    for (const prohibited of ['QueryUnitsByState', 'VerifyUnit', 'VerifyTrace', 'GetLabInterventionHistory']) {
        assert.doesNotMatch(serialized, new RegExp(prohibited, 'u'));
    }
});

test('planner isolates all four expected-rejection variants with dataset codes', () => {
    const rejectionUnit = (sequence, category, operation, expectedErrorCode) => ({
        sequence,
        preparation: [invocation('RegisterUnit', 'LabMSP', sequence)],
        expectedRejection: {
            category,
            operation,
            invokerMspId: operation === 'RegisterUnit' ? 'LabMSP' : 'FarmaciaMSP',
            request: invocation(operation, 'FarmaciaMSP', sequence).request,
            privateData: operation === 'DispatchTransfer'
                ? invocation(operation, 'FarmaciaMSP', sequence).privateData : undefined,
            expectedErrorCode,
        },
    });
    const dataset = {units: [
        rejectionUnit(1, 'UNAUTHORIZED_TRANSFER', 'DispatchTransfer', 'TRANSFER_NOT_AUTHORIZED'),
        rejectionUnit(2, 'DUPLICATE_IDENTITY', 'RegisterUnit', 'UNIT_ALREADY_EXISTS'),
        rejectionUnit(3, 'BLOCKING_STATE', 'DispatchTransfer', 'INVALID_STATE_TRANSITION'),
        rejectionUnit(4, 'BLOCKING_STATE', 'Dispense', 'INVALID_STATE_TRANSITION'),
    ]};
    const cases = [
        ['UNAUTHORIZED_TRANSFER', undefined, 'DispatchTransfer', 'TRANSFER_NOT_AUTHORIZED'],
        ['DUPLICATE_IDENTITY', undefined, 'RegisterUnit', 'UNIT_ALREADY_EXISTS'],
        ['BLOCKING_STATE', 'transfer', 'DispatchTransfer', 'INVALID_STATE_TRANSITION'],
        ['BLOCKING_STATE', 'dispense', 'Dispense', 'INVALID_STATE_TRANSITION'],
    ];
    for (const [family, operation, functionName, code] of cases) {
        const profile = buildProfile({
            scenario: 'expected-rejections', phase: 'warmup', repetition: '0', rate: '5', family, operation,
        });
        const plan = buildPlan(dataset, profile);
        const measured = plan.workers.flatMap((worker) => worker.operations);
        assert.equal(measured.every((entry) => entry.invocations[0].operation === functionName), true);
        assert.equal(measured.every((entry) => entry.expectedRejection.code === code), true);
    }
});

test('extraordinary operations are accepted only as dataset preparation', () => {
    const unit = happyUnit(1);
    unit.preparation.push({
        operation: 'Quarantine', invokerMspId: 'DrogueriaMSP',
        request: {gtin: '07791234567898', numeroSerie: 'SN-1', motivo: 'synthetic preparation'},
    });
    assert.doesNotThrow(() => candidatePools({units: [unit]}));
    assert.throws(() => buildCaliperRequest(unit.preparation.at(-1)), /unsupported workload operation/u);
});

test('mixed plan is deterministic and uses 2/11/2/5 operations per cycle', () => {
    assert.deepEqual(
        ['register', 'transfer', 'dispense', 'query-unit'].map((type) => MIX_PATTERN.filter((entry) => entry === type).length),
        [2, 11, 2, 5],
    );
    const dataset = {units: Array.from({length: 700}, (_, index) => happyUnit(index + 1))};
    const profile = {
        scenario: 'mixed', operation: 'mixed', workers: 2, durationSeconds: 1, rate: 1,
        module: 'mixed', transactionsPerOperation: 1.55, mix: {register: 10, transfer: 55, dispense: 10, query: 25},
    };
    const first = buildPlan(dataset, profile);
    const second = buildPlan(dataset, profile);
    assert.deepEqual(first, second);
    for (const worker of first.workers) {
        assert.equal(worker.operations.length, 40);
        assert.deepEqual(
            ['register', 'transfer', 'dispense', 'query-unit'].map((type) =>
                worker.operations.filter((entry) => entry.type === type).length),
            [4, 22, 4, 10],
        );
    }
    const sequences = first.workers.flatMap((worker) => worker.operations.map((entry) => entry.datasetSequence));
    assert.equal(new Set(sequences).size, sequences.length);
});

test('core scenarios reserve disjoint deterministic dataset bands', () => {
    const dataset = {units: Array.from({length: 700}, (_, index) => happyUnit(index + 1))};
    const profile = (scenario, operation) => ({
        scenario, operation, workers: 2, durationSeconds: 1, rate: 1,
        module: operation, transactionsPerOperation: operation === 'transfer' ? 2 : 1,
    });
    const scenarios = [
        profile('write-register', 'register'),
        profile('write-transfer', 'transfer'),
        profile('write-dispense', 'dispense'),
        profile('read-unit', 'query-unit'),
        profile('read-history', 'query-history'),
    ];
    const sequenceSets = scenarios.map((entry) => new Set(
        buildPlan(dataset, entry).workers.flatMap((worker) =>
            worker.operations.map((operation) => operation.datasetSequence)),
    ));
    for (let left = 0; left < sequenceSets.length; left += 1) {
        for (let right = left + 1; right < sequenceSets.length; right += 1) {
            assert.equal([...sequenceSets[left]].some((sequence) => sequenceSets[right].has(sequence)), false);
        }
    }
});

test('preparation runs independent recipes concurrently and preserves each recipe order', async () => {
    const activeByUnit = new Set();
    const completedByUnit = new Map();
    let maximumConcurrency = 0;
    const plan = {
        preparations: Array.from({length: 6}, (_, sequence) => ({
            invocations: [
                invocation('RegisterUnit', 'LabMSP', sequence),
                invocation('DispatchTransfer', 'LabMSP', sequence),
            ],
        })),
    };
    await preparePlan(plan, {}, {
        gatewayPool: {},
        concurrency: 3,
        quiet: true,
        invoke: async (_pool, entry) => {
            const unit = entry.request.numeroSerie;
            const completed = completedByUnit.get(unit) ?? [];
            if (entry.operation === 'RegisterUnit') activeByUnit.add(unit);
            else assert.deepEqual(completed, ['RegisterUnit']);
            maximumConcurrency = Math.max(maximumConcurrency, activeByUnit.size);
            await new Promise((resolve) => setImmediate(resolve));
            completed.push(entry.operation);
            completedByUnit.set(unit, completed);
            if (entry.operation === 'DispatchTransfer') activeByUnit.delete(unit);
        },
    });
    assert.equal(maximumConcurrency, 3);
    assert.equal(completedByUnit.size, 6);
    for (const completed of completedByUnit.values()) {
        assert.deepEqual(completed, ['RegisterUnit', 'DispatchTransfer']);
    }
});

test('receive preparation waits for all endorsers to observe transit without retrying the transaction', async () => {
    const calls = [];
    const receive = {
        ...invocation('ReceiveTransfer', 'DrogueriaMSP', 1),
        targetMspIds: ['LabMSP', 'DrogueriaMSP'],
    };
    const pool = {
        waitForUnitState: async (...arguments_) => calls.push(['wait', ...arguments_]),
        invoke: async (entry) => calls.push(['invoke', entry.operation]),
    };
    await invokePreparation(pool, receive);
    assert.deepEqual(calls, [
        ['wait', ['LabMSP', 'DrogueriaMSP'], receive.request, 'EN_TRANSITO'],
        ['invoke', 'ReceiveTransfer'],
    ]);
});

test('Caliper requests preserve public arguments, transient data and identities', () => {
    const request = buildCaliperRequest({
        ...invocation('DispatchTransfer', 'LabMSP', 1), targetMspIds: ['LabMSP'],
    });
    assert.equal(request.contractFunction, 'DispatchTransfer');
    assert.equal(request.invokerIdentity, 'lab-user1');
    assert.deepEqual(JSON.parse(request.transientMap.destinatario), {destino: 'GLN:7791234500024'});
    assert.throws(() => buildCaliperRequest(invocation('VerifyUnit', 'LabMSP', 1)), /unsupported workload operation/u);
});

test('contract error extraction recognizes expected errors and the exact private-data retry marker', () => {
    const envelope = extractContractError(new Error(
        'endorsement failed: {"code":"INTERNAL_ERROR","message":"wait","details":{"reintentable":true,"causa":"PRIVATE_DATA_NOT_DISSEMINATED"}}',
    ));
    assert.equal(envelope.code, 'INTERNAL_ERROR');
    assert.equal(isPrivateDataNotDisseminated(envelope), true);
    assert.equal(isPrivateDataNotDisseminated({code: 'INTERNAL_ERROR', details: {reintentable: true}}), false);
});

test('measured receive retries stale receiver state only in the correlated transfer path', async () => {
    let invocations = 0;
    const gatewayPool = {
        invoke: async () => {
            invocations += 1;
            if (invocations === 1) {
                throw new Error('endorsement failed: {"code":"NOT_IN_TRANSIT","message":"receiver is stale"}');
            }
            return {transactionId: 'tx-receive', result: Buffer.alloc(0)};
        },
    };
    const sutAdapter = {emit: () => {}};
    const receive = await measuredGatewayInvocation({
        gatewayPool,
        sutAdapter,
        invocation: invocation('ReceiveTransfer', 'DrogueriaMSP', 1),
        retryTransientReceive: true,
        retryDelayMs: 0,
    });
    assert.equal(receive.status.GetStatus(), 'success');
    assert.equal(receive.retryCount, 1);
    assert.equal(receive.attempts[0].envelope.code, 'NOT_IN_TRANSIT');

    invocations = 0;
    const uncorrelated = await measuredGatewayInvocation({
        gatewayPool,
        sutAdapter,
        invocation: invocation('ReceiveTransfer', 'DrogueriaMSP', 1),
        retryDelayMs: 0,
    });
    assert.equal(uncorrelated.status.GetStatus(), 'failed');
    assert.equal(invocations, 1);
});

test('core operations record transfer pairs, retries and expected rejection codes', async () => {
    const calls = [];
    const sutAdapter = {
        sendRequests: async (request) => {
            calls.push(request.contractFunction);
            return status('success');
        },
    };
    const gatewayPool = {
        waitForUnitState: async () => {
            throw new Error('measured transfer must not wait for peer synchronization');
        },
    };
    const transfer = {
        workerIndex: 0, ordinal: 0, datasetSequence: 1, type: 'transfer',
        invocations: [
            invocation('DispatchTransfer', 'LabMSP', 1),
            {...invocation('ReceiveTransfer', 'DrogueriaMSP', 1), targetMspIds: ['LabMSP', 'DrogueriaMSP']},
        ],
    };
    const transferRecord = await executeOperation({
        operation: transfer,
        sutAdapter,
        gatewayPool,
        measuredInvocation: async (options) => {
            calls.push(options.invocation.operation);
            assert.equal(options.retryTransientReceive, true);
            return {
                status: status('success', 1020, 1030), envelope: undefined, retryCount: 1,
                attempts: [
                    {status: status('failed', 1010), envelope: {code: 'INTERNAL_ERROR'}},
                    {status: status('success', 1020, 1030), envelope: undefined},
                ],
            };
        },
    });
    assert.equal(transferRecord.outcome, 'success');
    assert.deepEqual(transferRecord.transactions.map((entry) => entry.function), [
        'DispatchTransfer', 'ReceiveTransfer', 'ReceiveTransfer',
    ]);
    assert.equal(transferRecord.transactions[2].retry, true);
    assert.deepEqual(calls, ['DispatchTransfer', 'ReceiveTransfer']);

    const rejection = {
        workerIndex: 0, ordinal: 0, datasetSequence: 2, type: 'register',
        invocations: [invocation('RegisterUnit', 'LabMSP', 2)],
        expectedRejection: {family: 'DUPLICATE_IDENTITY', code: 'UNIT_ALREADY_EXISTS'},
    };
    const rejectionRecord = await executeOperation({
        operation: rejection,
        sutAdapter,
        measuredInvocation: async () => ({
            status: status('failed'), envelope: {code: 'UNIT_ALREADY_EXISTS'},
            attempts: [{status: status('failed'), envelope: {code: 'UNIT_ALREADY_EXISTS'}}], retryCount: 0,
        }),
    });
    assert.equal(rejectionRecord.outcome, 'expected-rejection');
    assert.equal(rejectionRecord.observedErrorCode, 'UNIT_ALREADY_EXISTS');
});

test('processable results aggregate operations and build valid metadata fields', (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-caliper-results-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    appendOperation(path.join(directory, 'raw-worker-0.jsonl'), {
        workerIndex: 0, ordinal: 0, datasetSequence: 1, type: 'register', outcome: 'success',
        startedAt: '2026-10-02T00:00:00.000Z', endedAt: '2026-10-02T00:00:00.010Z', latencyMs: 10,
        transactions: [{function: 'RegisterUnit', latencyMs: 10, status: 'success', retry: false}],
    });
    const profile = {
        ...buildProfile({scenario: 'write-register', phase: 'measurement', repetition: '1', rate: '5'}),
        durationSeconds: 0.2,
    };
    const {summary} = aggregateResults(directory, profile, {
        startedAt: '2026-10-02T00:00:00.000Z',
        endedAt: '2026-10-02T00:00:00.400Z',
    });
    assert.equal(summary.transactionCount, 1);
    assert.equal(summary.operationLatency.p95Ms, 10);
    assert.equal(summary.observedDurationSeconds, 0.4);
    assert.equal(summary.rate.effectiveOperationsPerSecond, 2.5);
    assert.equal(summary.discardReason, undefined);
    const metadata = buildRoundMetadata({
        profile,
        repositoryCommit: 'a'.repeat(40),
        dataset: {seed: 20260727, sha256: 'b'.repeat(64), units: 50000},
        startedAt: '2026-10-02T00:00:00Z', endedAt: '2026-10-02T00:02:01Z',
        host: {cpu: 'CPU', cpuCores: 1, memoryGB: 1, os: 'OS', kernel: 'kernel'},
        environment: {
            docker: '1', dockerCompose: '1', contractVersion: '2.11.2', packageID: `snt_1.0:${'c'.repeat(64)}`,
            fabric: '2.5.16', fabricCA: '1.5.15', caliper: '0.7.1',
        },
    }, summary);
    assert.equal(metadata.rate.targetTransactionsPerSecond, 5);
    assert.equal(metadata.rate.effectiveTransactionsPerSecond, 2.5);
    assert.equal(metadata.environment.contractVersion, '2.11.2');
});

test('below-target throughput is preserved and rejection codes remain processable', (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-caliper-incomplete-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    appendOperation(path.join(directory, 'raw-worker-0.jsonl'), {
        workerIndex: 0, ordinal: 0, datasetSequence: 1, type: 'register', outcome: 'expected-rejection',
        observedErrorCode: 'UNIT_ALREADY_EXISTS',
        startedAt: '2026-10-02T00:00:00.000Z', endedAt: '2026-10-02T00:00:00.010Z', latencyMs: 10,
        transactions: [{function: 'RegisterUnit', latencyMs: 10, status: 'failed', retry: false}],
    });
    const profile = buildProfile({
        scenario: 'expected-rejections', phase: 'measurement', repetition: '1', rate: '5',
        family: 'DUPLICATE_IDENTITY',
    });
    const {summary} = aggregateResults(directory, profile);
    assert.equal(summary.discardReason, undefined);
    assert.deepEqual(summary.observedErrorCodes, {UNIT_ALREADY_EXISTS: 1});
});

test('processable results count transfer pairs and pair-level retries', (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-caliper-transfer-results-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    appendOperation(path.join(directory, 'raw-worker-0.jsonl'), {
        workerIndex: 0,
        ordinal: 0,
        datasetSequence: 1,
        type: 'transfer',
        outcome: 'success',
        startedAt: '2026-10-02T00:00:00.000Z',
        endedAt: '2026-10-02T00:00:00.400Z',
        latencyMs: 400,
        transactions: [
            {function: 'DispatchTransfer', latencyMs: 10, status: 'success', retry: false},
            {function: 'ReceiveTransfer', latencyMs: 10, status: 'failed', retry: false, errorCode: 'NOT_IN_TRANSIT'},
            {function: 'ReceiveTransfer', latencyMs: 10, status: 'success', retry: true},
        ],
    });
    const profile = {
        ...buildProfile({scenario: 'write-transfer', phase: 'measurement', repetition: '1', rate: '5'}),
        durationSeconds: 0.2,
    };
    const {summary} = aggregateResults(directory, profile);
    assert.equal(summary.transferPairs, 1);
    assert.equal(summary.retriedTransferPairs, 1);
    assert.equal(summary.retriedTransferPairRate, 1);
    assert.equal(summary.retryAttempts, 1);
    assert.equal(summary.observedDurationSeconds, 0.4);
    assert.equal(summary.rate.effectiveOperationsPerSecond, 2.5);
    assert.equal(summary.rate.effectiveTransactionsPerSecond, 7.5);
    assert.equal(summary.discardReason, undefined);
});
