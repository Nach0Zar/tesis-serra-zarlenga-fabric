'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const grpc = require('@grpc/grpc-js');
const protos = require('@hyperledger/fabric-protos');

const {
    SNAPSHOT_SCHEMA_ID, canonicalJSON, fileDescriptor, readSnapshot, sha256, sha256File, validateDocument,
} = require('../src/artifacts');
const {candidateKey, indexCandidatePools} = require('../src/planner');
const {parseArguments: parseRoundArguments} = require('../src/run-round');
const {createBuildTimer, snapshotIdentifier} = require('../src/snapshot');
const {
    coefficientOfVariation, fullScenarioMatrix, runSeries, trialScenarioMatrix,
} = require('../src/run-series');
const {smokeRegistration, verifyExistingUnit} = require('../src/run-smoke');
const {
    EXPECTED_COUNTS, buildGoldenSnapshotPlan, profileMatrix,
} = require('../src/snapshot-plan');
const {
    PreparationJournal, journalStatistics, prepareRecipe, progressReaderMspId,
} = require('../src/snapshot-state');
const {
    CHAINCODE_CONTAINER, VOLUMES, assertVolumeManifest, treeDigest,
} = require('../src/snapshot-volumes');
const {decodeTransactionRWSet, summarizeCollections, summarizePrivateWrites} = require('../src/rwset');
const {loadDatasetBundle} = require('../src/sources');
const {warmConfiguredPeers} = require('../src/warm-peers');

const repoRoot = path.resolve(__dirname, '..', '..', '..');

function registration(sequence = 1) {
    return {
        operation: 'RegisterUnit',
        invokerMspId: 'LabMSP',
        request: {
            gtin: '07790202607277', numeroSerie: `SN${String(sequence).padStart(16, '0')}`,
            lote: `L${sequence}`, fechaVencimiento: '2100-07-26',
        },
    };
}

function unit(sequence = 1) {
    return {sequence, preparation: [registration(sequence)]};
}

function collectionRWSet(name, writes) {
    const hashed = new protos.ledger.rwset.kvrwset.HashedRWSet();
    for (let index = 0; index < writes; index += 1) {
        const write = new protos.ledger.rwset.kvrwset.KVWriteHash();
        write.setKeyHash(Uint8Array.from([index + 1]));
        write.setValueHash(Uint8Array.from([index + 2]));
        hashed.addHashedWrites(write);
    }
    const collection = new protos.ledger.rwset.CollectionHashedReadWriteSet();
    collection.setCollectionName(name);
    collection.setHashedRwset(hashed.serializeBinary());
    return collection;
}

function privateWriteEnvelope() {
    const namespace = new protos.ledger.rwset.NsReadWriteSet();
    namespace.setNamespace('snt');
    namespace.addCollectionHashedRwset(collectionRWSet('_implicit_org_LabMSP', 1));
    namespace.addCollectionHashedRwset(collectionRWSet('transfer_DrogueriaMSP_LabMSP', 2));
    const transactionRWSet = new protos.ledger.rwset.TxReadWriteSet();
    transactionRWSet.addNsRwset(namespace);
    const chaincodeAction = new protos.peer.ChaincodeAction();
    chaincodeAction.setResults(transactionRWSet.serializeBinary());
    const response = new protos.peer.ProposalResponsePayload();
    response.setExtension$(chaincodeAction.serializeBinary());
    const endorsed = new protos.peer.ChaincodeEndorsedAction();
    endorsed.setProposalResponsePayload(response.serializeBinary());
    const actionPayload = new protos.peer.ChaincodeActionPayload();
    actionPayload.setAction(endorsed);
    const transactionAction = new protos.peer.TransactionAction();
    transactionAction.setPayload(actionPayload.serializeBinary());
    const transaction = new protos.peer.Transaction();
    transaction.addActions(transactionAction);
    const payload = new protos.common.Payload();
    payload.setData(transaction.serializeBinary());
    const envelope = new protos.common.Envelope();
    envelope.setPayload(payload.serializeBinary());
    const prepared = new protos.gateway.PreparedTransaction();
    prepared.setTransactionId('tx-fixture');
    prepared.setEnvelope(envelope);
    return Buffer.from(prepared.serializeBinary());
}

