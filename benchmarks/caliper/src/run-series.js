#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const readline = require('node:readline');
const {spawn} = require('node:child_process');

const {writeJSONAtomic} = require('./metadata');
const {PROFILES, buildProfile} = require('./profiles');

const BASE_REPETITIONS = 5;
const EXTENSION_REPETITIONS = 3;
const MAX_REPETITION = 8;
const CV_THRESHOLD = 0.15;
const REJECTION_SPECS = Object.freeze([
    {family: 'UNAUTHORIZED_TRANSFER'},
    {family: 'DUPLICATE_IDENTITY'},
    {family: 'BLOCKING_STATE', operation: 'transfer'},
    {family: 'BLOCKING_STATE', operation: 'dispense'},
]);
const RESET_STEPS = Object.freeze([
    ['down', 'docker', ['compose', '-f', 'network/compose.yaml', 'down', '--volumes']],
    ['cleanup', 'bash', ['-c', 'ids=$(docker ps -aq --filter name=^dev-peer); [ -z "$ids" ] || docker rm -f $ids >/dev/null']],
    ['up', './network/network.sh', ['up']],
    ['createChannel', './network/network.sh', ['createChannel']],
    ['deployCC', './network/network.sh', ['deployCC']],
    ['verify', './network/network.sh', ['verify']],
]);

function scenarioKey(spec) {
    return [spec.scenario, spec.rate, spec.family, spec.operation].filter((part) => part !== undefined).join('-');
}

// Orden de las secciones 6.2 a 6.5 de docs/measurement-protocol.md: 20 escenarios.
function defaultPlan() {
    const specs = [];
    for (const [scenario, profile] of Object.entries(PROFILES)) {
        for (const rate of profile.rates) {
            if (scenario === 'expected-rejections') {
                for (const rejection of REJECTION_SPECS) specs.push({scenario, rate, ...rejection});
            } else {
                specs.push({scenario, rate});
            }
        }
    }
    return specs;
}

// Formato: escenario:tasa[:familia[:operación]], separados por coma.
function parseSpecs(value) {
    return value.split(',').map((text) => {
        const [scenario, rate, family, operation] = text.trim().split(':');
        const spec = {scenario, rate: Number(rate)};
        if (family) spec.family = family;
        if (operation) spec.operation = operation;
        buildProfile({...spec, phase: 'warmup', repetition: 0});
        return spec;
    });
}

function parseArguments(arguments_) {
    const options = {repetitions: BASE_REPETITIONS, maxAttempts: 2, dryRun: false};
    for (let index = 0; index < arguments_.length; index += 1) {
        const name = arguments_[index];
        if (name === '--dry-run') {
            options.dryRun = true;
            continue;
        }
        const value = arguments_[index + 1];
        if (value === undefined || value.startsWith('--')) throw new Error(`${name} requires a value`);
        index += 1;
        if (name === '--only') options.specs = parseSpecs(value);
        else if (name === '--repetitions') options.repetitions = Number(value);
        else if (name === '--max-attempts') options.maxAttempts = Number(value);
        else if (name === '--series-token') options.seriesToken = value;
        else if (name === '--dataset-dir') options.datasetDirectory = value;
        else throw new Error(`unsupported argument ${name}`);
    }
    if (!Number.isInteger(options.repetitions) || options.repetitions < 1 || options.repetitions > BASE_REPETITIONS) {
        throw new Error(`--repetitions must be between 1 and ${BASE_REPETITIONS}`);
    }
    if (!Number.isInteger(options.maxAttempts) || options.maxAttempts < 1) {
        throw new Error('--max-attempts must be a positive integer');
    }
    options.specs ??= defaultPlan();
    return options;
}

// Sección 7: desvío estándar muestral (n - 1) entre repeticiones dividido por su media.
function coefficientOfVariation(values) {
    if (values.length < 2) return undefined;
    const mean = values.reduce((sum, value) => sum + value, 0) / values.length;
    if (mean === 0) return undefined;
    const variance = values.reduce((sum, value) => sum + ((value - mean) ** 2), 0) / (values.length - 1);
    return Math.sqrt(variance) / mean;
}

function roundMetrics(summary) {
    const completed = summary.scenario === 'expected-rejections' ? summary.expectedRejections : summary.successfulOperations;
    return {
        throughput: completed / summary.observedDurationSeconds,
        p95Ms: summary.operationLatency?.p95Ms,
    };
}

