'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');

const Ajv2020 = require('ajv/dist/2020');

// Conteos canonicos del estado logico inicial (seccion 4, #137/#141): ambos SUT
// deben partir de este mismo snapshot logico (seccion 5).
const CANONICAL_COUNTS = Object.freeze({
    total: 50_000,
    participating: 25_183,
    unregistered: 3_080,
    prepared: 22_103,
    filler: 24_817,
    registered: 46_920,
});
const SCHEMA_DIRECTORY = path.resolve(__dirname, '..', '..', 'schema');
const FABRIC_MANIFEST_SCHEMA = 'urn:pfi-snt:fabric-snapshot-manifest:schema:1.0.0';
const SHA256 = /^[0-9a-f]{64}$/u;

const ajv = new Ajv2020({allErrors: true, strict: false});
const validators = {
    fabricManifest: ajv.compile(JSON.parse(fs.readFileSync(path.join(SCHEMA_DIRECTORY, 'fabric-snapshot-manifest.schema.json'), 'utf8'))),
    seriesIndex: ajv.compile(JSON.parse(fs.readFileSync(path.join(SCHEMA_DIRECTORY, 'fabric-series-index.schema.json'), 'utf8'))),
};

function schemaErrors(validate, document) {
    if (validate(document)) return [];
    return (validate.errors ?? []).slice(0, 3).map((error) => `${error.instancePath || '/'} ${error.message}`);
}

function sha256File(filePath) {
    return crypto.createHash('sha256').update(fs.readFileSync(filePath)).digest('hex');
}

function readJSON(filePath) {
    return JSON.parse(fs.readFileSync(filePath, 'utf8'));
}

function countProblems(counts) {
    const problems = [];
    for (const [name, expected] of Object.entries(CANONICAL_COUNTS)) {
        // La baseline no declara `registered`: se deriva de total y ausentes.
        const actual = name === 'registered' && counts?.registered === undefined
            ? counts?.total - counts?.unregistered
            : counts?.[name];
        if (actual !== expected) problems.push(`${name} ${actual} (esperado ${expected})`);
    }
    return problems;
}

// Los archivos que acreditan el estado deben estar junto al manifiesto y
// coincidir con su hash: un manifiesto solo con IDs no prueba nada.
function fileProblems(directory, files, names) {
    const problems = [];
    for (const name of names) {
        const file = files?.[name];
        if (!file?.name || !SHA256.test(file.sha256 ?? '')) {
            problems.push(`falta files.${name}`);
            continue;
        }
        const filePath = path.join(directory, file.name);
        if (!fs.existsSync(filePath)) problems.push(`falta el archivo ${file.name}`);
        else if (sha256File(filePath) !== file.sha256) problems.push(`hash distinto en ${file.name}`);
    }
    return problems;
}

// Fabric: contrato estricto de #141, con verificacion semantica incluida.
function fabricManifestProblems(directory, manifest) {
    const problems = schemaErrors(validators.fabricManifest, manifest).map((error) => `schema: ${error}`);
    if (problems.length) return problems;
    return fileProblems(directory, manifest.files, ['logicalPlan', 'preconditions', 'semanticVerification']);
}

// Baseline (#139): no tiene schema publicado; se exige lo que acredita el estado.
function baselineManifestProblems(directory, manifest) {
    const problems = [];
    if (manifest.schemaVersion !== '1.0.0') problems.push('schemaVersion desconocida');
    if (!SHA256.test(manifest.snapshotId ?? '')) problems.push('snapshotId invalido');
    if (!SHA256.test(manifest.dataset?.sha256 ?? '')) problems.push('falta dataset.sha256');
    if (!/^[0-9a-f]{40}$/u.test(manifest.artifacts?.baselineCommit ?? '')) problems.push('falta artifacts.baselineCommit');
    if (!/@sha256:[0-9a-f]{64}$/u.test(manifest.artifacts?.baselineImage ?? '')) problems.push('falta un digest inmutable en artifacts.baselineImage');
    if (manifest.logicalPlanSHA256 !== manifest.files?.logicalPlan?.sha256) problems.push('logicalPlanSHA256 no coincide con files.logicalPlan');
    problems.push(...fileProblems(directory, manifest.files, ['logicalPlan', 'preconditions', 'dump']));
    return problems;
}

