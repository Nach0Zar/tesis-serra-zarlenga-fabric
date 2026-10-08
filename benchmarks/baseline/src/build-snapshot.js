#!/usr/bin/env node
'use strict';

const {parseSnapshotArguments} = require('./arguments');
const {buildSnapshot} = require('./snapshot');

async function main() {
    await buildSnapshot(parseSnapshotArguments(process.argv.slice(2)));
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write(`Baseline snapshot build failed: ${error.message}\n`);
        process.exitCode = 1;
    });
}

module.exports = {main};