function seriesStatistics(metrics) {
    return {
        repetitions: metrics.length,
        throughputCV: coefficientOfVariation(metrics.map((metric) => metric.throughput)),
        p95CV: coefficientOfVariation(metrics.map((metric) => metric.p95Ms)),
    };
}

function needsExtension(statistics) {
    return [statistics.throughputCV, statistics.p95CV].some((value) => value !== undefined && value > CV_THRESHOLD);
}

class Series {
    constructor(repoRoot, options) {
        this.repoRoot = repoRoot;
        this.options = options;
        this.token = options.seriesToken ?? `series-${new Date().toISOString().replace(/[:.]/gu, '-')}`;
        if (!/^[A-Za-z0-9._-]{1,60}$/u.test(this.token)) throw new Error('--series-token contains unsupported characters');
        this.caliperDirectory = path.join(repoRoot, 'benchmarks', 'caliper');
        this.directory = path.join(repoRoot, 'build', 'benchmarks', 'series', this.token);
        fs.mkdirSync(path.dirname(this.directory), {recursive: true});
        fs.mkdirSync(this.directory, {recursive: false});
        this.logPath = path.join(this.directory, 'series.log');
        this.indexPath = path.join(this.directory, 'index.jsonl');
        this.report = {token: this.token, startedAt: new Date().toISOString(), options, scenarios: []};
    }

    log(message) {
        const line = `${new Date().toISOString()} ${message}`;
        fs.appendFileSync(this.logPath, `${line}\n`);
        process.stdout.write(`${line}\n`);
    }

    // Ejecuta un comando volcando su salida a un archivo; devuelve el estado y,
    // si se pide, el instante de la primera línea que cumple `marker`.
    exec(command, args, logFile, {env, marker, cwd} = {}) {
        return new Promise((resolve) => {
            const startedAt = Date.now();
            const output = fs.createWriteStream(logFile, {flags: 'a'});
            const child = spawn(command, args, {cwd: cwd ?? this.repoRoot, env: {...process.env, ...env}});
            let markerAt;
            let lastLine = '';
            for (const stream of [child.stdout, child.stderr]) {
                readline.createInterface({input: stream}).on('line', (line) => {
                    output.write(`${line}\n`);
                    if (line.trim()) lastLine = line.trim();
                    if (marker && markerAt === undefined && marker.test(line)) markerAt = Date.now();
                });
            }
            child.on('close', (status) => {
                output.end();
                resolve({status, seconds: (Date.now() - startedAt) / 1000, markerAt, startedAt, lastLine});
            });
        });
    }

    async reset(attemptDirectory, timings) {
        const logFile = path.join(attemptDirectory, 'reset.log');
        for (const [name, command, args] of RESET_STEPS) {
            fs.appendFileSync(logFile, `=== ${name}\n`);
            const result = await this.exec(command, args, logFile);
            timings[name] = result.seconds;
            if (result.status !== 0) return `reinicio falló en ${name} (código ${result.status})`;
        }
        return undefined;
    }

