'use strict';
const fs = require('node:fs');
const path = require('node:path');
const {coefficientOfVariation} = require('./series-policy');
const {SERIES_SCHEMA_ID, sha256File, validateDocument} = require('./artifacts');
const {writeJSONAtomic} = require('./metadata');

function writeIndex(repoRoot, indexPath, index) {
    validateDocument(repoRoot, 'fabric-series-index.schema.json', index);
    writeJSONAtomic(indexPath, index);
}

function attemptRecord(seriesDirectory, kind, repetition, tryNumber, attemptDirectory, startedAt, endedAt, result) {
    const record = {
        kind,
        repetition,
        try: tryNumber,
        status: result.status,
        startedAt,
        endedAt,
        directory: path.relative(seriesDirectory, attemptDirectory).split(path.sep).join('/'),
    };
    // Preserve the executor-specific path and omit resources not reached after a failure.
    const resourcesPath = result.resourcesPath ?? path.join(attemptDirectory, 'resources.txt');
    if (fs.existsSync(resourcesPath)) {
        record.resources = path.relative(seriesDirectory, resourcesPath).split(path.sep).join('/');
    }
    if (result.reason) record.reason = result.reason;
    if (result.summaryPath) record.summary = path.relative(seriesDirectory, result.summaryPath).split(path.sep).join('/');
    return record;
}

function initializeSeries(repoRoot, directory, token, plan, citable, snapshot, matrix, startedAt) {
    const relative = path.relative(repoRoot, snapshot.directory);
    if (relative.startsWith('..') || path.isAbsolute(relative)) throw new Error('series snapshot must be inside repository');
    const scenarios = matrix.map((entry) => ({...entry, status: 'pending', extended: false, attempts: []}));
    const index = {$schema: SERIES_SCHEMA_ID, schemaVersion: '1.0.0', seriesToken: token, plan, citable,
        snapshot: {id: snapshot.manifest.snapshotId, directory: relative.split(path.sep).join('/'),
            manifestSHA256: sha256File(snapshot.manifestPath)},
        startedAt, status: 'running', scenarios};
    const indexPath = path.join(directory, 'series-index.json');
    writeIndex(repoRoot, indexPath, index);
    return {index, indexPath, scenarios};
}

module.exports = {attemptRecord, coefficientOfVariation, initializeSeries, writeIndex};
