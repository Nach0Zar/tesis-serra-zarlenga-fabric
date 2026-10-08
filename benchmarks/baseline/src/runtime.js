'use strict';

const fs = require('node:fs');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

function run(command, args, options = {}) {
    const result = spawnSync(command, args, {
        cwd: options.cwd,
        env: options.env ?? process.env,
        encoding: options.binary ? null : 'utf8',
        input: options.input,
        maxBuffer: 512 * 1024 * 1024,
        stdio: options.inherit ? 'inherit' : undefined,
    });
    if (result.error) throw result.error;
    if (result.status !== 0 && !options.allowFailure) {
        const detail = options.binary ? '' : String(result.stderr || result.stdout || '').trim();
        throw new Error(`${command} ${args.join(' ')} failed${detail ? `: ${detail}` : ''}`);
    }
    return result;
}

function benchmarkEnvironment(environment = process.env) {
    const apiPort = environment.SNT_BASELINE_API_PORT ?? '18080';
    const dbPort = environment.SNT_BASELINE_DB_PORT ?? '15432';
    if (!/^\d+$/u.test(apiPort) || !/^\d+$/u.test(dbPort)) throw new Error('baseline benchmark ports must be numeric');
    return {
        ...environment,
        COMPOSE_PROJECT_NAME: environment.SNT_BASELINE_BENCHMARK_PROJECT ?? 'snt_baseline_benchmark',
        SNT_BASELINE_API_PORT: apiPort,
        SNT_BASELINE_DB_PORT: dbPort,
        SNT_BASELINE_IMAGE: environment.SNT_BASELINE_IMAGE ?? 'snt-baseline:local',
    };
}

function composeArguments(repoRoot, args) {
    return ['compose', '-f', path.join(repoRoot, 'baseline', 'compose.yaml'), ...args];
}

function compose(repoRoot, args, options = {}) {
    return run('docker', composeArguments(repoRoot, args), {
        cwd: path.join(repoRoot, 'baseline'), env: options.env ?? benchmarkEnvironment(), ...options,
    });
}

function ensureFabricStopped(repoRoot, environment) {
    const result = run('docker', [
        'compose', '--project-name', 'snt-fabric', '-f', path.join(repoRoot, 'network', 'compose.yaml'),
        'ps', '--status', 'running', '-q',
    ], {cwd: path.join(repoRoot, 'network'), env: environment, allowFailure: true});
    if (result.status === 0 && result.stdout.trim() !== '') {
        throw new Error('Fabric containers are running; the protocol requires non-concurrent SUT execution');
    }
}

function postgresIdentifiers(environment) {
    return {
        user: environment.SNT_BASELINE_DB_USER ?? 'snt_baseline',
        database: environment.SNT_BASELINE_DB_NAME ?? 'snt_baseline',
    };
}

function dumpDatabase(repoRoot, environment) {
    const {user, database} = postgresIdentifiers(environment);
    return compose(repoRoot, [
        'exec', '-T', 'postgres', 'pg_dump', '-U', user, '-d', database,
        '--format=custom', '--no-owner', '--no-privileges',
    ], {env: environment, binary: true}).stdout;
}

function restoreDatabase(repoRoot, environment, dump) {
    const {user, database} = postgresIdentifiers(environment);
    compose(repoRoot, ['stop', 'api'], {env: environment});
    compose(repoRoot, ['exec', '-T', 'postgres', 'dropdb', '-U', user, '--if-exists', '--force', database], {env: environment});
    compose(repoRoot, ['exec', '-T', 'postgres', 'createdb', '-U', user, database], {env: environment});
    compose(repoRoot, [
        'exec', '-T', 'postgres', 'pg_restore', '-U', user, '-d', database, '--no-owner', '--no-privileges',
    ], {env: environment, binary: true, input: dump});
    compose(repoRoot, ['up', '-d', '--wait', 'api'], {env: environment});
}

function queryDatabase(repoRoot, environment, sql) {
    const {user, database} = postgresIdentifiers(environment);
    return compose(repoRoot, [
        'exec', '-T', 'postgres', 'psql', '-U', user, '-d', database, '-At', '-v', 'ON_ERROR_STOP=1', '-c', sql,
    ], {env: environment}).stdout.trim();
}

function inspectRuntime(repoRoot, environment) {
    const composeConfig = JSON.parse(compose(repoRoot, ['config', '--format', 'json'], {env: environment}).stdout);
    const networkConfig = JSON.parse(run('docker', [
        'compose', '--project-name', 'snt-fabric', '-f', path.join(repoRoot, 'network', 'compose.yaml'),
        'config', '--format', 'json',
    ], {cwd: path.join(repoRoot, 'network'), env: environment}).stdout);
    const image = environment.SNT_BASELINE_IMAGE;
    const imageID = run('docker', ['image', 'inspect', '--format', '{{.Id}}', image], {env: environment}).stdout.trim();
    const postgresImage = composeConfig.services?.postgres?.image;
    const fabricImage = networkConfig.services?.['peer0.lab.snt.local']?.image;
    return {
        docker: run('docker', ['version', '--format', '{{.Server.Version}}'], {env: environment}).stdout.trim(),
        dockerCompose: run('docker', ['compose', 'version', '--short'], {env: environment}).stdout.trim(),
        postgres: String(postgresImage).replace(/^postgres:/u, ''),
        fabric: String(fabricImage).replace(/^hyperledger\/fabric-peer:/u, ''),
        baselineImage: `${image}@${imageID}`,
    };
}

function repositoryCommit(repoRoot) {
    return run('git', ['rev-parse', 'HEAD'], {cwd: repoRoot}).stdout.trim();
}

function writePrivateFile(filePath, contents) {
    fs.writeFileSync(filePath, contents, {mode: 0o600});
}

module.exports = {
    benchmarkEnvironment, compose, dumpDatabase, ensureFabricStopped, inspectRuntime,
    postgresIdentifiers, queryDatabase, repositoryCommit, restoreDatabase, run, writePrivateFile,
};
