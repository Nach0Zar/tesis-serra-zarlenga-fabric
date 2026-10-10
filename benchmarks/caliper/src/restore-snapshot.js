#!/usr/bin/env node
'use strict';

const path = require('node:path');

const {readSnapshot, restoreSnapshot} = require('./fabric-snapshot');
const {writeJSONAtomic} = require('./metadata');
const {unitReference} = require('./snapshot-builder');
const {loadDatasetBundle, readSourceTruth} = require('./sources');

const CALIPER_VERSION = require('../package.json').devDependencies['@hyperledger/caliper-cli'];

function parseArguments(arguments_) {
    const options = {};
    for (let index = 0; index < arguments_.length; index += 2) {
        const [name, value] = [arguments_[index], arguments_[index + 1]];
        if (value === undefined || value.startsWith('--')) throw new Error(`${name} requires a value`);
        if (name === '--snapshot-dir') options.snapshotDirectory = value;
        else if (name === '--dataset-dir') options.datasetDirectory = value;
        else if (name === '--output') options.output = value;
        else throw new Error(`unsupported argument ${name}`);
    }
    if (!options.snapshotDirectory) throw new Error('--snapshot-dir is required');
    return options;
}

async function main() {
    const options = parseArguments(process.argv.slice(2));
    const repoRoot = path.resolve(__dirname, '..', '..', '..');
    const snapshotDirectory = path.resolve(repoRoot, options.snapshotDirectory);
    const bundle = loadDatasetBundle(path.resolve(repoRoot, options.datasetDirectory ?? path.join('build', 'dataset')));
    const {manifest} = readSnapshot(snapshotDirectory);
    if (bundle.manifest.dataset.sha256 !== manifest.dataset.sha256) throw new Error('dataset does not match the snapshot');
    const smokeUnit = bundle.dataset.units.find((unit) => unit.sequence === manifest.smokeSequence);
    const result = await restoreSnapshot({
        repoRoot, snapshotDirectory, sourceTruth: readSourceTruth(repoRoot), caliperVersion: CALIPER_VERSION,
        smokeReference: unitReference(smokeUnit),
    });
    const record = {snapshotId: manifest.snapshotId, restoredAt: new Date().toISOString(), timings: result.timings};
    if (options.output) writeJSONAtomic(path.resolve(options.output), record);
    process.stdout.write(`Fabric snapshot ${manifest.snapshotId} restored: ${JSON.stringify(result.timings)}\n`);
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write(`Fabric snapshot restore failed: ${error.message}\n`);
        process.exitCode = 1;
    });
}

module.exports = {parseArguments};