    async attempt(spec, phase, repetition, attemptNumber) {
        const key = scenarioKey(spec);
        const label = `${phase === 'warmup' ? 'warmup' : `run-${String(repetition).padStart(2, '0')}`}-a${attemptNumber}`;
        const attemptDirectory = path.join(this.directory, key, label);
        fs.mkdirSync(attemptDirectory, {recursive: true});
        const runToken = `${this.token}-${key}-${label}`;
        const record = {
            scenario: spec.scenario, rate: spec.rate, family: spec.family, operation: spec.operation,
            phase, repetition, attempt: attemptNumber, runToken, startedAt: new Date().toISOString(), timings: {},
            directory: path.relative(this.repoRoot, attemptDirectory),
        };
        const finish = (fields) => {
            Object.assign(record, fields, {endedAt: new Date().toISOString()});
            record.timings.total = Object.values(record.timings).reduce((sum, value) => sum + value, 0);
            fs.appendFileSync(this.indexPath, `${JSON.stringify(record)}\n`);
            return record;
        };

        this.log(`${key} ${label}: reinicio con ledger limpio`);
        const resetFailure = await this.reset(attemptDirectory, record.timings);
        if (resetFailure) return finish({status: 'discarded', reason: resetFailure});

        const resources = await this.exec('network/scripts/measure-resources.sh', [],
            path.join(attemptDirectory, 'resources-idle.txt'));
        record.timings.resources = resources.seconds;
        if (resources.status !== 0) {
            return finish({status: 'discarded', reason: 'measure-resources.sh rechazó el estado de los contenedores'});
        }

        this.log(`${key} ${label}: smoke`);
        const smoke = await this.exec('npm', ['run', 'smoke'], path.join(attemptDirectory, 'smoke.log'), {
            cwd: this.caliperDirectory, env: {SNT_CALIPER_RUN_TOKEN: `${runToken}-smoke`},
        });
        record.timings.smoke = smoke.seconds;
        if (smoke.status !== 0) return finish({status: 'smoke-failed', reason: `smoke falló: ${smoke.lastLine}`});

        this.log(`${key} ${label}: ronda`);
        const roundArgs = ['run', 'round', '--', '--scenario', spec.scenario, '--phase', phase,
            '--repetition', String(repetition), '--rate', String(spec.rate), '--run-token', runToken];
        if (spec.family) roundArgs.push('--family', spec.family);
        if (spec.operation) roundArgs.push('--operation', spec.operation);
        if (this.options.datasetDirectory) roundArgs.push('--dataset-dir', this.options.datasetDirectory);
        const round = await this.exec('npm', roundArgs, path.join(attemptDirectory, 'round.log'), {
            cwd: this.caliperDirectory, marker: /\[caliper\]/u,
        });
        const caliperStartedAt = round.markerAt ?? round.startedAt + round.seconds * 1000;
        record.timings.preparation = (caliperStartedAt - round.startedAt) / 1000;
        record.timings.caliper = round.seconds - record.timings.preparation;

        const runDirectory = path.join(this.repoRoot, 'build', 'benchmarks', 'caliper', runToken);
        record.runDirectory = path.relative(this.repoRoot, runDirectory);
        const metadataPath = path.join(runDirectory, 'metadata.json');
        if (!fs.existsSync(metadataPath)) {
            return finish({status: 'discarded', reason: `la ronda no produjo metadata.json: ${round.lastLine}`});
        }
        // runmeta resuelve el contrato con una ruta relativa a client/.
        const validation = await this.exec(this.runmeta, [metadataPath], path.join(attemptDirectory, 'runmeta.log'), {
            cwd: path.join(this.repoRoot, 'client'),
        });
        record.timings.validation = validation.seconds;
        record.metadataValid = validation.status === 0;
        const summary = JSON.parse(fs.readFileSync(path.join(runDirectory, 'summary.json'), 'utf8'));
        const metadata = JSON.parse(fs.readFileSync(metadataPath, 'utf8'));
        record.summary = {
            operations: summary.operationCount,
            successful: summary.successfulOperations,
            expectedRejections: summary.expectedRejections,
            unexpectedFailures: summary.unexpectedFailures,
            retryAttempts: summary.retryAttempts,
            observedErrorCodes: summary.observedErrorCodes,
            offeredOperationsPerSecond: summary.rate.offeredOperationsPerSecond,
            observedDurationSeconds: summary.observedDurationSeconds,
            p50Ms: summary.operationLatency?.p50Ms,
            p99Ms: summary.operationLatency?.p99Ms,
        };
        record.metrics = roundMetrics(summary);
        const reason = metadata.discarded?.reason
            ?? (record.metadataValid ? undefined : 'metadata.json no valida contra el contrato')
            ?? (round.status === 0 ? undefined : `run-round terminó con código ${round.status}: ${round.lastLine}`);
        return finish(reason ? {status: 'discarded', reason} : {status: 'valid'});
    }

    async repetition(spec, phase, repetition) {
        for (let attemptNumber = 1; attemptNumber <= this.options.maxAttempts; attemptNumber += 1) {
            const record = await this.attempt(spec, phase, repetition, attemptNumber);
            this.log(`${scenarioKey(spec)} ${phase} ${repetition} intento ${attemptNumber}: ${record.status}`
                + `${record.reason ? ` (${record.reason})` : ''} en ${record.timings.total.toFixed(1)} s`);
            if (record.status === 'smoke-failed') {
                throw new Error(`smoke falló antes de ${scenarioKey(spec)} ${phase} ${repetition}; sección 6.1 impide medir`);
            }
            if (record.status === 'valid') return record;
            // El próximo reinicio elimina los contenedores: se conservan sus logs para diagnosticar el descarte.
            await this.exec('docker', ['compose', '-f', 'network/compose.yaml', 'logs', '--no-color', '--timestamps'],
                path.join(this.repoRoot, record.directory, 'sut-logs.txt'));
        }
        return undefined;
    }