function snapshotFixture(directory) {
    const fileNames = {
        logicalPlan: 'logical-plan.json',
        preconditions: 'preconditions.json',
        semanticVerification: 'semantic-verification.json',
        preparationMetadata: 'metadata.json',
        journal: 'preparation-journal.jsonl',
    };
    const files = {};
    for (const [key, name] of Object.entries(fileNames)) {
        const filePath = path.join(directory, name);
        fs.writeFileSync(filePath, '{}');
        files[key] = fileDescriptor(filePath, directory);
    }
    const volumesDirectory = path.join(directory, 'volumes');
    fs.mkdirSync(volumesDirectory);
    const volumes = VOLUMES.map((volume) => {
        const archive = `volumes/${volume.name}.tar`;
        const archivePath = path.join(directory, archive);
        fs.writeFileSync(archivePath, volume.name);
        return {
            ...volume,
            archive,
            sha256: sha256File(archivePath),
            treeSHA256: sha256(volume.name),
            bytes: fs.statSync(archivePath).size,
            fileCount: 1,
        };
    });
    const digest = '0'.repeat(64);
    const manifest = {
        $schema: SNAPSHOT_SCHEMA_ID,
        schemaVersion: '1.0.0',
        snapshotId: '1'.repeat(64),
        createdAt: '2026-10-09T00:00:00.000Z',
        dataset: {seed: 20260727, sha256: digest, manifestSHA256: digest, units: 50_000},
        counts: EXPECTED_COUNTS,
        preparation: {
            durationSeconds: 1,
            successfulWriteTransactions: 46_920,
            concurrency: 64,
            markers: {expected: 46_920, observed: 46_920, fromRegistrations: 46_920, fromRegulatoryEvents: 0},
        },
        artifacts: {
            repositoryCommit: '2'.repeat(40),
            contractVersion: '1.0.0',
            packageID: `snt_1.0:${'3'.repeat(64)}`,
            fabric: '2.5.16', fabricCA: '1.5.17', docker: '29.0.0', dockerCompose: '5.0.0',
            composeProject: 'snt-fabric', networkConfigSHA256: digest, sourceSHA256: digest,
        },
        smokeSequence: 1,
        files,
        volumes,
    };
    fs.writeFileSync(path.join(directory, 'manifest.json'), JSON.stringify(manifest));
    return manifest;
}

function fakeLedger() {
    let present = false;
    let current;
    const history = [];
    const notFound = () => new Error('query failed: {"code":"UNIT_NOT_FOUND","message":"missing"}');
    return {
        gateway: {
            readUnit: async () => {
                if (!present) throw notFound();
                return current;
            },
            getUnitHistory: async () => {
                if (!present) throw notFound();
                return history;
            },
        },
        commit(transactionId) {
            present = true;
            current = {
                gtin: registration().request.gtin,
                numeroSerie: registration().request.numeroSerie,
                lote: 'L1', fechaVencimiento: '2100-07-26',
                custodioActual: 'GLN:7791234500017', estado: 'EN_LABORATORIO',
            };
            history.push({txId: transactionId, value: current});
        },
        history,
    };
}

test('profile matrix and canonical constants encode the consolidated golden decision', () => {
    assert.equal(profileMatrix().length, 20);
    assert.deepEqual(EXPECTED_COUNTS, {
        total: 50_000, participating: 25_183, unregistered: 3_080,
        prepared: 22_103, filler: 24_817, registered: 46_920,
    });
});

const canonicalDatasetDirectory = process.env.SNT_CALIPER_DATASET_DIR
    ? path.resolve(process.env.SNT_CALIPER_DATASET_DIR)
    : path.join(repoRoot, 'build', 'dataset');
