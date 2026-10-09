'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const timers = require('node:timers/promises');
const {spawn, spawnSync} = require('node:child_process');

const {resolveOrganizationCredentials} = require('./credentials');
const {extractContractError} = require('./contract-errors');
const {GatewayPool} = require('./gateway-bridge');
const {writeJSONAtomic} = require('./metadata');
const {
    buildMultiOrganizationNetworkConfig, inspectOrganizationRuntime, resolveComposeVersions,
} = require('./network-config');
const {readJSON, sha256File} = require('./sources');

const SNAPSHOT_SCHEMA_VERSION = '1.0.0';
const CANONICAL_DATASET_SHA256 = 'de523ee8fafeed0a39f8518b501fce4692192869030762816de353654385163d';
const CHANNEL = 'snt-channel';
const COMPOSE_PROJECT = 'snt-fabric';
const LEDGER_VOLUMES = Object.freeze([
    'orderer-anmat-data', 'orderer-lab-data', 'orderer-drogueria-data',
    'peer-anmat-data', 'peer-lab-data', 'peer-drogueria-data', 'peer-distribuidor-data',
    'peer-farmacia-data', 'peer-centromedico-data', 'peer-financiador-data',
]);
// Archivos de red que fijan el canal, la politica y las colecciones: si cambian,
// el ledger guardado deja de describir la red que se mide.
const NETWORK_SOURCES = Object.freeze([
    'network/configtx.yaml', 'network/compose.yaml', 'network/collections_config.json',
    'network/organizations-manifest.json', 'network/chaincode-package.lock',
]);

function run(command, args, options = {}) {
    const result = spawnSync(command, args, {
        cwd: options.cwd, encoding: options.encoding ?? 'utf8', input: options.input,
        stdio: options.inherit ? 'inherit' : ['pipe', 'pipe', 'pipe'],
        maxBuffer: 64 * 1024 * 1024,
    });
    if (result.error) throw result.error;
    if (result.status !== 0 && !options.allowFailure) {
        throw new Error(`${command} ${args.join(' ')} failed: ${String(result.stderr || result.stdout || '').trim()}`);
    }
    return result;
}

function compose(repoRoot, args, options = {}) {
    return run('docker', ['compose', '-f', path.join(repoRoot, 'network', 'compose.yaml'), ...args], {cwd: repoRoot, ...options});
}

function sha256(value) {
    return crypto.createHash('sha256').update(value).digest('hex');
}

function listFiles(directory) {
    const files = [];
    for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
        const fullPath = path.join(directory, entry.name);
        if (entry.isDirectory()) files.push(...listFiles(fullPath));
        else if (entry.isFile()) files.push(fullPath);
    }
    return files;
}

// Huella de las identidades: el bloque genesis y el registro de organizaciones
// guardados en el ledger solo son validos con este mismo material. Excluye el
// estado interno de la CA, que cambia sin alterar ningun certificado.
function cryptoFingerprint(repoRoot) {
    const root = path.join(repoRoot, 'network', 'organizations');
    const lines = [];
    for (const entry of fs.readdirSync(root, {withFileTypes: true})) {
        if (!entry.isDirectory() || entry.name.startsWith('.')) continue;
        for (const file of listFiles(path.join(root, entry.name))) {
            lines.push(`${path.relative(root, file)} ${sha256File(file)}`);
        }
    }
    if (lines.length === 0) throw new Error('network/organizations has no generated identities');
    return sha256(lines.sort().join('\n'));
}

function networkFingerprint(repoRoot) {
    const files = Object.fromEntries(NETWORK_SOURCES.map((file) => [file, sha256File(path.join(repoRoot, file))]));
    return {files, crypto: cryptoFingerprint(repoRoot)};
}

function readArtifacts(repoRoot, sourceTruth, runtime) {
    return {
        contractVersion: sourceTruth.contractVersion,
        packageID: sourceTruth.chaincode.packageID,
        fabric: runtime.fabric,
        fabricCA: runtime.fabricCA,
        network: networkFingerprint(repoRoot),
    };
}

