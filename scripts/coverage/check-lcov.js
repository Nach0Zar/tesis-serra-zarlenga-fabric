#!/usr/bin/env node
'use strict';

// Verifica la cobertura de lineas de la logica de un paquete Node a partir del
// lcov que emite `node --test --experimental-test-coverage`. El alcance y el
// minimo se declaran en el campo `coverage` del package.json del paquete:
// `include` lista los archivos de logica y `excluded` explica por que cada
// archivo de infraestructura queda fuera. Uso: check-lcov.js <lcov>

const fs = require('node:fs');
const path = require('node:path');

function parseLcov(contents) {
    const files = new Map();
    let current;
    for (const line of contents.split(/\r?\n/u)) {
        if (line.startsWith('SF:')) {
            current = {found: 0, hit: 0};
            files.set(path.resolve(line.slice(3)), current);
        } else if (line.startsWith('LF:')) {
            current.found = Number(line.slice(3));
        } else if (line.startsWith('LH:')) {
            current.hit = Number(line.slice(3));
        }
    }
    return files;
}

function main() {
    const lcovPath = process.argv[2];
    if (!lcovPath) throw new Error('uso: check-lcov.js <lcov>');
    const manifest = JSON.parse(fs.readFileSync(path.resolve('package.json'), 'utf8'));
    const scope = manifest.coverage;
    if (!scope || !Array.isArray(scope.include) || typeof scope.minimum !== 'number') {
        throw new Error('package.json debe declarar coverage.minimum y coverage.include');
    }
    const files = parseLcov(fs.readFileSync(lcovPath, 'utf8'));
    let found = 0;
    let hit = 0;
    for (const file of scope.include) {
        const entry = files.get(path.resolve(file));
        if (!entry) throw new Error(`${file} no aparece en ${lcovPath}: ningun test lo carga`);
        found += entry.found;
        hit += entry.hit;
        process.stdout.write(`${file.padEnd(40)} ${(100 * entry.hit / entry.found).toFixed(1).padStart(5)} %\n`);
    }
    const total = 100 * hit / found;
    process.stdout.write(`Cobertura de la logica: ${total.toFixed(1)} % (${hit}/${found} lineas, minimo ${scope.minimum} %)\n`);
    if (total < scope.minimum) {
        process.stderr.write(`error: la cobertura ${total.toFixed(1)} % es menor que el minimo ${scope.minimum} %\n`);
        process.exitCode = 1;
    }
}

try {
    main();
} catch (error) {
    process.stderr.write(`error: ${error.message}\n`);
    process.exitCode = 2;
}
