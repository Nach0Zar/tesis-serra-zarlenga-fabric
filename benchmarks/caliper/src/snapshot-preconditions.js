'use strict';

const fs = require('node:fs');
const path = require('node:path');

const {equalUnit, readProgress, stableHistory} = require('./snapshot-state');

function readPreconditions(snapshot) {
    const filePath = path.join(snapshot.directory, snapshot.manifest.files.preconditions.name);
    const entries = JSON.parse(fs.readFileSync(filePath, 'utf8'));
    if (!Array.isArray(entries) || entries.length !== snapshot.manifest.dataset.units) {
        throw new Error('snapshot preconditions do not cover the complete dataset');
    }
    const bySequence = new Map();
    for (const entry of entries) {
        if (!Number.isInteger(entry?.sequence) || bySequence.has(entry.sequence)
            || !Array.isArray(entry.history)) {
            throw new Error('snapshot preconditions contain an invalid or duplicate sequence');
        }
        bySequence.set(entry.sequence, entry);
    }
    return bySequence;
}

function sequencesForPlan(plan, smokeSequence) {
    const sequences = new Set([smokeSequence]);
    for (const preparation of plan.preparations) sequences.add(preparation.datasetSequence);
    for (const worker of plan.workers) {
        for (const operation of worker.operations) sequences.add(operation.datasetSequence);
    }
    return [...sequences].sort((left, right) => left - right);
}

async function verifyPlanPreconditions(gatewayPool, dataset, preconditions, sequences) {
    const unitBySequence = new Map(dataset.units.map((unit) => [unit.sequence, unit]));
    const verified = [];
    for (const sequence of sequences) {
        const unit = unitBySequence.get(sequence);
        const expected = preconditions.get(sequence);
        if (!unit || !expected) throw new Error(`snapshot omits canonical precondition for sequence ${sequence}`);
        const progress = await readProgress(gatewayPool, unit);
        if (expected.unit === null) {
            if (progress.present) throw new Error(`sequence ${sequence} should be absent before the round`);
        } else {
            if (!progress.present || !equalUnit(progress.current, expected.unit)) {
                throw new Error(`sequence ${sequence} has a contaminated public precondition`);
            }
            if (JSON.stringify(stableHistory(progress.history)) !== JSON.stringify(expected.history)) {
                throw new Error(`sequence ${sequence} has a contaminated history precondition`);
            }
        }
        verified.push({sequence, present: expected.unit !== null});
    }
    return verified;
}

module.exports = {readPreconditions, sequencesForPlan, verifyPlanPreconditions};