test('canonical dataset produces the exact golden partition and reserved filler smoke', {
    skip: !fs.existsSync(path.join(canonicalDatasetDirectory, 'dataset.json')),
}, () => {
    const bundle = loadDatasetBundle(canonicalDatasetDirectory);
    const plan = buildGoldenSnapshotPlan(bundle.dataset, bundle.manifest.seed);
    assert.deepEqual(plan.counts, EXPECTED_COUNTS);
    assert.equal(plan.units.length, 50_000);
    assert.equal(plan.units.filter((entry) => entry.expected.present).length, 46_920);
    assert.equal(plan.units.filter((entry) => !entry.expected.present).length, 3_080);
    assert.equal(plan.units.filter((entry) => entry.role === 'prepared').length, 22_103);
    assert.equal(plan.units.filter((entry) => entry.role === 'filler').length, 24_817);
    assert.equal(plan.fillerSequences.includes(plan.smokeSequence), true);
    assert.equal(plan.excludedSequences.includes(plan.smokeSequence), false);
    assert.equal(plan.preparedSequences.includes(plan.smokeSequence), false);
    assert.equal(plan.expectedMarkers.registrations, 46_920);
    const smoke = plan.units.find((entry) => entry.datasetSequence === plan.smokeSequence);
    assert.deepEqual(smoke.expected, {
        present: true, state: 'EN_LABORATORIO', custodian: 'GLN:7791234500017',
    });
    assert.deepEqual(smoke.private, {
        participationMarkers: 1, dispatches: 0, closedTransfers: 0, activeTransfer: false,
    });
});

test('planner candidate indexes preserve rejection variants without linear scans', () => {
    const first = {datasetSequence: 1, invocations: [{operation: 'RegisterUnit'}]};
    const rejection = {
        datasetSequence: 1,
        invocations: [{operation: 'DispatchTransfer'}],
        expectedRejection: {family: 'UNAUTHORIZED_TRANSFER'},
    };
    const indexes = indexCandidatePools({register: [first], rejection: [rejection]});
    assert.equal(indexes.register.get(candidateKey(first)), first);
    assert.equal(indexes.rejection.get(candidateKey(rejection)), rejection);
});

test('snapshot progress reads are distributed deterministically across configured peers', () => {
    const gatewayPool = {
        organizations: new Map([
            ['LabMSP', {}],
            ['AnmatMSP', {}],
            ['FarmaciaMSP', {}],
        ]),
    };
    assert.equal(progressReaderMspId(gatewayPool, unit(1)), 'AnmatMSP');
    assert.equal(progressReaderMspId(gatewayPool, unit(2)), 'FarmaciaMSP');
    assert.equal(progressReaderMspId(gatewayPool, unit(3)), 'LabMSP');
    assert.equal(progressReaderMspId(gatewayPool, unit(4)), 'AnmatMSP');
    assert.equal(progressReaderMspId({}, unit(1)), 'LabMSP');
});

test('ambiguous transient commit is reconciled from ledger history before retry', async (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-journal-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    const journal = new PreparationJournal(path.join(directory, 'journal.jsonl'));
    const ledger = fakeLedger();
    let calls = 0;
    await prepareRecipe({
        gatewayPool: ledger.gateway,
        journal,
        unit: unit(),
        recipe: {datasetSequence: 1, invocations: [registration()]},
        retryDelayMs: 0,
        invoke: async (_pool, _invocation, hooks) => {
            calls += 1;
            await hooks.onTransactionId('tx-ambiguous');
            await hooks.onEndorsed({transactionId: 'tx-ambiguous', transactionBytes: Buffer.from('endorsed')});
            ledger.commit('tx-ambiguous');
            const error = new Error('DEADLINE_EXCEEDED after submit');
            error.code = grpc.status.DEADLINE_EXCEEDED;
            error.transactionId = 'tx-ambiguous';
            throw error;
        },
        summarize: () => ({collections: [], markerWrites: 1, pairPrivateWrites: 0}),
    });
    assert.equal(calls, 1);
    assert.equal(ledger.history.length, 1);
    assert.equal(journal.committed()[0].status, 'recovered');
});