// Manifiestos de snapshot provistos explicitamente. Sin manifiesto acreditado
// no hay forma de saber de que estado inicial partio una ronda.
function loadSnapshotManifests(directories) {
    const byId = new Map();
    const problems = [];
    for (const directory of directories) {
        const manifestPath = path.join(directory, 'manifest.json');
        if (!fs.existsSync(manifestPath)) {
            problems.push(`${directory}: falta manifest.json`);
            continue;
        }
        const manifest = readJSON(manifestPath);
        const sut = manifest.$schema === FABRIC_MANIFEST_SCHEMA ? 'fabric' : 'baseline';
        const entryProblems = sut === 'fabric'
            ? fabricManifestProblems(directory, manifest)
            : baselineManifestProblems(directory, manifest);
        const counts = countProblems(manifest.counts);
        if (counts.length) entryProblems.push(`conteos no canonicos: ${counts.join(', ')}`);
        byId.set(manifest.snapshotId, {
            id: manifest.snapshotId,
            sut,
            directory,
            manifestSHA256: sha256File(manifestPath),
            datasetSHA256: manifest.dataset?.sha256,
            logicalPlanSHA256: manifest.files?.logicalPlan?.sha256,
            problems: entryProblems,
        });
    }
    return {byId, problems};
}

function snapshotContextOf(runDirectory) {
    const contextPath = path.join(runDirectory, 'snapshot-context.json');
    if (!fs.existsSync(contextPath)) return undefined;
    const context = readJSON(contextPath);
    // Fabric (#141) y baseline (#139) usan formas distintas del mismo dato.
    return {
        id: context.snapshotId ?? context.snapshot?.id,
        manifestSHA256: context.manifestSHA256 ?? context.snapshot?.manifestSHA256,
        logicalPlanSHA256: context.logicalPlanSHA256 ?? context.snapshot?.logicalPlanSHA256,
        verifiedPreconditions: context.verifiedPreconditions,
    };
}

// La verificacion previa a la ronda debe cubrir todas las unidades que la ronda
// tocó; si no, el estado inicial de esas unidades no esta acreditado.
function verificationProblems(context, operations) {
    if (!Array.isArray(context.verifiedPreconditions) || context.verifiedPreconditions.length === 0) {
        return 'sin verifiedPreconditions: la verificacion previa no esta acreditada';
    }
    const verified = new Set(context.verifiedPreconditions.map((entry) => entry?.sequence));
    const missing = [...new Set((operations ?? []).map((operation) => operation.datasetSequence)
        .filter((sequence) => Number.isInteger(sequence) && !verified.has(sequence)))];
    if (missing.length) return `${missing.length} unidades de la ronda sin verificacion previa (por ejemplo ${missing[0]})`;
    return undefined;
}

// El indice de serie esta en la raiz de la serie; la ronda puede estar varios
// niveles abajo (<serie>/<escenario>/measurements/run-NN/round).
function findSeriesIndex(runDirectory, roots) {
    let current = path.resolve(runDirectory);
    const stops = new Set(roots.map((root) => path.dirname(path.resolve(root))));
    while (!stops.has(current) && current !== path.dirname(current)) {
        const candidate = path.join(current, 'series-index.json');
        if (fs.existsSync(candidate)) return {path: candidate, root: current, index: readJSON(candidate)};
        current = path.dirname(current);
    }
    return undefined;
}

