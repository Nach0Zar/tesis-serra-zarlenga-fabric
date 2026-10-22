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
    const operation = (startedAt, outcome, availabilityClass) => ({startedAt, endedAt: startedAt, outcome, availabilityClass});
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

    // En vuelo al inyectar la falla: empezo antes, fallo despues por la falla.
    const inFlight = availabilitySummary({}, [{
        startedAt: '2026-10-16T10:00:58.000Z', endedAt: '2026-10-16T10:01:01.200Z', outcome: 'unexpected-failure', availabilityClass: 'target',
    }], windows, definition);
    assert.equal(inFlight.discardReason, undefined);
    assert.equal(inFlight.availability.preFaultFailures, 0);
});

test('availability rounds run through the series orchestrator with their own plans', async () => {
    assert.deepEqual(availabilityScenarioMatrix().map((entry) => entry.key), Object.keys(SCENARIOS));
    assert.deepEqual(scenarioArguments({scenario: 'peer-1c', rate: 20}, 'measurement', 3),
        ['--scenario', 'peer-1c', '--phase', 'measurement', '--repetition', '3']);
    assert.deepEqual(PLANS.availability, {citable: true, repetitions: 5, extendByCV: true});
    assert.deepEqual(PLANS['availability-trial'], {citable: false, repetitions: 1, extendByCV: false});
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

test('the citable availability plan extends a scenario to eight repetitions when its CV exceeds 15 %', async () => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'availability-cv-'));
    const p95 = [1, 100, 1, 100, 1, 100, 1, 100];
    const attempts = [];
    const result = await runSeries(repoRoot, {plan: 'availability', snapshotDirectory: 'snapshot'}, {
        readSnapshot: () => ({
            directory: path.join(repoRoot, 'build', 'snapshot'), manifestPath: __filename,
            manifest: {snapshotId: 'c'.repeat(64)},
        }),
        createSeriesDirectory: () => ({directory, token: 'availability-cv'}),
        scenarioMatrix: [{key: 'raft-1', scenario: 'raft-1', rate: 20}],
        executeAttempt: async (_root, options) => {
            attempts.push(`${options.kind}:${options.repetition}`);
            const p95Ms = options.kind === 'warmup' ? 1 : p95[options.repetition - 1];
            return {status: 'completed', summary: {rate: {effectiveOperationsPerSecond: 20}, operationLatency: {p95Ms}}};
        },
        now: () => '2026-10-20T10:00:00.000Z',
    });
    const [scenario] = result.index.scenarios;
    assert.equal(result.index.citable, true);
    assert.equal(scenario.extended, true);
    assert.ok(scenario.p95CV > 0.15);
    assert.deepEqual(attempts, ['warmup:0', 'measurement:1', 'measurement:2', 'measurement:3', 'measurement:4',
        'measurement:5', 'measurement:6', 'measurement:7', 'measurement:8']);
    validateDocument(repoRoot, 'fabric-series-index.schema.json', result.index);
});

test('the availability summary counts every observed attempt, probe included, with a per-source breakdown', () => {
    const {summarizeOperations} = require('../src/raw-results');
    const {sourceStatistics} = require('../src/run-availability');
    const profile = availabilityProfile({phase: 'measurement', repetition: '1'}, 180);
    const transaction = (functionName, status) => ({function: functionName, status, latencyMs: 10, retry: false});
    const mixed = {workerIndex: 0, ordinal: 0, type: 'query-unit', outcome: 'success', latencyMs: 10,
        startedAt: '2026-10-20T10:00:00.000Z', endedAt: '2026-10-20T10:00:00.010Z', transactions: [transaction('ReadUnit', 'success')]};
    const probe = {workerIndex: 2, ordinal: 0, type: 'regulatory-event', outcome: 'success', latencyMs: 10,
        startedAt: '2026-10-20T10:00:01.000Z', endedAt: '2026-10-20T10:00:01.010Z', transactions: [transaction('Quarantine', 'success')]};
    const window = {startedAt: '2026-10-20T10:00:00.000Z', endedAt: '2026-10-20T10:03:00.000Z'};
    const complete = summarizeOperations([mixed, probe], profile, window);
    const summary = availabilitySummary(complete, [mixed, probe], {injectedAt: '2026-10-20T10:01:00.000Z'}, scenarioDefinition('peer-1c'));
    assert.equal(summary.operationCount, 2);
    assert.equal(summary.transactionCount, 2);
    assert.equal(summary.successfulOperations, 2);
    assert.equal(summary.rate.effectiveTransactionsPerSecond, 2 / 180);
    assert.equal(summary.rate.targetOperationsPerSecond, 20, 'the reference conceptual rate does not change');
    assert.deepEqual(sourceStatistics([probe], 180), {
        operations: 1, successfulOperations: 1, failedOperations: 0, transactions: 1, successfulTransactions: 1,
        effectiveOperationsPerSecond: 1 / 180, effectiveTransactionsPerSecond: 1 / 180,
    });
});

