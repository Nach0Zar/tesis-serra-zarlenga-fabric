#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const projectRoot = path.resolve(__dirname, '..');
const roots = ['src', 'workloads', 'rate-controllers', 'scripts', 'test'];
const files = [];

function visit(directory) {
    for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
        const candidate = path.join(directory, entry.name);
        if (entry.isDirectory()) {
            visit(candidate);
        } else if (entry.isFile() && candidate.endsWith('.js')) {
            files.push(candidate);
        }
    }
}

for (const root of roots) {
    visit(path.join(projectRoot, root));
}

for (const file of files.sort()) {
    const result = spawnSync(process.execPath, ['--check', file], {stdio: 'inherit'});
    if (result.status !== 0) {
        process.exit(result.status ?? 1);
    }
}
