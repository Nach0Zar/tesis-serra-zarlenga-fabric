'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const {sha256File} = require('./artifacts');

const COMPOSE_PROJECT = 'snt-fabric';
const VOLUMES = Object.freeze([
    {name: 'orderer-anmat-data', service: 'orderer.anmat.snt.local', containerPath: '/var/hyperledger/production/orderer'},
    {name: 'orderer-lab-data', service: 'orderer.lab.snt.local', containerPath: '/var/hyperledger/production/orderer'},
    {name: 'orderer-drogueria-data', service: 'orderer.drogueria.snt.local', containerPath: '/var/hyperledger/production/orderer'},
    {name: 'peer-anmat-data', service: 'peer0.anmat.snt.local', containerPath: '/var/hyperledger/production'},
    {name: 'peer-lab-data', service: 'peer0.lab.snt.local', containerPath: '/var/hyperledger/production'},
    {name: 'peer-drogueria-data', service: 'peer0.drogueria.snt.local', containerPath: '/var/hyperledger/production'},
    {name: 'peer-distribuidor-data', service: 'peer0.distribuidor.snt.local', containerPath: '/var/hyperledger/production'},
    {name: 'peer-farmacia-data', service: 'peer0.farmacia.snt.local', containerPath: '/var/hyperledger/production'},
    {name: 'peer-centromedico-data', service: 'peer0.centromedico.snt.local', containerPath: '/var/hyperledger/production'},
    {name: 'peer-financiador-data', service: 'peer0.financiador.snt.local', containerPath: '/var/hyperledger/production'},
]);

const CHAINCODE_CONTAINER = /^dev-peer0\.(?:anmat|lab|drogueria|distribuidor|farmacia|centromedico|financiador)\.snt\.local-snt_1\.0-/u;

function run(command, args, options = {}) {
    const result = (options.spawnSync ?? spawnSync)(command, args, {
        cwd: options.cwd,
        encoding: 'utf8',
        stdio: options.inherit ? 'inherit' : ['ignore', 'pipe', 'pipe'],
        env: options.env ?? process.env,
        maxBuffer: 16 * 1024 * 1024,
    });
    if (result.error) throw result.error;
    if (result.status !== 0 && !options.allowFailure) {
        throw new Error(`${command} ${args.join(' ')} failed: ${(result.stderr || result.stdout || '').trim()}`);
    }
    return result;
}

function compose(repoRoot, args, options = {}) {
    return run('docker', ['compose', '-f', path.join(repoRoot, 'network', 'compose.yaml'), ...args], options);
}

function containerId(repoRoot, service, options = {}) {
    const value = compose(repoRoot, ['ps', '-aq', service], options).stdout.trim().split(/\r?\n/u).filter(Boolean);
    if (value.length !== 1) throw new Error(`expected exactly one container for ${service}, got ${value.length}`);
    return value[0];
}

function listTree(rootDirectory) {
    const entries = [];
    function visit(directory) {
        for (const entry of fs.readdirSync(directory, {withFileTypes: true}).sort((a, b) => a.name.localeCompare(b.name))) {
            const absolute = path.join(directory, entry.name);
            const relative = path.relative(rootDirectory, absolute).split(path.sep).join('/');
            const stat = fs.lstatSync(absolute);
            if (entry.isDirectory()) {
                entries.push({path: `${relative}/`, type: 'directory'});
                visit(absolute);
            } else if (entry.isSymbolicLink()) {
                entries.push({path: relative, type: 'symlink', target: fs.readlinkSync(absolute)});
            } else if (entry.isFile()) {
                entries.push({path: relative, type: 'file', bytes: stat.size, sha256: sha256File(absolute)});
            } else {
                throw new Error(`unsupported volume entry type: ${relative}`);
            }
        }
    }
    visit(rootDirectory);
    return entries;
}

function treeDigest(rootDirectory) {
    const entries = listTree(rootDirectory);
    return {
        sha256: crypto.createHash('sha256').update(JSON.stringify(entries)).digest('hex'),
        fileCount: entries.filter((entry) => entry.type !== 'directory').length,
    };
}

