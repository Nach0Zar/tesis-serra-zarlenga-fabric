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

function resolveLabCredentials(repoRoot) {
    const manifest = readJSON(path.join(repoRoot, 'network', 'organizations-manifest.json'));
    const organization = manifest.organizations?.find((entry) => entry.mspId === 'LabMSP');
    if (!organization || organization.slug !== 'lab' || !organization.active || !organization.peerHostname) {
        throw new Error('active LabMSP entry is missing or incomplete in organizations-manifest.json');
    }

    const organizationRoot = path.join(repoRoot, 'network', 'organizations', organization.slug);
    const userMSP = path.join(
        organizationRoot,
        'users',
        `User1@${organization.slug}.snt.local`,
        'msp',
    );
    const certificatePath = onlyRegularFile(path.join(userMSP, 'signcerts'));
    const privateKeyPath = onlyRegularFile(path.join(userMSP, 'keystore'));
    const tlsCACertPath = requireRegularFile(path.join(
        organizationRoot,
        'peers',
        organization.peerHostname,
        'tls',
        'ca.crt',
    ));

    return {
        mspId: organization.mspId,
        peerHostname: organization.peerHostname,
        certificatePath,
        privateKeyPath,
        tlsCACertPath,
    };
}

module.exports = {onlyRegularFile, resolveLabCredentials};
