'use strict';

function pairs(arguments_) {
    const values = {};
    for (let index = 0; index < arguments_.length; index += 2) {
        const name = arguments_[index];
        const value = arguments_[index + 1];
        if (!name?.startsWith('--') || value === undefined || value.startsWith('--')) {
            throw new Error('arguments must be supplied as --name value pairs');
        }
        const key = name.slice(2);
        if (values[key] !== undefined) throw new Error(`duplicate argument --${key}`);
        values[key] = value;
    }
    return values;
}

function assertSupported(values, supported) {
    for (const key of Object.keys(values)) {
        if (!supported.has(key)) throw new Error(`unsupported argument --${key}`);
    }
}

function parseRoundArguments(arguments_) {
    const values = pairs(arguments_);
    assertSupported(values, new Set([
        'scenario', 'phase', 'repetition', 'rate', 'family', 'operation',
        'dataset-dir', 'snapshot-dir', 'run-token',
    ]));
    for (const required of ['scenario', 'phase', 'repetition', 'rate', 'snapshot-dir']) {
        if (!values[required]) throw new Error(`--${required} is required`);
    }
    return {
        scenario: values.scenario,
        phase: values.phase,
        repetition: values.repetition,
        rate: values.rate,
        family: values.family,
        operation: values.operation,
        datasetDirectory: values['dataset-dir'],
        snapshotDirectory: values['snapshot-dir'],
        runToken: values['run-token'],
    };
}

function parseSnapshotArguments(arguments_, requireSnapshot = false) {
    const values = pairs(arguments_);
    assertSupported(values, new Set(['dataset-dir', 'snapshot-dir', 'snapshot-token', 'run-token']));
    if (requireSnapshot && !values['snapshot-dir']) throw new Error('--snapshot-dir is required');
    return {
        datasetDirectory: values['dataset-dir'],
        snapshotDirectory: values['snapshot-dir'],
        snapshotToken: values['snapshot-token'],
        runToken: values['run-token'],
    };
}

module.exports = {parseRoundArguments, parseSnapshotArguments, pairs};
