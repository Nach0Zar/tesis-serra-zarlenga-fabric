'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');

const EXPECTED_DATASET_SEED = 20260727;
const MINIMUM_DATASET_UNITS = 50000;
const DATASET_SCHEMA_ID = 'urn:pfi-snt:synthetic-dataset:schema:2.0.0';
const MANIFEST_SCHEMA_ID = 'urn:pfi-snt:synthetic-dataset-manifest:schema:2.0.0';
const DATASET_VERSION = '2.0.0';

function readJSON(filePath) {
    let document;
    try {
        document = JSON.parse(fs.readFileSync(filePath, 'utf8'));
    } catch (error) {
        throw new Error(`cannot read JSON ${filePath}: ${error.message}`);
    }
    return document;
}

function parseContractVersion(contents) {
    const matches = [...contents.matchAll(/^- \*\*Versión del contrato\*\*: `([^`]+)`\s*$/gmu)];
    if (matches.length !== 1 || !/^\d+\.\d+\.\d+$/.test(matches[0]?.[1] ?? '')) {
        throw new Error('docs/api-contract.md must contain exactly one semantic contract version header');
    }
    return matches[0][1];
}

function parseChaincodeLock(contents) {
    const values = new Map();
    for (const rawLine of contents.split(/\r?\n/u)) {
        const line = rawLine.trim();
        if (line === '') {
            continue;
        }
        const separator = line.indexOf('=');
        if (separator <= 0) {
            throw new Error(`invalid chaincode lock line: ${rawLine}`);
        }
        const key = line.slice(0, separator);
        if (values.has(key)) {
            throw new Error(`duplicate chaincode lock key: ${key}`);
        }
        values.set(key, line.slice(separator + 1));
    }

    const label = values.get('label');
    const version = values.get('version');
    const packageID = values.get('package_id');
    const sha256 = values.get('sha256');
    if (!label || !version || !packageID || !sha256) {
        throw new Error('network/chaincode-package.lock is incomplete');
    }
    if (!/^[0-9a-f]{64}$/u.test(sha256) || packageID !== `${label}:${sha256}`) {
        throw new Error('network/chaincode-package.lock has an inconsistent package_id');
    }
    return {label, version, packageID, sha256};
}

function sha256File(filePath) {
    return crypto.createHash('sha256').update(fs.readFileSync(filePath)).digest('hex');
}

function loadDatasetBundle(datasetDirectory) {
    const manifestPath = path.join(datasetDirectory, 'manifest.json');
    const datasetPath = path.join(datasetDirectory, 'dataset.json');
    const hashPath = path.join(datasetDirectory, 'dataset.sha256');
    const manifest = readJSON(manifestPath);
    const dataset = readJSON(datasetPath);

    if (manifest.$schema !== MANIFEST_SCHEMA_ID || manifest.schemaVersion !== DATASET_VERSION
        || dataset.$schema !== DATASET_SCHEMA_ID || dataset.schemaVersion !== DATASET_VERSION) {
        throw new Error(`dataset and manifest must use schema version ${DATASET_VERSION}`);
    }
    if (manifest.generator?.name !== 'cli-3-dataset-generator'
        || manifest.generator?.version !== DATASET_VERSION) {
        throw new Error(`dataset must be produced by cli-3-dataset-generator ${DATASET_VERSION}`);
    }
    if (manifest.seed !== EXPECTED_DATASET_SEED) {
        throw new Error(`dataset seed must be ${EXPECTED_DATASET_SEED}`);
    }
    if (!Number.isInteger(manifest.dataset?.units) || manifest.dataset.units < MINIMUM_DATASET_UNITS) {
        throw new Error(`dataset manifest must declare at least ${MINIMUM_DATASET_UNITS} units`);
    }
    if (!Array.isArray(dataset.units) || dataset.units.length !== manifest.dataset.units) {
        throw new Error('dataset unit count does not match manifest.json');
    }

    const calculatedHash = sha256File(datasetPath);
    const sidecarMatch = fs.readFileSync(hashPath, 'utf8').trim().match(/^([0-9a-f]{64})(?:\s+.*)?$/u);
    if (!sidecarMatch) {
        throw new Error('dataset.sha256 does not contain a SHA-256 digest');
    }
    if (calculatedHash !== manifest.dataset.sha256 || calculatedHash !== sidecarMatch[1]) {
        throw new Error('dataset SHA-256 does not match manifest.json and dataset.sha256');
    }

    return {
        manifest,
        dataset,
        manifestSHA256: sha256File(manifestPath),
        paths: {manifestPath, datasetPath, hashPath},
    };
}

function readSourceTruth(repoRoot) {
    const apiContractPath = path.join(repoRoot, 'docs', 'api-contract.md');
    const chaincodeLockPath = path.join(repoRoot, 'network', 'chaincode-package.lock');
    return {
        contractVersion: parseContractVersion(fs.readFileSync(apiContractPath, 'utf8')),
        chaincode: parseChaincodeLock(fs.readFileSync(chaincodeLockPath, 'utf8')),
        paths: {apiContractPath, chaincodeLockPath},
    };
}

module.exports = {
    EXPECTED_DATASET_SEED,
    DATASET_SCHEMA_ID,
    DATASET_VERSION,
    MANIFEST_SCHEMA_ID,
    MINIMUM_DATASET_UNITS,
    loadDatasetBundle,
    parseChaincodeLock,
    parseContractVersion,
    readJSON,
    readSourceTruth,
    sha256File,
};
