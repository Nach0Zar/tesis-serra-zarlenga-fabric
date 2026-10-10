#!/usr/bin/env node
'use strict';

const path = require('node:path');

const {readSnapshot} = require('./artifacts');
const {resolveOrganizationCredentials} = require('./credentials');
const {GatewayPool} = require('./gateway-bridge');
const {buildMultiOrganizationNetworkConfig, inspectOrganizationRuntime} = require('./network-config');
const {assertSnapshotCompatibility, CALIPER_VERSION} = require('./snapshot');
const {readPreconditions} = require('./snapshot-preconditions');
const {equalUnit, stableUnit} = require('./snapshot-state');
const {loadDatasetBundle, readSourceTruth} = require('./sources');
const {registrationForUnit} = require('./snapshot-plan');

function parseArguments(arguments_) {
    const values = {};
    for (let index = 0; index < arguments_.length; index += 2) {
        const name = arguments_[index];
        const value = arguments_[index + 1];
        if (!name?.startsWith('--') || value === undefined || value.startsWith('--')) {
            throw new Error('arguments must be supplied as --name value pairs');
        }
        values[name.slice(2)] = value;
    }
    for (const key of Object.keys(values)) {
        if (!['snapshot-dir', 'dataset-dir'].includes(key)) throw new Error(`unsupported argument --${key}`);
    }
    if (!values['snapshot-dir']) throw new Error('--snapshot-dir is required');
    return {snapshotDirectory: values['snapshot-dir'], datasetDirectory: values['dataset-dir']};
}

async function warmConfiguredPeers(gatewayPool, credentials, request, expected) {
    const warmed = [];
    for (const organization of credentials) {
        const observed = await gatewayPool.readUnit(organization.mspId, request);
        if (!equalUnit(stableUnit(observed), expected)) {
            throw new Error(`${organization.mspId} returned a different smoke unit state`);
        }
        warmed.push({mspId: organization.mspId, peerHostname: organization.peerHostname});
    }
    if (warmed.length !== 7) throw new Error(`expected to warm 7 peers, warmed ${warmed.length}`);
    return warmed;
}

async function warmPeers(repoRoot, options = {}) {
    const snapshot = readSnapshot(repoRoot, options.snapshotDirectory);
    assertSnapshotCompatibility(repoRoot, snapshot);
    const bundle = loadDatasetBundle(path.resolve(
        repoRoot, options.datasetDirectory ?? path.join('build', 'dataset'),
    ));
    if (bundle.manifest.dataset.sha256 !== snapshot.manifest.dataset.sha256) {
        throw new Error('snapshot and warm-up dataset hashes differ');
    }
    const unit = bundle.dataset.units.find((entry) => entry.sequence === snapshot.manifest.smokeSequence);
    const registration = registrationForUnit(unit);
    const expected = readPreconditions(snapshot).get(unit.sequence)?.unit;
    if (!expected) throw new Error('snapshot smoke sequence is not present');
    const credentials = resolveOrganizationCredentials(repoRoot);
    const sourceTruth = readSourceTruth(repoRoot);
    const runtime = inspectOrganizationRuntime(repoRoot, credentials);
    const networkConfig = buildMultiOrganizationNetworkConfig({
        organizations: credentials,
        endpoints: runtime.endpoints,
        sourceTruth,
        versions: runtime,
        caliperVersion: CALIPER_VERSION,
    });
    const gatewayPool = new GatewayPool(networkConfig);
    let warmed;
    try {
        warmed = await warmConfiguredPeers(gatewayPool, credentials, registration.request, expected);
    } finally {
        gatewayPool.close();
    }
    return {snapshotId: snapshot.manifest.snapshotId, sequence: unit.sequence, warmed};
}

async function main() {
    const repoRoot = path.resolve(__dirname, '..', '..', '..');
    const result = await warmPeers(repoRoot, parseArguments(process.argv.slice(2)));
    process.stdout.write(`${JSON.stringify(result)}\n`);
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write(`Fabric peer warm-up failed: ${error.message}\n`);
        process.exitCode = 1;
    });
}

module.exports = {main, parseArguments, warmConfiguredPeers, warmPeers};
