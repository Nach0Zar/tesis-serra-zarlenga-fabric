'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');

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
const SERIES_INDEX_SCHEMA = 'urn:pfi-snt:fabric-series-index:schema:1.0.0';

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
        const counts = countProblems(manifest.counts);
        const entry = {
            id: manifest.snapshotId,
            directory,
            manifestSHA256: sha256File(manifestPath),
            datasetSHA256: manifest.dataset?.sha256,
            logicalPlanSHA256: manifest.files?.logicalPlan?.sha256,
            problems: counts.length ? [`conteos no canonicos: ${counts.join(', ')}`] : [],
        };
        if (!/^[0-9a-f]{64}$/u.test(entry.id ?? '')) entry.problems.push('snapshotId invalido');
        byId.set(entry.id, entry);
    }
    return {byId, problems};
}

function snapshotContextOf(runDirectory) {
    for (const candidate of [path.join(runDirectory, 'snapshot-context.json')]) {
        if (!fs.existsSync(candidate)) continue;
        const context = readJSON(candidate);
        // Fabric (#141) y baseline (#139) usan formas distintas del mismo dato.
        return {
            id: context.snapshotId ?? context.snapshot?.id,
            manifestSHA256: context.manifestSHA256 ?? context.snapshot?.manifestSHA256,
            counts: context.counts,
        };
    }
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

function seriesProblems(index) {
    const problems = [];
    if (index.$schema !== SERIES_INDEX_SCHEMA) problems.push('schema de indice desconocido');
    if (!['full', 'trial'].includes(index.plan)) problems.push(`plan ${index.plan} desconocido`);
    if ((index.plan === 'full') !== (index.citable === true)) problems.push('citable no corresponde al plan');
    if (!Array.isArray(index.scenarios)) problems.push('indice sin escenarios');
    return problems;
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

// Acredita una ronda medida: snapshot canonico provisto, mismo dataset, y un
// intento `measurement` completado de una serie citable. Devuelve el motivo de
// exclusion o, si la ronda solo falla por no ser citable, lo marca aparte para
// que `--allow-noncitable` pueda admitirla en un reporte rotulado.
function accreditRun(run, {manifests, roots}) {
    const context = snapshotContextOf(run.directory);
    if (!context?.id) return {reason: 'sin snapshot-context.json: estado inicial no acreditado'};
    const manifest = manifests.byId.get(context.id);
    if (!manifest) return {reason: `manifiesto del snapshot ${context.id.slice(0, 12)} no provisto (--snapshot)`};
    if (manifest.problems.length) return {reason: `snapshot ${context.id.slice(0, 12)}: ${manifest.problems.join('; ')}`};
    if (context.manifestSHA256 && context.manifestSHA256 !== manifest.manifestSHA256) {
        return {reason: 'el manifiesto provisto no coincide con el registrado por la ronda'};
    }
    if (manifest.datasetSHA256 && manifest.datasetSHA256 !== run.datasetSHA256) {
        return {reason: 'el snapshot corresponde a otro dataset'};
    }
    const snapshot = {id: manifest.id, manifestSHA256: manifest.manifestSHA256, logicalPlanSHA256: manifest.logicalPlanSHA256};
    const series = findSeriesIndex(run.directory, roots);
    if (!series) return {snapshot, nonCitable: 'sin indice de serie'};
    const problems = seriesProblems(series.index);
    if (problems.length) return {reason: `indice de serie invalido: ${problems.join('; ')}`};
    if (series.index.snapshot?.id !== context.id) return {reason: 'la serie declara otro snapshot'};
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
    accreditRun,
    countProblems,
    findSeriesIndex,
    loadSnapshotManifests,
    snapshotContextOf,
};