function temporaryDirectory(prefix) {
    return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

function exportVolume(repoRoot, snapshotDirectory, volume, options = {}) {
    const work = temporaryDirectory(`snt-volume-${volume.name}-`);
    const contents = path.join(work, 'contents');
    fs.mkdirSync(contents);
    try {
        const id = containerId(repoRoot, volume.service, options);
        run('docker', ['cp', `${id}:${volume.containerPath}/.`, contents], options);
        const tree = treeDigest(contents);
        if (tree.fileCount === 0) throw new Error(`volume ${volume.name} is empty`);
        const volumesDirectory = path.join(snapshotDirectory, 'volumes');
        fs.mkdirSync(volumesDirectory, {recursive: true});
        const archivePath = path.join(volumesDirectory, `${volume.name}.tar`);
        run('tar', [
            '--sort=name', '--mtime=@0', '--owner=0', '--group=0', '--numeric-owner',
            '-C', contents, '-cf', archivePath, '.',
        ], options);
        return {
            ...volume,
            archive: path.relative(snapshotDirectory, archivePath).split(path.sep).join('/'),
            sha256: sha256File(archivePath),
            treeSHA256: tree.sha256,
            bytes: fs.statSync(archivePath).size,
            fileCount: tree.fileCount,
        };
    } finally {
        fs.rmSync(work, {recursive: true, force: true});
    }
}

function assertVolumeManifest(volumes) {
    if (!Array.isArray(volumes) || volumes.length !== VOLUMES.length) {
        throw new Error(`snapshot must contain exactly ${VOLUMES.length} Fabric volumes`);
    }
    const expected = new Map(VOLUMES.map((entry) => [entry.name, entry]));
    for (const volume of volumes) {
        const canonical = expected.get(volume.name);
        if (!canonical || canonical.service !== volume.service || canonical.containerPath !== volume.containerPath) {
            throw new Error(`snapshot contains an unexpected volume mapping: ${volume.name ?? 'unknown'}`);
        }
        expected.delete(volume.name);
    }
    if (expected.size !== 0) throw new Error(`snapshot omits volumes: ${[...expected.keys()].join(', ')}`);
}

function removeManagedChaincodeContainers(options = {}) {
    const result = run('docker', ['ps', '-a', '--format', '{{.ID}}\t{{.Names}}'], options);
    const ids = result.stdout.trim().split(/\r?\n/u).filter(Boolean).flatMap((line) => {
        const [id, name] = line.split('\t');
        return CHAINCODE_CONTAINER.test(name) ? [id] : [];
    });
    if (ids.length > 0) run('docker', ['rm', '-f', ...ids], options);
    return ids;
}

function saveVolumes(repoRoot, snapshotDirectory, options = {}) {
    compose(repoRoot, ['stop'], options);
    const descriptors = VOLUMES.map((volume) => exportVolume(repoRoot, snapshotDirectory, volume, options));
    removeManagedChaincodeContainers(options);
    return descriptors;
}

function restoreOneVolume(repoRoot, snapshotDirectory, volume, options = {}) {
    const archivePath = path.resolve(snapshotDirectory, volume.archive);
    const relative = path.relative(snapshotDirectory, archivePath);
    if (relative.startsWith('..') || path.isAbsolute(relative)) throw new Error(`volume archive escapes snapshot: ${volume.archive}`);
    if (sha256File(archivePath) !== volume.sha256 || fs.statSync(archivePath).size !== volume.bytes) {
        throw new Error(`volume archive integrity mismatch: ${volume.name}`);
    }
    const work = temporaryDirectory(`snt-restore-${volume.name}-`);
    const contents = path.join(work, 'contents');
    const verified = path.join(work, 'verified');
    fs.mkdirSync(contents);
    fs.mkdirSync(verified);
    try {
        run('tar', ['-C', contents, '-xf', archivePath], options);
        const before = treeDigest(contents);
        if (before.sha256 !== volume.treeSHA256 || before.fileCount !== volume.fileCount) {
            throw new Error(`volume tree digest mismatch before restore: ${volume.name}`);
        }
        const id = containerId(repoRoot, volume.service, options);
        run('docker', ['cp', `${contents}/.`, `${id}:${volume.containerPath}`], options);
        run('docker', ['cp', `${id}:${volume.containerPath}/.`, verified], options);
        const after = treeDigest(verified);
        if (after.sha256 !== volume.treeSHA256 || after.fileCount !== volume.fileCount) {
            throw new Error(`volume tree digest mismatch after restore: ${volume.name}`);
        }
    } finally {
        fs.rmSync(work, {recursive: true, force: true});
    }
}

function restoreVolumes(repoRoot, snapshotDirectory, volumes, options = {}) {
    assertVolumeManifest(volumes);
    removeManagedChaincodeContainers(options);
    compose(repoRoot, ['down', '--volumes', '--remove-orphans'], {...options, allowFailure: true});
    compose(repoRoot, ['create'], options);
    for (const volume of volumes) restoreOneVolume(repoRoot, snapshotDirectory, volume, options);
    compose(repoRoot, ['up', '--detach', '--wait', '--wait-timeout', '180'], options);
}

module.exports = {
    CHAINCODE_CONTAINER,
    COMPOSE_PROJECT,
    VOLUMES,
    assertVolumeManifest,
    compose,
    containerId,
    exportVolume,
    listTree,
    removeManagedChaincodeContainers,
    restoreOneVolume,
    restoreVolumes,
    run,
    saveVolumes,
    treeDigest,
};
