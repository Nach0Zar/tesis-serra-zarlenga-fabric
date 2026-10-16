'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const {
    ORDERERS, PEERS, ROUND_SECONDS, SCENARIOS,
    buildAvailabilityPlan, classify, classifyOperations, eventLines, faultWindows, leaderFromLogs, orderersToStop,
    probeSequences, requirements, scenarioDefinition, schedule, RAFT_EVENT,
} = require('../src/availability');
const planner = require('../src/planner');
const {buildProfile} = require('../src/profiles');
const {availabilityProfile, availabilitySummary, parseArguments, probeInvocation} = require('../src/run-availability');
const {PLANS, availabilityScenarioMatrix, runSeries, scenarioArguments} = require('../src/run-series');
const {validateDocument} = require('../src/artifacts');
const {loadDatasetBundle} = require('../src/sources');

const repoRoot = path.resolve(__dirname, '..', '..', '..');

function invocation(operation, invokerMspId, targetMspIds) {
    return {operation, invokerMspId, targetMspIds, request: {gtin: '07791234567898', numeroSerie: 'SN-1'}};
}

test('the six scenarios of section 8.1 are defined, and only Raft-2 keeps the fault until the end', () => {
    assert.deepEqual(Object.keys(SCENARIOS), ['raft-1', 'raft-2', 'peer-1a', 'peer-1b', 'peer-1c', 'peer-1d']);
    assert.deepEqual(schedule(scenarioDefinition('raft-1')), {injectAfter: 60, recoverAfter: 120, durationSeconds: 180});
    assert.deepEqual(schedule(scenarioDefinition('raft-2')), {injectAfter: 60, recoverAfter: undefined, durationSeconds: 180});
    assert.equal(ROUND_SECONDS, 180);
    for (const name of ['peer-1a', 'peer-1b', 'peer-1c', 'peer-1d']) {
        assert.ok(PEERS[scenarioDefinition(name).mspId], name);
    }
    assert.throws(() => scenarioDefinition('db-1'), /unsupported availability scenario/u);
});

test('an operation is a target when the down peer is its gateway or one of its endorsers', () => {
    const transfer = [invocation('DispatchTransfer', 'LabMSP', ['LabMSP']),
        invocation('ReceiveTransfer', 'DistribuidorMSP', ['LabMSP', 'DistribuidorMSP'])];
    assert.deepEqual(requirements(transfer), {gateways: ['DistribuidorMSP', 'LabMSP'], endorsers: ['DistribuidorMSP', 'LabMSP']});
    assert.deepEqual(classify(transfer, 'DistribuidorMSP').availabilityRequirement.downPeerRequiredAs, ['gateway', 'endorsement']);
    assert.equal(classify(transfer, 'CentroMedicoMSP').availabilityClass, 'control');

    // Recepcion invocada por el receptor: el laboratorio caido sólo falta como endosante.
    const receive = [invocation('ReceiveTransfer', 'FarmaciaMSP', ['LabMSP', 'FarmaciaMSP'])];
    assert.deepEqual(classify(receive, 'LabMSP'), {
        availabilityClass: 'target',
        availabilityRequirement: {gateways: ['FarmaciaMSP'], endorsers: ['FarmaciaMSP', 'LabMSP'], downPeerRequiredAs: ['endorsement']},
    });

    const quarantine = [invocation('Quarantine', 'AnmatMSP', ['AnmatMSP', 'LabMSP'])];
    assert.equal(classify(quarantine, 'AnmatMSP').availabilityClass, 'target');
});

test('raw operations are classified from the work plan, including the regulatory probe', () => {
    const plan = {workers: [{workerIndex: 0, operations: [
        {ordinal: 0, invocations: [invocation('Dispense', 'CentroMedicoMSP', ['CentroMedicoMSP'])]},
        {ordinal: 1, invocations: [invocation('ReadUnit', 'LabMSP', ['LabMSP'])]},
    ]}]};
    const probe = new Map([['2:0', [invocation('Quarantine', 'AnmatMSP', ['AnmatMSP', 'LabMSP'])]]]);
    const classified = classifyOperations([
        {workerIndex: 0, ordinal: 0, outcome: 'unexpected-failure'},
        {workerIndex: 0, ordinal: 1, outcome: 'success'},
        {workerIndex: 2, ordinal: 0, outcome: 'success'},
    ], plan, 'CentroMedicoMSP', probe);
    assert.deepEqual(classified.map((operation) => operation.availabilityClass), ['target', 'control', 'control']);
    assert.throws(() => classifyOperations([{workerIndex: 1, ordinal: 0}], plan, 'LabMSP'), /not in the work plan/u);
});

