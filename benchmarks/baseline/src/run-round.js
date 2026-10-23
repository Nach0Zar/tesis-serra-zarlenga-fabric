#!/usr/bin/env node
'use strict';

const {parseRoundArguments} = require('./arguments');
const {runRound} = require('./runner');

async function main() {
    await runRound(parseRoundArguments(process.argv.slice(2)));
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write(`Baseline round failed: ${error.message}\n`);
        process.exitCode = 1;
    });
}

module.exports = {main};
