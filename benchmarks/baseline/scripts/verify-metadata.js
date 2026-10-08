#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const {writeJSONAtomic} = require('../../caliper/src/metadata');
const {buildProfile} = require('../../caliper/src/profiles');
const {buildRoundMetadata, buildSmokeMetadata} = require('../src/metadata');
const {repoRoot} = require('../src/paths');

const cases = [
    {scenario: 'write-register', rate: 5},
    {scenario: 'write-transfer', rate: 5},
    {scenario: 'write-dispense', rate: 5},
    {scenario: 'read-unit', rate: 10},
    {scenario: 'read-history', rate: 10},
    {scenario: 'mixed', rate: 20},
    {scenario: 'expected-rejections', rate: 5, family: 'UNAUTHORIZED_TRANSFER'},
    {scenario: 'expected-rejections', rate: 5, family: 'DUPLICATE_IDENTITY'},
    {scenario: 'expected-rejections', rate: 5, family: 'BLOCKING_STATE', operation: 'transfer'},
    {scenario: 'expected-rejections', rate: 5, family: 'BLOCKING_STATE', operation: 'dispense'},
];

function baseContext() {
    return {
        repositoryCommit: 'a'.repeat(40),
        dataset: {seed: 20260727, sha256: 'b'.repeat(64), units: 50000},
        host: {cpu: 'CI CPU', cpuCores: 2, memoryGB: 4, os: 'CI OS', kernel: 'CI kernel'},
        environment: {
            docker: '29.0.0', dockerCompose: '5.0.0',
            baselineCommit: 'c'.repeat(40), baselineImage: `snt-baseline@sha256:${'d'.repeat(64)}`, postgres: '16.15',
        },
    };
}

function validate(paths) {
    const result = spawnSync('go', ['run', './cmd/runmeta', ...paths], {
        cwd: path.join(repoRoot, 'client'), encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
    });
    if (result.error) throw result.error;
    if (result.status !== 0) throw new Error((result.stderr || result.stdout).trim());
}

function main() {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-baseline-metadata-'));
    try {
        const paths = cases.map((entry, index) => {
            const profile = buildProfile({...entry, phase: 'measurement', repetition: 1});
            const startedAt = new Date(Date.UTC(2026, 9, 8, 0, index * 3, 0));
            const endedAt = new Date(startedAt.getTime() + (profile.durationSeconds * 1000));
            const document = buildRoundMetadata({
                ...baseContext(), profile, startedAt: startedAt.toISOString(), endedAt: endedAt.toISOString(),
            }, {
                rate: {effectiveTransactionsPerSecond: profile.rate * profile.transactionsPerOperation},
            });
            const filePath = path.join(directory, `${String(index).padStart(2, '0')}-${entry.scenario}.json`);
            writeJSONAtomic(filePath, document);
            return filePath;
        });
        const smoke = buildSmokeMetadata(baseContext(), {
            startedAt: '2026-10-08T01:00:00.000Z', endedAt: '2026-10-08T01:00:30.000Z',
            observedDurationSeconds: 30, successfulTransactions: 30,
        });
        const smokePath = path.join(directory, 'smoke.json');
        writeJSONAtomic(smokePath, smoke);
        validate([...paths, smokePath]);
        process.stdout.write('All baseline metadata variants satisfy the metadata contract.\n');
    } finally {
        fs.rmSync(directory, {recursive: true, force: true});
    }
}

if (require.main === module) main();

module.exports = {baseContext, main};