test('the Raft leader comes from the latest leader change in the orderer logs', () => {
    const logs = {
        'orderer.anmat.snt.local': '\u001b[34m2026-10-15 13:24:53.172 UTC 00be INFO\u001b[0m run -> Raft leader changed: 0 -> 1 channel=snt-channel node=1',
        'orderer.drogueria.snt.local': [
            '2026-10-15 13:12:15.155 UTC 0050 INFO run -> Raft leader changed: 0 -> 2 channel=snt-channel node=3',
            '2026-10-15 13:30:01.000 UTC 0090 INFO run -> Raft leader changed: 1 -> 3 channel=snt-channel node=3',
            '2026-10-15 13:31:00.000 UTC 0091 INFO run -> Raft leader changed: 0 -> 2 channel=other-channel node=3',
        ].join('\n'),
    };
    assert.deepEqual(leaderFromLogs(logs), ORDERERS[2]);
    assert.equal(leaderFromLogs({x: '2026-10-15 13:24:53.172 UTC Raft leader changed: 1 -> 0 channel=snt-channel'}), undefined);

    assert.deepEqual(orderersToStop(ORDERERS[1], 1), [ORDERERS[1]]);
    assert.deepEqual(orderersToStop(ORDERERS[1], 2), [ORDERERS[1], ORDERERS[0]]);
    assert.throws(() => orderersToStop(undefined, 1), /could not be determined/u);
});

test('fault windows derive from the recorded instants, as the metadata validator requires', () => {
    const recovered = faultWindows({
        startedAt: '2026-10-16T10:00:00.000Z', endedAt: '2026-10-16T10:03:01.500Z',
        injectedAt: '2026-10-16T10:01:00.004Z', recoveredAt: '2026-10-16T10:02:00.010Z', durationSeconds: 180,
    });
    assert.equal(recovered.preFaultSeconds, 60.004);
    assert.equal(recovered.faultSeconds, 60.006);
    assert.ok(Math.abs(recovered.preFaultSeconds + recovered.faultSeconds + recovered.recoverySeconds - 180) < 1e-9);

    const quorumLoss = faultWindows({
        startedAt: '2026-10-16T10:00:00.000Z', endedAt: '2026-10-16T10:03:05.000Z',
        injectedAt: '2026-10-16T10:01:00.000Z', durationSeconds: 180,
    });
    assert.deepEqual(quorumLoss, {injectedAt: '2026-10-16T10:01:00.000Z', preFaultSeconds: 60, faultSeconds: 120});

    assert.throws(() => faultWindows({
        startedAt: '2026-10-16T10:00:00.000Z', endedAt: '2026-10-16T10:00:30.000Z',
        injectedAt: '2026-10-16T10:01:00.000Z', durationSeconds: 180,
    }), /outside the measured window/u);
});

test('log events keep only the lines that explain the fault inside the window', () => {
    const logs = [
        '2026-10-16T10:00:59.000000000Z \u001b[34mINFO\u001b[0m Raft leader changed: 1 -> 0 channel=snt-channel',
        '2026-10-16T10:01:01.000000000Z INFO raft.node: 3 elected leader 3 at term 12 channel=snt-channel',
        '2026-10-16T10:01:02.000000000Z INFO Writing block [10] to ledger',
        '2026-10-16T11:00:00.000000000Z INFO Raft leader changed: 3 -> 1 channel=snt-channel',
    ].join('\n');
    const lines = eventLines(logs, RAFT_EVENT, Date.parse('2026-10-16T10:00:00Z'), Date.parse('2026-10-16T10:05:00Z'));
    assert.equal(lines.length, 2);
    assert.ok(!lines[0].includes('\u001b'));
});

test('the regulatory probe uses filler units and needs ANMAT and the custodian laboratory', () => {
    assert.deepEqual(probeSequences([7, 8, 9, 10], 7, 2), [8, 9]);
    assert.throws(() => probeSequences([7, 8], 7, 2), /only 1 filler units/u);
    const quarantine = probeInvocation({preparation: [{
        operation: 'RegisterUnit', invokerMspId: 'LabMSP', request: {gtin: '07791234567898', numeroSerie: 'SN-9'},
    }]});
    assert.equal(quarantine.operation, 'Quarantine');
    assert.equal(quarantine.invokerMspId, 'AnmatMSP');
    assert.deepEqual(quarantine.targetMspIds, ['AnmatMSP', 'LabMSP']);
    assert.equal(quarantine.request.numeroSerie, 'SN-9');
});

const canonicalDatasetDirectory = process.env.SNT_CALIPER_DATASET_DIR
    ? path.resolve(process.env.SNT_CALIPER_DATASET_DIR)
    : path.join(repoRoot, 'build', 'dataset');