    async scenario(spec) {
        const key = scenarioKey(spec);
        const entry = {key, ...spec, measured: [], missing: []};
        this.report.scenarios.push(entry);
        const warmup = await this.repetition(spec, 'warmup', 0);
        entry.warmup = warmup ? 'valid' : 'discarded';
        let target = this.options.repetitions;
        for (let repetition = 1; repetition <= target; repetition += 1) {
            const record = await this.repetition(spec, 'measurement', repetition);
            if (record) entry.measured.push({repetition, runToken: record.runToken, ...record.metrics});
            else entry.missing.push(repetition);
            if (repetition === BASE_REPETITIONS && target === BASE_REPETITIONS) {
                entry.original = seriesStatistics(entry.measured);
                if (needsExtension(entry.original)) {
                    target = Math.min(MAX_REPETITION, BASE_REPETITIONS + EXTENSION_REPETITIONS);
                    this.log(`${key}: CV supera ${CV_THRESHOLD * 100} %, se agregan ${EXTENSION_REPETITIONS} repeticiones`);
                }
            }
            this.writeReport();
        }
        entry.final = seriesStatistics(entry.measured);
        entry.extended = target > BASE_REPETITIONS;
        this.log(`${key}: ${entry.measured.length} válidas, CV throughput=${formatCV(entry.final.throughputCV)}`
            + ` CV p95=${formatCV(entry.final.p95CV)}`);
        this.writeReport();
    }

    writeReport() {
        writeJSONAtomic(path.join(this.directory, 'series.json'), {...this.report, updatedAt: new Date().toISOString()});
    }

    async run() {
        this.log(`serie ${this.token}: ${this.options.specs.length} escenarios, ${this.options.repetitions} repeticiones base`);
        this.report.repositoryCommit = (await this.capture('git', ['rev-parse', 'HEAD'])).trim();
        this.runmeta = path.join(this.directory, 'runmeta');
        const build = await this.exec('go', ['build', '-o', this.runmeta, './cmd/runmeta'],
            path.join(this.directory, 'runmeta-build.log'), {cwd: path.join(this.repoRoot, 'client')});
        if (build.status !== 0) throw new Error('no se pudo compilar client/cmd/runmeta');
        for (const spec of this.options.specs) await this.scenario(spec);
        this.report.endedAt = new Date().toISOString();
        this.writeReport();
        this.log(`serie ${this.token} terminada: ${path.relative(this.repoRoot, this.directory)}`);
    }

    capture(command, args) {
        return new Promise((resolve, reject) => {
            let output = '';
            const child = spawn(command, args, {cwd: this.repoRoot});
            child.stdout.on('data', (chunk) => { output += chunk; });
            child.on('close', (status) => (status === 0 ? resolve(output) : reject(new Error(`${command} failed`))));
        });
    }
}

function formatCV(value) {
    return value === undefined ? 'n/d' : `${(value * 100).toFixed(2)} %`;
}

async function main() {
    const options = parseArguments(process.argv.slice(2));
    if (options.dryRun) {
        for (const spec of options.specs) process.stdout.write(`${scenarioKey(spec)}\n`);
        process.stdout.write(`${options.specs.length} escenarios, ${options.specs.length * (1 + options.repetitions)} rondas base\n`);
        return;
    }
    for (const variable of ['SNT_FABRIC_BIN_DIR', 'SNT_FABRIC_CFG_PATH']) {
        if (!process.env[variable]) throw new Error(`${variable} must be exported (see network/README.md)`);
    }
    const repoRoot = path.resolve(__dirname, '..', '..', '..');
    await new Series(repoRoot, options).run();
}

if (require.main === module) {
    main().catch((error) => {
        process.stderr.write(`Fabric series failed: ${error.message}\n`);
        process.exitCode = 1;
    });
}

module.exports = {
    CV_THRESHOLD,
    coefficientOfVariation,
    defaultPlan,
    needsExtension,
    parseArguments,
    roundMetrics,
    scenarioKey,
    seriesStatistics,
};
