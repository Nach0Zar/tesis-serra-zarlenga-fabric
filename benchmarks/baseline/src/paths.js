'use strict';

const fs = require('node:fs');
const path = require('node:path');

const repoRoot = path.resolve(__dirname, '..', '..', '..');
const buildRoot = path.join(repoRoot, 'build', 'benchmarks', 'baseline');

function safeToken(explicitToken) {
    const token = explicitToken ?? new Date().toISOString().replace(/[:.]/gu, '-');
    if (!/^[A-Za-z0-9._-]{1,100}$/u.test(token)) {
        throw new Error('token contains unsupported characters');
    }
    return token;
}

// outputRoot permite que una serie guarde cada ronda dentro de su propio
// directorio; debe quedar dentro de build/ del repositorio.
function createOutputDirectory(kind, explicitToken, outputRoot) {
    const root = outputRoot ? path.resolve(repoRoot, outputRoot) : path.join(buildRoot, kind);
    const relative = path.relative(path.join(repoRoot, 'build'), root);
    if (relative.startsWith('..') || path.isAbsolute(relative)) {
        throw new Error('--output-root must resolve inside the repository build directory');
    }
    const directory = path.join(root, safeToken(explicitToken));
    fs.mkdirSync(path.dirname(directory), {recursive: true});
    fs.mkdirSync(directory, {recursive: false});
    return directory;
}

module.exports = {buildRoot, createOutputDirectory, repoRoot, safeToken};