test('the extended plan keeps the mixed cycle and the first operations of the reference mixed load', {
    skip: !fs.existsSync(path.join(canonicalDatasetDirectory, 'dataset.json')),
}, () => {
    const {dataset} = loadDatasetBundle(canonicalDatasetDirectory);
    const profile = availabilityProfile({phase: 'measurement', repetition: '1'}, 180);
    const plan = buildAvailabilityPlan(dataset, profile, 20260727, {...planner, buildProfile});
    const mixed = planner.buildPlan(dataset, buildProfile({scenario: 'mixed', phase: 'measurement', repetition: '1', rate: '20'}), 20260727);
    for (const worker of mixed.workers) {
        const extended = plan.workers[worker.workerIndex].operations;
        assert.deepEqual(extended.slice(0, worker.operations.length), worker.operations, `worker ${worker.workerIndex}`);
        assert.equal(extended.length, planner.plannedCalls(profile));
        extended.forEach((operation, ordinal) => {
            const rotation = (20260727 + worker.workerIndex) % planner.MIX_PATTERN.length;
            assert.equal(operation.type, planner.MIX_PATTERN[(ordinal + rotation) % planner.MIX_PATTERN.length]);
        });
    }
    const sequences = plan.workers.flatMap((worker) => worker.operations.map((operation) => operation.datasetSequence));
    assert.equal(new Set(sequences).size, sequences.length, 'no unit is used twice');
    assert.deepEqual(plan.sources, {
        register: {fromMixed: 280, fromDedicated: 140, dedicatedScenario: 'write-register'},
        transfer: {fromMixed: 1540, fromDedicated: 770, dedicatedScenario: 'write-transfer'},
        dispense: {fromMixed: 280, fromDedicated: 140, dedicatedScenario: 'write-dispense'},
        'query-unit': {fromMixed: 700, fromDedicated: 350, dedicatedScenario: 'read-unit'},
    });
    assert.equal(plan.profile.durationSeconds, 180);
});

test('an availability round is discarded only when the load was already failing before the fault', () => {
    const windows = {injectedAt: '2026-10-16T10:01:00.000Z'};
    const operation = (startedAt, outcome, availabilityClass) => ({startedAt, outcome, availabilityClass});
    const definition = scenarioDefinition('peer-1b');
    const healthy = availabilitySummary({scenario: 'mixed'}, [
        operation('2026-10-16T10:00:10.000Z', 'success', 'control'),
        operation('2026-10-16T10:01:10.000Z', 'unexpected-failure', 'target'),
    ], windows, definition);
    assert.equal(healthy.discardReason, undefined);
    assert.equal(healthy.scenario, 'peer-1b');
    assert.deepEqual(healthy.availability.operationsByClass, {control: {operations: 1, successes: 1}, target: {operations: 1, successes: 0}});

    const unstable = availabilitySummary({}, [operation('2026-10-16T10:00:10.000Z', 'unexpected-failure', 'control')], windows, definition);
    assert.match(unstable.discardReason, /antes de inyectar la falla/u);
});

test('availability rounds run through the series orchestrator with their own plans', async () => {
    assert.deepEqual(availabilityScenarioMatrix().map((entry) => entry.key), Object.keys(SCENARIOS));
    assert.deepEqual(scenarioArguments({scenario: 'peer-1c', rate: 20}, 'measurement', 3),
        ['--scenario', 'peer-1c', '--phase', 'measurement', '--repetition', '3']);
    assert.deepEqual(PLANS.availability, {citable: true, repetitions: 5, extendByCV: false});
    assert.throws(() => parseArguments(['--scenario', 'raft-3', '--phase', 'measurement', '--repetition', '1', '--snapshot-dir', 'x']),
        /unsupported availability scenario/u);

    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'availability-series-'));
    const attempts = [];
    const result = await runSeries(repoRoot, {plan: 'availability-trial', snapshotDirectory: 'snapshot'}, {
        readSnapshot: () => ({
            directory: path.join(repoRoot, 'build', 'snapshot'),
            manifestPath: __filename,
            manifest: {snapshotId: 'a'.repeat(64)},
        }),
        createSeriesDirectory: () => ({directory, token: 'availability-trial'}),
        executeAttempt: async (_root, options) => {
            attempts.push(`${options.scenario.key}:${options.kind}:${options.repetition}`);
            return {status: 'completed', summary: {rate: {effectiveOperationsPerSecond: 1}, operationLatency: {p95Ms: 1}}};
        },
        now: () => '2026-10-16T10:00:00.000Z',
    });
    assert.equal(result.index.citable, false);
    assert.equal(result.index.status, 'completed');
    assert.equal(attempts.length, 12, 'warm-up and one measurement per scenario');
    assert.ok(result.index.scenarios.every((scenario) => scenario.throughputCV === undefined && !scenario.extended));
    validateDocument(repoRoot, 'fabric-series-index.schema.json', result.index);
    assert.throws(() => validateDocument(repoRoot, 'fabric-series-index.schema.json', {...result.index, citable: true}));
});

test('availability rounds bound the gRPC reconnect backoff without touching other options', () => {
    const {RECONNECT_GRPC_OPTIONS, withBoundedReconnect} = require('../src/availability');
    const config = {name: 'x', organizations: [{mspid: 'LabMSP', peers: [{endpoint: 'localhost:8051', grpcOptions: {'ssl-target-name-override': 'peer0.lab'}}]}]};
    const bounded = withBoundedReconnect(config);
    assert.deepEqual(bounded.organizations[0].peers[0].grpcOptions, {'ssl-target-name-override': 'peer0.lab', ...RECONNECT_GRPC_OPTIONS});
    assert.deepEqual(config.organizations[0].peers[0].grpcOptions, {'ssl-target-name-override': 'peer0.lab'});
    assert.ok(RECONNECT_GRPC_OPTIONS['grpc.max_reconnect_backoff_ms'] <= 1000);
});
