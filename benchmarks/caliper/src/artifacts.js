'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');

const Ajv2020 = require('ajv/dist/2020');

const SNAPSHOT_SCHEMA_ID = 'urn:pfi-snt:fabric-snapshot-manifest:schema:1.0.0';
const SERIES_SCHEMA_ID = 'urn:pfi-snt:fabric-series-index:schema:1.0.0';

function canonicalValue(value) {
    if (Array.isArray(value)) return value.map(canonicalValue);
    if (value && typeof value === 'object') {
        return Object.fromEntries(Object.keys(value).sort().map((key) => [key, canonicalValue(value[key])]));
    }
    return value;
}

function canonicalJSON(value) {
    return JSON.stringify(canonicalValue(value));
}

function sha256(value) {
    return crypto.createHash('sha256').update(value).digest('hex');
}

function sha256File(filePath) {
    const hash = crypto.createHash('sha256');
    const descriptor = fs.openSync(filePath, 'r');
    const buffer = Buffer.allocUnsafe(1024 * 1024);
    try {
        let bytes;
        do {
            bytes = fs.readSync(descriptor, buffer, 0, buffer.length, null);
            if (bytes > 0) hash.update(buffer.subarray(0, bytes));
        } while (bytes > 0);
    } finally {
        fs.closeSync(descriptor);
    }
    return hash.digest('hex');
}

function fileDescriptor(filePath, rootDirectory = path.dirname(filePath)) {
    const relative = path.relative(rootDirectory, filePath);
    if (relative.startsWith('..') || path.isAbsolute(relative)) throw new Error('artifact file must be inside its root');
    return {
        name: relative.split(path.sep).join('/'),
        sha256: sha256File(filePath),
        bytes: fs.statSync(filePath).size,
    };
}

function schemaPath(repoRoot, schemaName) {
    return path.join(repoRoot, 'benchmarks', 'schema', schemaName);
}

function validateDocument(repoRoot, schemaName, document) {
    const schema = JSON.parse(fs.readFileSync(schemaPath(repoRoot, schemaName), 'utf8'));
    const ajv = new Ajv2020({allErrors: true, strict: true});
    const validate = ajv.compile(schema);
    if (!validate(document)) {
        throw new Error(`${schemaName} validation failed: ${ajv.errorsText(validate.errors, {separator: '; '})}`);
    }
    return document;
}

function resolveArtifact(rootDirectory, descriptor) {
    const candidate = path.resolve(rootDirectory, descriptor.name);
    const relative = path.relative(rootDirectory, candidate);
    if (relative.startsWith('..') || path.isAbsolute(relative)) throw new Error(`artifact escapes snapshot directory: ${descriptor.name}`);
    return candidate;
}

function verifyDescriptor(rootDirectory, descriptor) {
    const filePath = resolveArtifact(rootDirectory, descriptor);
    if (!fs.statSync(filePath).isFile()) throw new Error(`snapshot artifact is not a file: ${descriptor.name}`);
    if (fs.statSync(filePath).size !== descriptor.bytes) throw new Error(`snapshot artifact size mismatch: ${descriptor.name}`);
    if (sha256File(filePath) !== descriptor.sha256) throw new Error(`snapshot artifact SHA-256 mismatch: ${descriptor.name}`);
    return filePath;
}

function readSnapshot(repoRoot, snapshotDirectory, options = {}) {
    const directory = path.resolve(repoRoot, snapshotDirectory);
    const manifestPath = path.join(directory, 'manifest.json');
    const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
    validateDocument(repoRoot, 'fabric-snapshot-manifest.schema.json', manifest);
    for (const descriptor of Object.values(manifest.files)) verifyDescriptor(directory, descriptor);
    if (options.verifyVolumes !== false) {
        for (const volume of manifest.volumes) verifyDescriptor(directory, {
            name: volume.archive,
            sha256: volume.sha256,
            bytes: volume.bytes,
        });
    }
    return {directory, manifest, manifestPath};
}

module.exports = {
    SERIES_SCHEMA_ID,
    SNAPSHOT_SCHEMA_ID,
    canonicalJSON,
    fileDescriptor,
    readSnapshot,
    resolveArtifact,
    sha256,
    sha256File,
    validateDocument,
    verifyDescriptor,
};
