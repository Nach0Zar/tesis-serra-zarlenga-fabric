'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const {selectLabRegistration} = require('../src/dataset');
const {onlyRegularFile, resolveLabCredentials, resolveOrganizationCredentials} = require('../src/credentials');
const {
    buildMultiOrganizationNetworkConfig,
    buildNetworkConfig,
    normalizePublishedEndpoint,
    resolveComposeVersions,
} = require('../src/network-config');
const {buildMetadata} = require('../src/metadata');
const {loadDatasetBundle, parseChaincodeLock, parseContractVersion, readSourceTruth} = require('../src/sources');

const repoRoot = path.resolve(__dirname, '..', '..', '..');

test('canonical contract and package sources are parsed', () => {
    const sourceTruth = readSourceTruth(repoRoot);
    assert.equal(sourceTruth.contractVersion, '2.11.2');
    assert.equal(
        sourceTruth.chaincode.packageID,
        'snt_1.0:a4d9315a304ac10b467c096f37f4960f86e4ce2d828310aed58d5ad34082d9a9',
    );
});

test('source parsers reject ambiguous or inconsistent content', () => {
    assert.throws(() => parseContractVersion('- **Versión del contrato**: `2.11.2`\n- **Versión del contrato**: `2.11.3`'));
    assert.throws(() => parseChaincodeLock('label=snt_1.0\nversion=1.0\npackage_id=snt_1.0:bad\nsha256=bad\n'));
});

test('dataset bundle preflight rejects unversioned CLI-3 artifacts', (t) => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-dataset-version-'));
    t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
    fs.writeFileSync(path.join(directory, 'manifest.json'), JSON.stringify({seed: 20260727}));
    fs.writeFileSync(path.join(directory, 'dataset.json'), JSON.stringify({units: []}));
    fs.writeFileSync(path.join(directory, 'dataset.sha256'), `${'0'.repeat(64)}\n`);
    assert.throws(() => loadDatasetBundle(directory), /schema version 2\.0\.0/u);
});

test('LabMSP registration is selected from preparation recipes only', () => {
    const selected = selectLabRegistration({units: [{
        sequence: 7,
        preparation: [{
            operation: 'RegisterUnit',
            invokerMspId: 'LabMSP',
            request: {
                gtin: '07791234567898',
                numeroSerie: 'SN-7',
                lote: 'LOT-7',
                fechaVencimiento: '2099-01-01',
            },
        }],
        expectedSuccess: {operation: 'Dispense'},
    }]});
    assert.equal(selected.sequence, 7);
    assert.equal(selected.request.numeroSerie, 'SN-7');
});

test('credential resolution requires exactly one certificate and key without reading PEM data', (t) => {
    const temporaryRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-caliper-'));
    t.after(() => fs.rmSync(temporaryRoot, {recursive: true, force: true}));
    const userMSP = path.join(temporaryRoot, 'network', 'organizations', 'lab', 'users', 'User1@lab.snt.local', 'msp');
    const peerTLS = path.join(temporaryRoot, 'network', 'organizations', 'lab', 'peers', 'peer0.lab.snt.local', 'tls');
    fs.mkdirSync(path.join(userMSP, 'signcerts'), {recursive: true});
    fs.mkdirSync(path.join(userMSP, 'keystore'), {recursive: true});
    fs.mkdirSync(peerTLS, {recursive: true});
    fs.writeFileSync(path.join(userMSP, 'signcerts', 'cert.pem'), 'CERTIFICATE-CONTENT');
    fs.writeFileSync(path.join(userMSP, 'keystore', 'key.pem'), 'PRIVATE-KEY-CONTENT');
    fs.writeFileSync(path.join(peerTLS, 'ca.crt'), 'CA-CONTENT');
    fs.writeFileSync(path.join(temporaryRoot, 'network', 'organizations-manifest.json'), JSON.stringify({
        organizations: [{
            mspId: 'LabMSP',
            slug: 'lab',
            active: true,
            peerHostname: 'peer0.lab.snt.local',
        }],
    }));

    const credentials = resolveLabCredentials(temporaryRoot);
    assert.equal(path.basename(credentials.privateKeyPath), 'key.pem');
    assert.doesNotMatch(JSON.stringify(credentials), /PRIVATE-KEY-CONTENT/u);
    fs.writeFileSync(path.join(userMSP, 'keystore', 'second.pem'), 'SECOND');
    assert.throws(() => onlyRegularFile(path.join(userMSP, 'keystore')), /exactly one/u);
});

test('effective network config targets the own peer, channel and chaincode', () => {
    const sourceTruth = readSourceTruth(repoRoot);
    const config = buildNetworkConfig({
        credentials: {
            mspId: 'LabMSP',
            peerHostname: 'peer0.lab.snt.local',
            privateKeyPath: '/generated/key.pem',
            certificatePath: '/generated/cert.pem',
            tlsCACertPath: '/generated/ca.crt',
        },
        endpoint: '127.0.0.1:8051',
        sourceTruth,
        versions: {fabric: '2.5.16', fabricCA: '1.5.17'},
        caliperVersion: '0.7.1',
    });
    assert.equal(config.channels[0].channelName, 'snt-channel');
    assert.equal(config.channels[0].contracts[0].id, 'snt');
    assert.equal(config.organizations[0].peers[0].endpoint, '127.0.0.1:8051');
    assert.equal(config.info.ContractVersion, '2.11.2');
    assert.equal(config.info.PackageID, sourceTruth.chaincode.packageID);
    assert.doesNotMatch(JSON.stringify(config), /BEGIN (?:PRIVATE KEY|CERTIFICATE)/u);
});