test('transient failure without commit retries and records only the confirmed transaction', async (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-journal-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    const journal = new PreparationJournal(path.join(directory, 'journal.jsonl'));
    const ledger = fakeLedger();
    let calls = 0;
    await prepareRecipe({
        gatewayPool: ledger.gateway,
        journal,
        unit: unit(),
        recipe: {datasetSequence: 1, invocations: [registration()]},
        retryDelayMs: 0,
        sleep: async () => {},
        invoke: async (_pool, _invocation, hooks) => {
            calls += 1;
            const transactionId = `tx-${calls}`;
            await hooks.onTransactionId(transactionId);
            if (calls === 1) {
                const error = new Error('UNAVAILABLE before commit');
                error.code = grpc.status.UNAVAILABLE;
                error.transactionId = transactionId;
                throw error;
            }
            await hooks.onEndorsed({transactionId, transactionBytes: Buffer.from('endorsed')});
            ledger.commit(transactionId);
            return {transactionId, transactionBytes: Buffer.from('endorsed'), result: Buffer.from('{}')};
        },
        summarize: () => ({collections: [], markerWrites: 1, pairPrivateWrites: 0}),
    });
    assert.equal(calls, 2);
    assert.equal(journal.committed().length, 1);
    assert.equal(journal.committed()[0].transactionId, 'tx-2');
});

test('successful commit waits for the reader peer to observe the new history entry', async (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-journal-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    const journal = new PreparationJournal(path.join(directory, 'journal.jsonl'));
    const ledger = fakeLedger();
    let hiddenCommittedReads = 2;
    const gatewayPool = {
        ...ledger.gateway,
        getUnitHistory: async (...arguments_) => {
            const history = await ledger.gateway.getUnitHistory(...arguments_);
            if (history.length > 0 && hiddenCommittedReads > 0) {
                hiddenCommittedReads -= 1;
                return [];
            }
            return history;
        },
    };
    await prepareRecipe({
        gatewayPool,
        journal,
        unit: unit(),
        recipe: {datasetSequence: 1, invocations: [registration()]},
        visibilityTimeoutMs: 1_000,
        visibilityPollMs: 0,
        sleep: async () => {},
        invoke: async (_pool, _invocation, hooks) => {
            await hooks.onTransactionId('tx-visible-late');
            await hooks.onEndorsed({transactionId: 'tx-visible-late', transactionBytes: Buffer.from('endorsed')});
            ledger.commit('tx-visible-late');
            return {
                transactionId: 'tx-visible-late',
                transactionBytes: Buffer.from('endorsed'),
                result: Buffer.from('{}'),
            };
        },
        summarize: () => ({collections: [], markerWrites: 1, pairPrivateWrites: 0}),
    });
    assert.equal(hiddenCommittedReads, 0);
    assert.equal(journal.committed()[0].transactionId, 'tx-visible-late');
});

test('valid commit result avoids redundant public-state and history evaluations', async (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-journal-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    const journal = new PreparationJournal(path.join(directory, 'journal.jsonl'));
    const ledger = fakeLedger();
    let historyReads = 0;
    const gatewayPool = {
        ...ledger.gateway,
        getUnitHistory: async (...arguments_) => {
            historyReads += 1;
            return ledger.gateway.getUnitHistory(...arguments_);
        },
    };
    await prepareRecipe({
        gatewayPool,
        journal,
        unit: unit(),
        recipe: {datasetSequence: 1, invocations: [registration()]},
        invoke: async (_pool, _invocation, hooks) => {
            await hooks.onTransactionId('tx-result');
            await hooks.onEndorsed({transactionId: 'tx-result', transactionBytes: Buffer.from('endorsed')});
            ledger.commit('tx-result');
            const current = await ledger.gateway.readUnit('LabMSP', registration().request);
            return {
                transactionId: 'tx-result',
                transactionBytes: Buffer.from('endorsed'),
                result: Buffer.from(JSON.stringify(current)),
            };
        },
        summarize: () => ({collections: [], markerWrites: 1, pairPrivateWrites: 0}),
    });
    assert.equal(historyReads, 0);
    assert.equal(journal.committed()[0].transactionId, 'tx-result');
});

