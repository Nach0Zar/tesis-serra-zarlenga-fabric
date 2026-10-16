'use strict';

const fs = require('node:fs');
const path = require('node:path');

const {verifyUnitPrecondition} = require('./snapshot');

function readPreconditions(snapshot) {
    const fileName = snapshot.manifest.files.preconditions.name;
    const entries = JSON.parse(fs.readFileSync(path.join(snapshot.directory, fileName), 'utf8'));
    if (!Array.isArray(entries) || entries.length !== snapshot.manifest.dataset.units) {
        throw new Error('snapshot preconditions do not cover the complete dataset');
    }
    const bySequence = new Map();
    for (const entry of entries) {
        if (!Number.isInteger(entry?.sequence) || bySequence.has(entry.sequence) || !Array.isArray(entry.history)) {
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

function unitReference(dataset, sequence) {
    const unit = dataset.units.find((entry) => entry.sequence === sequence);
    const registration = unit?.preparation?.[0];
    if (registration?.operation !== 'RegisterUnit') {
        throw new Error(`dataset sequence ${sequence} has no canonical registration`);
    }
    return {
        gtin: registration.request.gtin,
        numeroSerie: registration.request.numeroSerie,
        invokerMspId: registration.invokerMspId,
    };
}

async function verifyPreconditions(client, dataset, preconditions, sequences) {
    const verified = [];
    const units = new Map(dataset.units.map((unit) => [unit.sequence, unit]));
    for (const sequence of sequences) {
        if (!preconditions.has(sequence)) throw new Error(`snapshot omits precondition for sequence ${sequence}`);
        const expected = preconditions.get(sequence);
        await verifyUnitPrecondition(client, units.get(sequence), expected);
        verified.push({sequence, present: expected.unit !== null});
    }
    return verified;
}

module.exports = {readPreconditions, sequencesForPlan, unitReference, verifyPreconditions};