test('multi-organization credentials and config keep only generated file paths', (t) => {
    const temporaryRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-caliper-multi-'));
    t.after(() => fs.rmSync(temporaryRoot, {recursive: true, force: true}));
    const organizations = [
        {mspId: 'LabMSP', slug: 'lab', peerHostname: 'peer0.lab.snt.local'},
        {mspId: 'FarmaciaMSP', slug: 'farmacia', peerHostname: 'peer0.farmacia.snt.local'},
    ];
    for (const organization of organizations) {
        const root = path.join(temporaryRoot, 'network', 'organizations', organization.slug);
        const msp = path.join(root, 'users', `User1@${organization.slug}.snt.local`, 'msp');
        fs.mkdirSync(path.join(msp, 'signcerts'), {recursive: true});
        fs.mkdirSync(path.join(msp, 'keystore'), {recursive: true});
        fs.mkdirSync(path.join(root, 'peers', organization.peerHostname, 'tls'), {recursive: true});
        fs.writeFileSync(path.join(msp, 'signcerts', 'cert.pem'), 'CERTIFICATE-CONTENT');
        fs.writeFileSync(path.join(msp, 'keystore', 'key.pem'), 'PRIVATE-KEY-CONTENT');
        fs.writeFileSync(path.join(root, 'peers', organization.peerHostname, 'tls', 'ca.crt'), 'CA-CONTENT');
    }
    fs.writeFileSync(path.join(temporaryRoot, 'network', 'organizations-manifest.json'), JSON.stringify({
        organizations: organizations.map((organization) => ({...organization, active: true})),
    }));
    const credentials = resolveOrganizationCredentials(temporaryRoot, ['LabMSP', 'FarmaciaMSP']);
    const sourceTruth = readSourceTruth(repoRoot);
    const config = buildMultiOrganizationNetworkConfig({
        organizations: credentials,
        endpoints: {LabMSP: 'localhost:8051', FarmaciaMSP: 'localhost:11051'},
        sourceTruth,
        versions: {fabric: '2.5.16', fabricCA: '1.5.15'},
        caliperVersion: '0.7.1',
    });
    assert.deepEqual(config.organizations.map((entry) => entry.mspid), ['LabMSP', 'FarmaciaMSP']);
    assert.deepEqual(config.organizations.map((entry) => entry.identities.certificates[0].name), [
        'lab-user1', 'farmacia-user1',
    ]);
    assert.doesNotMatch(JSON.stringify(config), /PRIVATE-KEY-CONTENT|CERTIFICATE-CONTENT/u);
});

test('Docker Compose port and image versions are normalized', () => {
    assert.equal(normalizePublishedEndpoint('0.0.0.0:8051\n'), 'localhost:8051');
    assert.equal(normalizePublishedEndpoint('127.0.0.1:8051'), 'localhost:8051');
    assert.equal(normalizePublishedEndpoint('[::]:8051'), 'localhost:8051');
    assert.deepEqual(resolveComposeVersions({services: {
        'peer0.lab.snt.local': {image: 'hyperledger/fabric-peer:2.5.16'},
        'fabric-ca.snt.local': {image: 'hyperledger/fabric-ca:1.5.17'},
    }}), {fabric: '2.5.16', fabricCA: '1.5.17'});
});

test('metadata builder emits the smoke shape and canonical identifiers', () => {
    const sourceTruth = readSourceTruth(repoRoot);
    const context = {
        repositoryCommit: 'a'.repeat(40),
        expectedTransactions: 30,
        dataset: {seed: 20260727, sha256: 'b'.repeat(64), units: 50000},
        host: {cpu: 'test CPU', cpuCores: 1, memoryGB: 1, os: 'test OS', kernel: 'test kernel'},
        environment: {
            docker: '29.0.0',
            dockerCompose: '5.0.0',
            contractVersion: sourceTruth.contractVersion,
            packageID: sourceTruth.chaincode.packageID,
            fabric: '2.5.16',
            fabricCA: '1.5.17',
            caliper: '0.7.1',
        },
    };
    const document = buildMetadata(context, {
        startedAt: '2026-10-01T00:00:00.000Z',
        endedAt: '2026-10-01T00:00:30.000Z',
        successfulTransactions: 30,
    });
    assert.equal(document.scenario, 'smoke');
    assert.equal(document.transactions, 30);
    assert.equal(document.rate.effectiveTransactionsPerSecond, 1);
    assert.equal(document.environment.contractVersion, '2.11.2');
    assert.equal(document.environment.packageID, sourceTruth.chaincode.packageID);
    assert.equal(document.discarded, undefined);

    const discarded = buildMetadata(context, {
        startedAt: '2026-10-01T00:00:00.000Z',
        endedAt: '2026-10-01T00:00:30.000Z',
        successfulTransactions: 29,
        discardReason: 'completed 29 of 30 ReadUnit requests',
    });
    assert.deepEqual(discarded.discarded, {reason: 'completed 29 of 30 ReadUnit requests'});
});