test('resume aborts before submitting when ledger history has an unknown prefix', async (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-journal-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    const journal = new PreparationJournal(path.join(directory, 'journal.jsonl'));
    const ledger = fakeLedger();
    ledger.commit('unexpected-tx');
    let submitted = false;
    await assert.rejects(() => prepareRecipe({
        gatewayPool: ledger.gateway,
        journal,
        unit: unit(),
        recipe: {datasetSequence: 1, invocations: [registration()]},
        invoke: async () => {
            submitted = true;
        },
    }), /unrecognized committed transaction/u);
    assert.equal(submitted, false);
});

test('journal statistics deduplicate transactions and separate marker origins', (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-journal-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    const journal = new PreparationJournal(path.join(directory, 'journal.jsonl'));
    journal.append({datasetSequence: 1, step: 0, operation: 'RegisterUnit', status: 'committed', transactionId: 'a', privateWrites: {markerWrites: 1, pairPrivateWrites: 0}});
    journal.append({datasetSequence: 1, step: 0, operation: 'RegisterUnit', status: 'recovered', transactionId: 'a', privateWrites: {markerWrites: 1, pairPrivateWrites: 0}});
    journal.append({datasetSequence: 2, step: 1, operation: 'Quarantine', status: 'committed', transactionId: 'b', privateWrites: {markerWrites: 1, pairPrivateWrites: 1}});
    assert.deepEqual(journalStatistics(journal), {
        successfulWriteTransactions: 2,
        markers: {observed: 2, fromRegistrations: 1, fromRegulatoryEvents: 1},
        pairPrivateWrites: 1,
    });
});

test('durable journal group commit persists every concurrent event before resolving', async (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-journal-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    const journalPath = path.join(directory, 'journal.jsonl');
    const journal = new PreparationJournal(journalPath);
    await Promise.all(Array.from({length: 64}, (_, index) => journal.appendDurable({
        datasetSequence: index + 1,
        step: 0,
        operation: 'RegisterUnit',
        status: 'pending',
        transactionId: `tx-${index + 1}`,
    })));
    const reloaded = new PreparationJournal(journalPath);
    assert.equal(reloaded.events.length, 64);
    assert.deepEqual(
        reloaded.events.map((event) => event.transactionId),
        Array.from({length: 64}, (_, index) => `tx-${index + 1}`),
    );
});

test('private RW-set summaries distinguish implicit markers from pair collections', () => {
    assert.deepEqual(summarizeCollections([
        {collection: '_implicit_org_LabMSP', hashedWrites: 1},
        {collection: 'transfer_DrogueriaMSP_LabMSP', hashedWrites: 2},
    ]), {
        collections: [
            {collection: '_implicit_org_LabMSP', hashedWrites: 1},
            {collection: 'transfer_DrogueriaMSP_LabMSP', hashedWrites: 2},
        ],
        markerWrites: 1,
        pairPrivateWrites: 2,
    });
});

test('Fabric transaction envelopes decode private marker and pair RW sets', () => {
    const encoded = privateWriteEnvelope();
    assert.deepEqual(decodeTransactionRWSet(encoded), [
        {
            namespace: 'snt', collection: '_implicit_org_LabMSP',
            hashedReads: 0, hashedWrites: 1, metadataWrites: 0,
        },
        {
            namespace: 'snt', collection: 'transfer_DrogueriaMSP_LabMSP',
            hashedReads: 0, hashedWrites: 2, metadataWrites: 0,
        },
    ]);
    assert.equal(summarizePrivateWrites(encoded).markerWrites, 1);
    assert.equal(summarizePrivateWrites(encoded).pairPrivateWrites, 2);
});

test('snapshot manifest is strict and artifact corruption is rejected', (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-snapshot-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    const manifest = snapshotFixture(directory);
    assert.doesNotThrow(() => readSnapshot(repoRoot, directory));
    assert.throws(() => validateDocument(repoRoot, 'fabric-snapshot-manifest.schema.json', {
        ...manifest, unexpected: true,
    }), /additional propert/u);
    fs.writeFileSync(path.join(directory, manifest.files.logicalPlan.name), 'tampered');
    assert.throws(() => readSnapshot(repoRoot, directory), /artifact size mismatch/u);
});