// Lo que ata un snapshot a la red que se mide (D2): contrato, package, version de
// Fabric, dataset y la red misma. El commit del repositorio queda solo como dato.
function compareArtifacts(expected, actual) {
    const differences = [];
    for (const field of ['contractVersion', 'packageID', 'fabric', 'fabricCA']) {
        if (expected[field] !== actual[field]) differences.push(`${field}: ${expected[field]} != ${actual[field]}`);
    }
    for (const [file, hash] of Object.entries(expected.network.files)) {
        if (actual.network.files[file] !== hash) differences.push(`${file} changed`);
    }
    if (expected.network.crypto !== actual.network.crypto) differences.push('network/organizations identities changed');
    return differences;
}

// Versiones declaradas por Compose: no requiere la red en ejecucion, a
// diferencia de inspectOrganizationRuntime, que consulta los puertos publicados.
function composeRuntime(repoRoot) {
    return {
        docker: run('docker', ['version', '--format', '{{.Server.Version}}']).stdout.trim(),
        dockerCompose: run('docker', ['compose', 'version', '--short']).stdout.trim(),
        ...resolveComposeVersions(JSON.parse(compose(repoRoot, ['config', '--format', 'json']).stdout)),
    };
}

function connectAllOrganizations(repoRoot, sourceTruth, caliperVersion) {
    const manifest = readJSON(path.join(repoRoot, 'network', 'organizations-manifest.json'));
    const mspIds = manifest.organizations.filter((entry) => entry.active).map((entry) => entry.mspId);
    const credentials = resolveOrganizationCredentials(repoRoot, mspIds);
    const runtime = inspectOrganizationRuntime(repoRoot, credentials);
    const networkConfig = buildMultiOrganizationNetworkConfig({
        organizations: credentials, endpoints: runtime.endpoints, sourceTruth, versions: runtime, caliperVersion,
    });
    return {pool: new GatewayPool(networkConfig), runtime, mspIds, networkConfig};
}

function volumeName(volume) {
    return `${COMPOSE_PROJECT}_${volume}`;
}

function peerImage(runtime) {
    return `hyperledger/fabric-peer:${runtime.fabric}`;
}

function hostOwner() {
    return `${process.getuid()}:${process.getgid()}`;
}

// Guarda los volumenes con la red detenida: orderers y peers quedan en el mismo
// punto del ledger y la restauracion reproduce un cluster Raft consistente.
function saveVolumes(repoRoot, directory, runtime) {
    compose(repoRoot, ['stop']);
    const volumes = [];
    try {
        for (const volume of LEDGER_VOLUMES) {
            const name = `${volume}.tar`;
            run('docker', [
                'run', '--rm', '-v', `${volumeName(volume)}:/data:ro`, '-v', `${directory}:/backup`,
                '--entrypoint', 'sh', peerImage(runtime), '-c',
                `tar --sort=name -C /data -cf /backup/${name} . && chown ${hostOwner()} /backup/${name}`,
            ]);
            const filePath = path.join(directory, name);
            volumes.push({volume, name, sha256: sha256File(filePath), bytes: fs.statSync(filePath).size});
        }
    } finally {
        compose(repoRoot, ['up', '--detach', '--wait', '--wait-timeout', '180']);
    }
    return volumes;
}

function readSnapshot(snapshotDirectory) {
    const directory = path.resolve(snapshotDirectory);
    const manifest = readJSON(path.join(directory, 'manifest.json'));
    if (manifest.schemaVersion !== SNAPSHOT_SCHEMA_VERSION) throw new Error('unsupported Fabric snapshot manifest version');
    if (manifest.dataset?.sha256 !== CANONICAL_DATASET_SHA256) throw new Error('snapshot is not bound to the canonical dataset');
    const preconditionsPath = path.join(directory, manifest.files.preconditions.name);
    if (sha256File(preconditionsPath) !== manifest.files.preconditions.sha256) {
        throw new Error('snapshot preconditions hash mismatch');
    }
    return {directory, manifest};
}

function verifyVolumeArchives(snapshot) {
    for (const entry of snapshot.manifest.files.volumes) {
        if (sha256File(path.join(snapshot.directory, entry.name)) !== entry.sha256) {
            throw new Error(`snapshot volume hash mismatch: ${entry.name}`);
        }
    }
}

