'use strict';

const fs = require('node:fs');
const path = require('node:path');

const {readJSON} = require('./sources');

function onlyRegularFile(directory) {
    const matches = fs.readdirSync(directory, {withFileTypes: true})
        .filter((entry) => entry.isFile())
        .map((entry) => path.join(directory, entry.name));
    if (matches.length !== 1) {
        throw new Error(`expected exactly one regular file in ${directory}, found ${matches.length}`);
    }
    return matches[0];
}

function requireRegularFile(filePath) {
    if (!fs.statSync(filePath).isFile()) {
        throw new Error(`expected a regular file at ${filePath}`);
    }
    return filePath;
}

function resolveOrganizationCredentials(repoRoot, requestedMspIds) {
    const manifest = readJSON(path.join(repoRoot, 'network', 'organizations-manifest.json'));
    if (!Array.isArray(manifest.organizations)) {
        throw new Error('organizations-manifest.json must contain an organizations array');
    }
    const requested = requestedMspIds === undefined
        ? manifest.organizations.filter((entry) => entry.active).map((entry) => entry.mspId)
        : [...new Set(requestedMspIds)];

    return requested.map((mspId) => {
        const organization = manifest.organizations.find((entry) => entry.mspId === mspId);
        if (!organization || !organization.slug || !organization.active || !organization.peerHostname) {
            throw new Error(`active ${mspId} entry is missing or incomplete in organizations-manifest.json`);
        }
        const organizationRoot = path.join(repoRoot, 'network', 'organizations', organization.slug);
        const userMSP = path.join(
            organizationRoot,
            'users',
            `User1@${organization.slug}.snt.local`,
            'msp',
        );

        return {
            mspId: organization.mspId,
            slug: organization.slug,
            peerHostname: organization.peerHostname,
            certificatePath: onlyRegularFile(path.join(userMSP, 'signcerts')),
            privateKeyPath: onlyRegularFile(path.join(userMSP, 'keystore')),
            tlsCACertPath: requireRegularFile(path.join(
                organizationRoot,
                'peers',
                organization.peerHostname,
                'tls',
                'ca.crt',
            )),
        };
    });
}

function resolveLabCredentials(repoRoot) {
    return resolveOrganizationCredentials(repoRoot, ['LabMSP'])[0];
}

module.exports = {onlyRegularFile, resolveLabCredentials, resolveOrganizationCredentials};
