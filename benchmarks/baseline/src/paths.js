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

function createOutputDirectory(kind, explicitToken) {
    const directory = path.join(buildRoot, kind, safeToken(explicitToken));
    fs.mkdirSync(path.dirname(directory), {recursive: true});
    fs.mkdirSync(directory, {recursive: false});
    return directory;
}

module.exports = {buildRoot, createOutputDirectory, repoRoot, safeToken};
