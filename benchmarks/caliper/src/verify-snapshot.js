#!/usr/bin/env node
'use strict';

const path = require('node:path');

const {parseSnapshotArguments, verifySnapshot} = require('./snapshot');

async function main() {
    const repoRoot = path.resolve(__dirname, '..', '..', '..');
    const options = parseSnapshotArguments(process.argv.slice(2), 'verify');
    const result = await verifySnapshot(repoRoot, options.snapshotDirectory, options);
    process.stdout.write(`${JSON.stringify(result)}\n`);
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write(`Fabric snapshot verification failed: ${error.message}\n`);
        process.exitCode = 1;
    });
}

module.exports = {main};
