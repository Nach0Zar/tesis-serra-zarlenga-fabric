#!/usr/bin/env node
'use strict';

const {parseSnapshotArguments} = require('./arguments');
const {runStandaloneSmoke} = require('./runner');

async function main() {
    const options = parseSnapshotArguments(process.argv.slice(2), true);
    await runStandaloneSmoke(options);
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write(`Baseline smoke failed: ${error.message}\n`);
        process.exitCode = 1;
    });
}

module.exports = {main};