function removeChaincodeContainers() {
    const ids = run('docker', ['ps', '-aq', '--filter', 'name=^dev-peer']).stdout.trim().split(/\s+/u).filter(Boolean);
    if (ids.length > 0) run('docker', ['rm', '-f', ...ids]);
}

// Primer endoso de cada peer fuera de la ventana medida (D4): tras restaurar,
// los contenedores de chaincode arrancan en frio.
async function warmChaincode(pool, mspIds, reference, timeoutMs = 180_000) {
    const deadline = Date.now() + timeoutMs;
    const warmed = {};
    for (const mspId of mspIds) {
        const started = Date.now();
        for (;;) {
            try {
                await pool.connection(mspId).contract.evaluateTransaction('ReadUnit', reference.gtin, reference.numeroSerie);
                break;
            } catch (error) {
                const envelope = extractContractError(error);
                if (envelope) throw new Error(`warm-up ReadUnit on ${mspId} failed with ${envelope.code}`);
                if (Date.now() >= deadline) throw error;
                await timers.setTimeout(1000);
            }
        }
        warmed[mspId] = (Date.now() - started) / 1000;
    }
    return warmed;
}

function runAsync(command, args) {
    return new Promise((resolve, reject) => {
        const child = spawn(command, args, {stdio: ['ignore', 'ignore', 'pipe']});
        let stderr = '';
        child.stderr.on('data', (chunk) => { stderr += chunk; });
        child.on('error', reject);
        child.on('close', (status) => (status === 0
            ? resolve()
            : reject(new Error(`${command} ${args.join(' ')} failed: ${stderr.trim()}`))));
    });
}

// Los diez volumenes son independientes: se extraen en paralelo.
async function restoreVolumes(repoRoot, snapshot, runtime) {
    compose(repoRoot, ['down', '--volumes', '--remove-orphans']);
    removeChaincodeContainers();
    compose(repoRoot, ['create']);
    await Promise.all(snapshot.manifest.files.volumes.map((entry) => runAsync('docker', [
        'run', '--rm', '-v', `${volumeName(entry.volume)}:/data`, '-v', `${snapshot.directory}:/backup:ro`,
        '--entrypoint', 'tar', peerImage(runtime), '-C', '/data', '-xf', `/backup/${entry.name}`,
    ])));
    compose(repoRoot, ['up', '--detach', '--wait', '--wait-timeout', '180']);
}

async function restoreSnapshot({repoRoot, snapshotDirectory, sourceTruth, caliperVersion, smokeReference}) {
    const timings = {};
    let started = Date.now();
    const snapshot = readSnapshot(snapshotDirectory);
    const runtime = composeRuntime(repoRoot);
    const differences = compareArtifacts(snapshot.manifest.artifacts, readArtifacts(repoRoot, sourceTruth, runtime));
    if (differences.length > 0) {
        throw new Error(`snapshot does not match the current network: ${differences.join('; ')}`);
    }
    verifyVolumeArchives(snapshot);
    timings.verifyArchives = (Date.now() - started) / 1000;

    started = Date.now();
    await restoreVolumes(repoRoot, snapshot, runtime);
    timings.restoreVolumes = (Date.now() - started) / 1000;

    started = Date.now();
    const warm = connectAllOrganizations(repoRoot, sourceTruth, caliperVersion);
    try {
        timings.warmChaincodeByMsp = await warmChaincode(warm.pool, warm.mspIds, smokeReference);
    } finally {
        warm.pool.close();
    }
    timings.warmChaincode = (Date.now() - started) / 1000;
    return {snapshot, timings};
}

function writeSnapshotManifest(directory, manifest) {
    writeJSONAtomic(path.join(directory, 'manifest.json'), manifest);
}

module.exports = {
    CANONICAL_DATASET_SHA256,
    CHANNEL,
    LEDGER_VOLUMES,
    SNAPSHOT_SCHEMA_VERSION,
    compareArtifacts,
    compose,
    composeRuntime,
    connectAllOrganizations,
    cryptoFingerprint,
    readArtifacts,
    readSnapshot,
    restoreSnapshot,
    run,
    saveVolumes,
    sha256,
    verifyVolumeArchives,
    warmChaincode,
    writeSnapshotManifest,
};