test('snapshot ID excludes timestamps and preparation duration', () => {
    const descriptor = {name: 'file.json', sha256: '4'.repeat(64), bytes: 1};
    const binding = {
        dataset: {seed: 20260727, sha256: '0'.repeat(64), manifestSHA256: '1'.repeat(64), units: 50_000},
        counts: EXPECTED_COUNTS,
        artifacts: {repositoryCommit: '2'.repeat(40), sourceSHA256: '3'.repeat(64)},
        smokeSequence: 12,
        files: {logicalPlan: descriptor, preconditions: descriptor, semanticVerification: descriptor},
        volumes: [{name: 'volume', sha256: '5'.repeat(64)}],
        preparation: {durationSeconds: 1},
        createdAt: '2026-10-09T00:00:00.000Z',
    };
    const first = snapshotIdentifier(binding);
    const second = snapshotIdentifier({
        ...binding,
        preparation: {durationSeconds: 999},
        createdAt: '2027-01-01T00:00:00.000Z',
    });
    assert.equal(first, second);
});

test('snapshot build timer checkpoints active duration across interrupted sessions', (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-build-timer-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    const buildStatePath = path.join(directory, 'build-state.json');
    const state = {activeDurationMs: 200, startedAt: '2026-10-09T00:00:00.000Z'};
    let current = 1_000;
    const timer = createBuildTimer(state, buildStatePath, () => current);

    current = 1_500;
    assert.equal(timer.elapsedMs(), 700);
    assert.equal(timer.checkpoint(), 700);
    assert.deepEqual(JSON.parse(fs.readFileSync(buildStatePath, 'utf8')), state);

    current = 1_750;
    assert.equal(timer.elapsedMs(), 950);
    assert.equal(timer.checkpoint(), 950);
});

test('volume allowlist is exact and tree digests detect byte changes', (t) => {
    assert.doesNotThrow(() => assertVolumeManifest(VOLUMES));
    assert.throws(() => assertVolumeManifest([...VOLUMES.slice(1), {...VOLUMES[0], name: 'other'}]), /unexpected volume/u);
    assert.equal(CHAINCODE_CONTAINER.test('dev-peer0.lab.snt.local-snt_1.0-abcd'), true);
    assert.equal(CHAINCODE_CONTAINER.test('postgres'), false);
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-tree-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    fs.writeFileSync(path.join(directory, 'state'), 'one');
    const first = treeDigest(directory);
    fs.writeFileSync(path.join(directory, 'state'), 'two');
    assert.notEqual(treeDigest(directory).sha256, first.sha256);
});

test('smoke requires a registered filler and executes only read-unit', () => {
    const selected = smokeRegistration({units: [unit(3)]}, 3);
    const calls = [];
    const result = verifyExistingUnit('/repo', selected, (command, args) => {
        calls.push([command, args]);
        return {status: 0, stdout: '{}', stderr: ''};
    });
    assert.equal(result, 'read-only-existing');
    assert.equal(calls.length, 1);
    assert.equal(calls[0][1].includes('read-unit'), true);
    assert.equal(calls[0][1].includes('register-unit'), false);
});

test('seven-peer warm-up reads every configured peer and fails on divergence', async () => {
    const credentials = Array.from({length: 7}, (_, index) => ({mspId: `Org${index}MSP`, peerHostname: `peer${index}`}));
    const expected = {
        gtin: 'g', numeroSerie: 's', lote: 'l', fechaVencimiento: 'd', custodioActual: 'c', estado: 'e',
    };
    const calls = [];
    const warmed = await warmConfiguredPeers({readUnit: async (mspId) => {
        calls.push(mspId);
        return expected;
    }}, credentials, {gtin: 'g', numeroSerie: 's'}, expected);
    assert.equal(warmed.length, 7);
    assert.deepEqual(calls, credentials.map((entry) => entry.mspId));
    await assert.rejects(() => warmConfiguredPeers({readUnit: async (mspId) => (
        mspId === 'Org3MSP' ? {...expected, estado: 'different'} : expected
    )}, credentials, {gtin: 'g', numeroSerie: 's'}, expected), /different smoke unit state/u);
});

