#!/usr/bin/env node
'use strict';

const {parseSnapshotArguments} = require('./arguments');
const {repoRoot} = require('./paths');
const {compose} = require('./runtime');
const {restoreSnapshot} = require('./snapshot');

function main() {
    const options = parseSnapshotArguments(process.argv.slice(2), true);
    const restored = restoreSnapshot(options.snapshotDirectory);
    process.stdout.write(`Baseline snapshot restored: ${restored.directory}\n`);
    process.stdout.write(`Compose project: ${restored.environment.COMPOSE_PROJECT_NAME}\n`);
    if (process.env.SNT_BASELINE_RESTORE_KEEP_RUNNING !== '1') {
        compose(repoRoot, ['down', '--volumes', '--remove-orphans'], {env: restored.environment, allowFailure: true});
    }
}

if (require.main === module) {
    try {
        main();
    } catch (error) {
        process.stderr.write(`Baseline snapshot restore failed: ${error.message}\n`);
        process.exitCode = 1;
    }
}

module.exports = {main};