test('a component that does not come back is recorded as a failed restoration instead of aborting', async () => {
    const {restoreNetwork, restorationEvidence} = require('../src/run-availability');
    const unhealthy = () => {
        throw new Error('docker compose up failed: Container orderer.anmat.snt.local  Started\ncontainer orderer.anmat.snt.local is unhealthy');
    };
    const timeouts = [];
    const leader = ORDERERS[1];
    const faultEvents = {};
    const restoration = await restoreNetwork(repoRoot, scenarioDefinition('raft-1'), faultEvents, {
        ensureNetworkRunning: unhealthy,
        electedLeader: async (timeoutMs) => {
            timeouts.push(timeoutMs);
            return leader;
        },
    });
    assert.equal(restoration.status, 'failed');
    assert.equal(restoration.reason, 'container orderer.anmat.snt.local is unhealthy');
    assert.deepEqual(faultEvents.restoration, restoration);
    assert.deepEqual(faultEvents.leaderAfter, leader);
    assert.deepEqual(timeouts, [10_000], 'a failed restoration does not wait the full election timeout');

    const healthy = await restoreNetwork(repoRoot, scenarioDefinition('peer-1a'), {}, {
        ensureNetworkRunning: () => {},
        electedLeader: async () => assert.fail('peer scenarios do not look for a leader'),
    });
    assert.equal(healthy.status, 'restored');

    const logs = [
        '2026-10-21T01:56:50.000Z INFO starting',
        '2026-10-21T01:56:53.009Z 2026-10-21 01:56:53.009 UTC 0010 \u001b[35mPANI\u001b[0m failed to read WAL and cannot repair',
        '2026-10-21T01:56:53.009Z panic: failed to read WAL and cannot repair',
    ].join('\n');
    const evidence = restorationEvidence(['orderer.anmat.snt.local'], '2026-10-21T01:56:49.689Z', (container, {since}) => {
        assert.equal(since, '2026-10-21T01:56:49.689Z');
        return logs;
    });
    assert.equal(evidence.length, 2);
    assert.match(evidence[0], /^orderer\.anmat\.snt\.local: .*PANI failed to read WAL/u);
    assert.match(evidence[1], /panic: failed to read WAL/u);
});

test('reconstruction reads the fault instants, the abort reason and per-container logs a failed round kept', () => {
    const {abortReason, composeLogsByContainer, faultActions} = require('../src/reconstruct-availability-round');
    const roundLog = [
        'Discovery ready: {}',
        'Fault injected: docker kill orderer.anmat.snt.local at 2026-10-21T01:55:49.687Z',
        'Fault recovered: docker start orderer.anmat.snt.local at 2026-10-21T01:56:49.689Z',
        'Benchmark successfully finished!',
        'Fabric availability round failed: docker compose up failed: Container orderer.anmat.snt.local  Started',
        'container orderer.anmat.snt.local is unhealthy',
    ].join('\n');
    assert.deepEqual(faultActions(roundLog), [
        {action: 'kill', containers: ['orderer.anmat.snt.local'], issuedAt: '2026-10-21T01:55:49.687Z'},
        {action: 'start', containers: ['orderer.anmat.snt.local'], issuedAt: '2026-10-21T01:56:49.689Z'},
    ]);
    assert.equal(abortReason(roundLog), 'container orderer.anmat.snt.local is unhealthy');
    assert.throws(() => faultActions('Discovery ready: {}'), /does not record the fault injection/u);
    assert.throws(() => abortReason('Fabric availability round completed: x'), /does not record a failed round/u);

    const sutLog = [
        'orderer.anmat.snt.local       | \u001b[34m2026-10-21 01:56:53.009 UTC 0010 PANI\u001b[0m [multichannel] failed to read WAL',
        'orderer.anmat.snt.local       | panic: failed to read WAL and cannot repair',
        'orderer.lab.snt.local         | 2026-10-21 01:55:55.100 UTC 0042 INFO Raft leader changed: 0 -> 2 channel=snt-channel',
        'peer0.lab.snt.local           | 2026-10-21 01:59:00.000 UTC 0001 INFO outside the window',
    ].join('\n');
    const logsFor = composeLogsByContainer(sutLog);
    assert.deepEqual(logsFor('orderer.anmat.snt.local').split('\n'), [
        '2026-10-21T01:56:53.009Z 2026-10-21 01:56:53.009 UTC 0010 PANI [multichannel] failed to read WAL',
        '2026-10-21T01:56:53.009Z panic: failed to read WAL and cannot repair',
    ], 'lines without their own timestamp inherit the previous one');
    assert.equal(logsFor('peer0.lab.snt.local', {until: '2026-10-21T01:58:00.000Z'}), '');
    assert.equal(eventLines(logsFor('orderer.lab.snt.local'), RAFT_EVENT, Date.parse('2026-10-21T01:55:00Z'),
        Date.parse('2026-10-21T01:56:00Z')).length, 1);
});

test('series attempts point at the resources file each executor actually wrote', () => {
    const {attemptRecord} = require('../src/run-series');
    const series = path.join(os.tmpdir(), 'series');
    const attempt = path.join(series, 'db-1', 'measurements', 'run-01');
    const base = ['measurement', 1, 1, attempt, '2026-10-21T10:00:00.000Z', '2026-10-21T10:03:00.000Z'];
    assert.equal(attemptRecord(series, ...base, {status: 'completed'}).resources, 'db-1/measurements/run-01/resources.txt');
    assert.equal(attemptRecord(series, ...base, {status: 'completed', resourcesPath: path.join(attempt, 'round', 'run-context.json')}).resources,
        'db-1/measurements/run-01/round/run-context.json');
});