test('round arguments preserve prepare mode and require a snapshot for snapshot mode', () => {
    const base = ['--scenario', 'read-unit', '--phase', 'measurement', '--repetition', '1', '--rate', '50'];
    assert.equal(parseRoundArguments(base).stateMode, 'prepare');
    assert.throws(() => parseRoundArguments([...base, '--state-mode', 'snapshot']), /snapshot-dir/u);
    const snapshot = parseRoundArguments([...base, '--state-mode', 'snapshot', '--snapshot-dir', 'snapshot']);
    assert.equal(snapshot.stateMode, 'snapshot');
});

test('series matrices, population CV and warm-up blocking follow the protocol', async (t) => {
    assert.equal(fullScenarioMatrix().length, 20);
    assert.deepEqual(trialScenarioMatrix().map((entry) => entry.key), [
        'read-unit-50', 'write-transfer-20', 'mixed-20',
    ]);
    assert.equal(coefficientOfVariation([10, 10]), 0);
    assert.equal(Number(coefficientOfVariation([10, 20]).toFixed(6)), 0.333333);

    const directory = fs.mkdtempSync(path.join(repoRoot, 'build-series-test-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    const manifestPath = path.join(directory, 'manifest.json');
    fs.writeFileSync(manifestPath, '{}');
    const seriesDirectory = path.join(directory, 'series');
    fs.mkdirSync(seriesDirectory);
    const result = await runSeries(repoRoot, {
        snapshotDirectory: directory, plan: 'trial', seriesToken: 'test-series',
    }, {
        readSnapshot: () => ({
            directory, manifestPath,
            manifest: {snapshotId: sha256(canonicalJSON({test: true}))},
        }),
        createSeriesDirectory: () => ({directory: seriesDirectory, token: 'test-series'}),
        now: () => '2026-10-09T00:00:00.000Z',
        executeAttempt: async (_root, options) => {
            fs.mkdirSync(options.attemptDirectory, {recursive: true});
            if (options.scenario.key === 'read-unit-50' && options.kind === 'warmup') {
                return {status: 'failed', reason: 'warm-up failed'};
            }
            return {
                status: 'completed',
                summary: {rate: {effectiveOperationsPerSecond: 10}, operationLatency: {p95Ms: 5}},
            };
        },
    });
    assert.equal(result.index.status, 'incomplete');
    assert.equal(result.index.scenarios[0].status, 'blocked');
    assert.equal(result.index.scenarios[0].attempts.length, 2);
    assert.equal(result.index.scenarios[0].attempts.every((entry) => entry.kind === 'warmup'), true);
    assert.equal(result.index.scenarios.slice(1).every((entry) => entry.status === 'completed'), true);
});

test('full series extends from five to eight repetitions when population CV exceeds 15 percent', async (t) => {
    const directory = fs.mkdtempSync(path.join(repoRoot, 'build-series-test-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    const manifestPath = path.join(directory, 'manifest.json');
    fs.writeFileSync(manifestPath, '{}');
    const seriesDirectory = path.join(directory, 'series');
    fs.mkdirSync(seriesDirectory);
    const result = await runSeries(repoRoot, {
        snapshotDirectory: directory, plan: 'full', seriesToken: 'test-cv',
    }, {
        readSnapshot: () => ({
            directory, manifestPath,
            manifest: {snapshotId: sha256(canonicalJSON({test: 'cv'}))},
        }),
        createSeriesDirectory: () => ({directory: seriesDirectory, token: 'test-cv'}),
        scenarioMatrix: [{key: 'read-unit-50', scenario: 'read-unit', rate: 50}],
        now: () => '2026-10-09T00:00:00.000Z',
        executeAttempt: async (_root, options) => {
            fs.mkdirSync(options.attemptDirectory, {recursive: true});
            return {
                status: 'completed',
                summary: {
                    rate: {effectiveOperationsPerSecond: options.repetition === 2 ? 20 : 10},
                    operationLatency: {p95Ms: 5},
                },
            };
        },
    });
    const scenario = result.index.scenarios[0];
    assert.equal(scenario.extended, true);
    assert.equal(scenario.attempts.filter((entry) => entry.kind === 'measurement').length, 8);
    assert.equal(result.index.status, 'completed');
});
