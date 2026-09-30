'use strict';

const {spawnSync} = require('node:child_process');
const path = require('node:path');

const IDENTITY_NAME = 'lab-user1';

function run(command, args, options = {}) {
    const result = spawnSync(command, args, {
        cwd: options.cwd,
        encoding: 'utf8',
        stdio: ['ignore', 'pipe', 'pipe'],
    });
    if (result.error) {
        throw result.error;
    }
    if (result.status !== 0) {
        throw new Error(`${command} ${args.join(' ')} failed: ${(result.stderr || result.stdout).trim()}`);
    }
    return result.stdout.trim();
}

function normalizePublishedEndpoint(value) {
    const endpoint = value.trim().split(/\r?\n/u)[0];
    const ipv6 = endpoint.match(/^\[(.*)\]:(\d+)$/u);
    const ipv4 = endpoint.match(/^([^:]+):(\d+)$/u);
    const port = ipv6?.[2] ?? ipv4?.[2];
    let host = ipv6?.[1] ?? ipv4?.[1];
    if (!host || !port) {
        throw new Error(`unexpected Docker Compose port output: ${value}`);
    }
    if (['0.0.0.0', '127.0.0.1', '::', '::1'].includes(host)) {
        host = 'localhost';
    }
    return `${host}:${port}`;
}

function parseImageVersion(image, repository) {
    const prefix = `${repository}:`;
    if (typeof image !== 'string' || !image.startsWith(prefix) || image.slice(prefix.length) === '') {
        throw new Error(`cannot resolve ${repository} version from Docker Compose`);
    }
    return image.slice(prefix.length);
}

function resolveComposeVersions(composeDocument) {
    const services = composeDocument.services ?? {};
    return {
        fabric: parseImageVersion(services['peer0.lab.snt.local']?.image, 'hyperledger/fabric-peer'),
        fabricCA: parseImageVersion(services['fabric-ca.snt.local']?.image, 'hyperledger/fabric-ca'),
    };
}

function inspectRuntime(repoRoot, peerHostname) {
    const composePath = path.join(repoRoot, 'network', 'compose.yaml');
    const networkDirectory = path.dirname(composePath);
    const composeArgs = ['compose', '-f', composePath];
    const endpoint = normalizePublishedEndpoint(run('docker', [
        ...composeArgs,
        'port',
        peerHostname,
        '7051',
    ], {cwd: networkDirectory}));
    const composeDocument = JSON.parse(run('docker', [
        ...composeArgs,
        'config',
        '--format',
        'json',
    ], {cwd: networkDirectory}));
    const versions = resolveComposeVersions(composeDocument);

    return {
        endpoint,
        docker: run('docker', ['version', '--format', '{{.Server.Version}}']),
        dockerCompose: run('docker', ['compose', 'version', '--short']),
        ...versions,
    };
}

function buildNetworkConfig({credentials, endpoint, sourceTruth, versions, caliperVersion}) {
    return {
        name: 'SNT Fabric local network',
        version: '2.0.0',
        caliper: {
            blockchain: 'fabric',
            sutOptions: {mutualTls: false},
        },
        info: {
            ContractVersion: sourceTruth.contractVersion,
            PackageID: sourceTruth.chaincode.packageID,
            FabricVersion: versions.fabric,
            FabricCAVersion: versions.fabricCA,
            CaliperVersion: caliperVersion,
            Channel: 'snt-channel',
            Chaincode: 'snt',
        },
        channels: [{
            channelName: 'snt-channel',
            contracts: [{id: 'snt'}],
        }],
        organizations: [{
            mspid: credentials.mspId,
            identities: {
                certificates: [{
                    name: IDENTITY_NAME,
                    clientPrivateKey: {path: credentials.privateKeyPath},
                    clientSignedCert: {path: credentials.certificatePath},
                }],
            },
            peers: [{
                endpoint,
                tlsCACerts: {path: credentials.tlsCACertPath},
                grpcOptions: {
                    'ssl-target-name-override': credentials.peerHostname,
                },
            }],
        }],
    };
}

module.exports = {
    IDENTITY_NAME,
    buildNetworkConfig,
    inspectRuntime,
    normalizePublishedEndpoint,
    resolveComposeVersions,
};
