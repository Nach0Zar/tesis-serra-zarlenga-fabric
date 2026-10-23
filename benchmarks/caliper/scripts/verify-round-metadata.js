#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const {buildPreparationMetadata, buildRoundMetadata, writeJSONAtomic} = require('../src/metadata');
const {buildProfile} = require('../src/profiles');
const {preparationDurationSeconds} = require('../src/snapshot');
const {readSourceTruth} = require('../src/sources');

const repoRoot = path.resolve(__dirname, '..', '..', '..');
const sourceTruth = readSourceTruth(repoRoot);
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
const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'snt-round-metadata-'));
try {
    const paths = cases.map((entry, index) => {
        const profile = buildProfile({...entry, phase: 'measurement', repetition: 1});
        const startedAt = new Date(Date.UTC(2026, 9, 2, 0, index * 3, 0));
        const endedAt = new Date(startedAt.getTime() + ((profile.durationSeconds + 1) * 1000));
        const document = buildRoundMetadata({
            profile,
            repositoryCommit: 'a'.repeat(40),
            dataset: {seed: 20260727, sha256: 'b'.repeat(64), units: 50000},
            startedAt: startedAt.toISOString(),
            endedAt: endedAt.toISOString(),
            host: {cpu: 'CI CPU', cpuCores: 2, memoryGB: 4, os: 'CI OS', kernel: 'CI kernel'},
            environment: {
                docker: '29.0.0', dockerCompose: '5.0.0',
                contractVersion: sourceTruth.contractVersion,
                packageID: sourceTruth.chaincode.packageID,
                fabric: '2.5.16', fabricCA: '1.5.15', caliper: '0.7.1',
            },
        }, {
            transactionCount: profile.durationSeconds * profile.rate * profile.transactionsPerOperation,
        });
        const filePath = path.join(directory, `${String(index).padStart(2, '0')}-${entry.scenario}.json`);
        writeJSONAtomic(filePath, document);
        return filePath;
    });
    for (const [name, elapsedMs] of [['clock-overrun', 6_425_567], ['resumed', 3_600_125]]) {
        const startedAt = '2026-10-13T00:00:00.000Z';
        const endedAt = '2026-10-13T01:47:05.566Z';
        const document = buildPreparationMetadata({
            repositoryCommit: 'a'.repeat(40),
            dataset: {seed: 20260727, sha256: 'b'.repeat(64), units: 50000},
            concurrency: 64,
            host: {cpu: 'CI CPU', cpuCores: 2, memoryGB: 4, os: 'CI OS', kernel: 'CI kernel'},
            environment: {
                docker: '29.0.0', dockerCompose: '5.0.0',
                contractVersion: sourceTruth.contractVersion,
                packageID: sourceTruth.chaincode.packageID,
                fabric: '2.5.16', fabricCA: '1.5.15', caliper: '0.7.1',
            },
        }, {
            startedAt,
            endedAt,
            durationSeconds: preparationDurationSeconds(startedAt, endedAt, elapsedMs),
            successfulWriteTransactions: 129878,
            markers: {expected: 46924, observed: 46924, fromRegistrations: 46920, fromRegulatoryEvents: 4},
        });
        const filePath = path.join(directory, `preparation-${name}.json`);
        writeJSONAtomic(filePath, document);
        paths.push(filePath);
    }
    const result = spawnSync('go', ['run', './cmd/runmeta', ...paths], {
        cwd: path.join(repoRoot, 'client'), encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
    });
    if (result.error) throw result.error;
    if (result.status !== 0) throw new Error((result.stderr || result.stdout).trim());
    process.stdout.write('All round and preparation metadata variants satisfy the metadata contract.\n');
} finally {
    fs.rmSync(directory, {recursive: true, force: true});
}