function attemptFor(series, runDirectory) {
    const relative = path.relative(series.root, path.resolve(runDirectory)).split(path.sep).join('/');
    for (const scenario of series.index.scenarios ?? []) {
        for (const attempt of scenario.attempts ?? []) {
            if (relative === attempt.directory || relative.startsWith(`${attempt.directory}/`)) return attempt;
        }
    }
    return undefined;
}

// Acredita una ronda medida. Exige, para cada SUT, la huella del manifiesto en
// el contexto, un manifiesto provisto que cumpla su contrato, la verificacion
// previa de todas las unidades de la ronda y, si hay indice, que cumpla su
// schema y declare el mismo snapshot y manifiesto. Devuelve el motivo de
// rechazo o, si la ronda solo falla por no ser citable, lo marca aparte.
function accreditRun(run, {manifests, roots}) {
    const context = snapshotContextOf(run.directory);
    if (!context?.id) return {reason: 'sin snapshot-context.json: estado inicial no acreditado'};
    if (!SHA256.test(context.manifestSHA256 ?? '')) return {reason: 'el contexto no registra el hash del manifiesto'};
    const manifest = manifests.byId.get(context.id);
    if (!manifest) return {reason: `manifiesto del snapshot ${context.id.slice(0, 12)} no provisto (--snapshot)`};
    if (manifest.sut !== run.sut) return {reason: `el snapshot ${context.id.slice(0, 12)} pertenece a ${manifest.sut}`};
    if (manifest.problems.length) return {reason: `snapshot ${context.id.slice(0, 12)}: ${manifest.problems.join('; ')}`};
    if (context.manifestSHA256 !== manifest.manifestSHA256) {
        return {reason: 'el manifiesto provisto no coincide con el registrado por la ronda'};
    }
    if (context.logicalPlanSHA256 && context.logicalPlanSHA256 !== manifest.logicalPlanSHA256) {
        return {reason: 'el plan logico del contexto no coincide con el del manifiesto'};
    }
    if (manifest.datasetSHA256 !== run.datasetSHA256) return {reason: 'el snapshot corresponde a otro dataset'};
    const verification = verificationProblems(context, run.raw);
    if (verification) return {reason: verification};
    const snapshot = {id: manifest.id, manifestSHA256: manifest.manifestSHA256, logicalPlanSHA256: manifest.logicalPlanSHA256};
    const series = findSeriesIndex(run.directory, roots);
    if (!series) return {snapshot, nonCitable: 'sin indice de serie'};
    const indexErrors = schemaErrors(validators.seriesIndex, series.index);
    if (indexErrors.length) return {reason: `indice de serie invalido: ${indexErrors.join('; ')}`};
    if (series.index.snapshot.id !== context.id) return {reason: 'la serie declara otro snapshot'};
    if (series.index.snapshot.manifestSHA256 !== manifest.manifestSHA256) {
        return {reason: 'la serie declara otro manifiesto del snapshot'};
    }
    const attempt = attemptFor(series, run.directory);
    if (!attempt) return {reason: 'la ronda no figura en el indice de su serie'};
    if (attempt.kind !== 'measurement') return {reason: `intento ${attempt.kind} en el indice`};
    if (attempt.repetition !== run.repetition) return {reason: 'la repeticion no coincide con el indice'};
    if (attempt.status !== 'completed') return {reason: `intento ${attempt.status} en el indice${attempt.reason ? `: ${attempt.reason}` : ''}`};
    const seriesInfo = {token: series.index.seriesToken, plan: series.index.plan, citable: series.index.citable,
        status: series.index.status, indexPath: series.path};
    if (!series.index.citable) return {snapshot, series: seriesInfo, nonCitable: `serie ${series.index.plan} no citable`};
    return {snapshot, series: seriesInfo};
}

module.exports = {
    CANONICAL_COUNTS,
    FABRIC_MANIFEST_SCHEMA,
    accreditRun,
    countProblems,
    findSeriesIndex,
    loadSnapshotManifests,
    snapshotContextOf,
};
