#!/usr/bin/env node
'use strict';

const path = require('node:path');

const {buildSnapshot, parseSnapshotArguments} = require('./snapshot');
const {preparationErrorDiagnostics} = require('./snapshot-state');

async function main() {
    const repoRoot = path.resolve(__dirname, '..', '..', '..');
    await buildSnapshot(repoRoot, parseSnapshotArguments(process.argv.slice(2), 'build'));
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write('Fabric snapshot build failed; see failure.json and the preparation journal.\n');
        process.stderr.write(`${JSON.stringify(preparationErrorDiagnostics(error))}\n`);
        process.exitCode = 1;
    });
}

module.exports = {main};
