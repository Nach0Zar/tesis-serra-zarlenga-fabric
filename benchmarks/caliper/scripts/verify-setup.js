#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const projectRoot = path.resolve(__dirname, '..');
const packageDocument = JSON.parse(fs.readFileSync(path.join(projectRoot, 'package.json'), 'utf8'));
const lockDocument = JSON.parse(fs.readFileSync(path.join(projectRoot, 'package-lock.json'), 'utf8'));

if (packageDocument.packageManager !== 'npm@11.6.2'
    || packageDocument.engines?.node !== '>=22 <23'
    || packageDocument.engines?.npm !== '>=11.5.1 <12') {
    throw new Error('Node and npm versions must remain inside the supported pinned runtime');
}

function installedVersion(packageName) {
    const entry = lockDocument.packages?.[`node_modules/${packageName}`];
    if (!entry?.version) {
        throw new Error(`${packageName} is absent from package-lock.json; run npm run bind:fabric`);
    }
    return entry.version;
}

if (packageDocument.devDependencies?.['@hyperledger/caliper-cli'] !== '0.7.1') {
    throw new Error('@hyperledger/caliper-cli must be pinned exactly to 0.7.1');
}
for (const dependency of ['@hyperledger/caliper-cli', '@hyperledger/caliper-core']) {
    if (packageDocument.devDependencies?.[dependency] !== '0.7.1' || installedVersion(dependency) !== '0.7.1') {
        throw new Error(`${dependency} must be pinned exactly to 0.7.1`);
    }
}
for (const dependency of ['@hyperledger/fabric-gateway', '@grpc/grpc-js']) {
    const declared = packageDocument.devDependencies?.[dependency];
    if (!declared || declared.startsWith('^') || declared.startsWith('~')) {
        throw new Error(`${dependency} must be an exact devDependency produced by fabric:fabric-gateway binding`);
    }
    if (installedVersion(dependency) !== declared) {
        throw new Error(`${dependency} does not match its locked version`);
    }
}
for (const [dependency, expected] of Object.entries({
    '@hyperledger/fabric-protos': '0.3.7',
    ajv: '8.17.1',
})) {
    if (packageDocument.devDependencies?.[dependency] !== expected || installedVersion(dependency) !== expected) {
        throw new Error(`${dependency} must be pinned exactly to ${expected}`);
    }
}
if (installedVersion('@hyperledger/caliper-fabric') !== '0.7.1') {
    throw new Error('@hyperledger/caliper-fabric 0.7.1 must be present in the locked Caliper dependency tree');
}

process.stdout.write('Caliper 0.7.1 and fabric:fabric-gateway binding are pinned.\n');
