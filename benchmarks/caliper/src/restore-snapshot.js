#!/usr/bin/env node
'use strict';

const path = require('node:path');

const {parseSnapshotArguments, restoreSnapshot} = require('./snapshot');

function main() {
    const repoRoot = path.resolve(__dirname, '..', '..', '..');
    const options = parseSnapshotArguments(process.argv.slice(2), 'restore');
    const restored = restoreSnapshot(repoRoot, options.snapshotDirectory, options);
    process.stdout.write(`${JSON.stringify({
        snapshotId: restored.snapshot.manifest.snapshotId,
        directory: restored.snapshot.directory,
        durationSeconds: restored.durationSeconds,
    })}\n`);
}

if (require.main === module) {
    try {
        main();
    } catch (error) {
        process.stderr.write(`Fabric snapshot restore failed: ${error.message}\n`);
        process.exitCode = 1;
    }
}

module.exports = {main};
