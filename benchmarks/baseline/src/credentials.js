'use strict';

const fs = require('node:fs');
const path = require('node:path');

function parseCredentials(contents, manifest, requiredMspIds = []) {
    let entries;
    try {
        entries = JSON.parse(contents);
    } catch (error) {
        throw new Error(`SNT_BASELINE_API_KEYS is not valid JSON: ${error.message}`);
    }
    if (!Array.isArray(entries)) throw new Error('SNT_BASELINE_API_KEYS must be an array');
    const organizations = new Map((manifest.organizations ?? []).map((entry) => [entry.mspId, entry]));
    const credentials = new Map();
    for (const entry of entries) {
        const organization = organizations.get(entry?.mspId);
        if (!organization || !organization.active || entry.role !== organization.clientRole
            || typeof entry.key !== 'string' || entry.key.length === 0) {
            throw new Error(`invalid baseline credential entry for ${entry?.mspId ?? 'unknown'}`);
        }
        if (credentials.has(entry.mspId)) throw new Error(`duplicate baseline credential for ${entry.mspId}`);
        credentials.set(entry.mspId, {key: entry.key, role: entry.role});
    }
    for (const mspId of requiredMspIds) {
        if (!credentials.has(mspId)) throw new Error(`missing baseline credential for ${mspId}`);
    }
    return credentials;
}

function resolveCredentials(repoRoot, requiredMspIds, environment = process.env) {
    if (!environment.SNT_BASELINE_API_KEYS) throw new Error('SNT_BASELINE_API_KEYS is required');
    const manifest = JSON.parse(fs.readFileSync(path.join(repoRoot, 'network', 'organizations-manifest.json'), 'utf8'));
    return parseCredentials(environment.SNT_BASELINE_API_KEYS, manifest, requiredMspIds);
}

function publicCredentialConfiguration(credentials) {
    return [...credentials.entries()].map(([mspId, value]) => ({mspId, role: value.role}));
}

module.exports = {parseCredentials, publicCredentialConfiguration, resolveCredentials};
