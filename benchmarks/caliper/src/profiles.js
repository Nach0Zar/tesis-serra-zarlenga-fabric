'use strict';

const PROFILES = Object.freeze({
    'write-register': {operation: 'register', workers: 2, durationSeconds: 120, rates: [5, 10, 20], module: 'register-unit'},
    'write-transfer': {operation: 'transfer', workers: 2, durationSeconds: 120, rates: [5, 10, 20], module: 'transfer-unit'},
    'write-dispense': {operation: 'dispense', workers: 2, durationSeconds: 120, rates: [5, 10, 20], module: 'dispense-unit'},
    'read-unit': {operation: 'query-unit', workers: 4, durationSeconds: 120, rates: [10, 25, 50], module: 'read-unit'},
    'read-history': {operation: 'query-history', workers: 4, durationSeconds: 120, rates: [10, 25, 50], module: 'unit-history'},
    mixed: {
        operation: 'mixed', workers: 2, durationSeconds: 120, rates: [20], module: 'mixed',
        mix: {register: 10, transfer: 55, dispense: 10, query: 25},
    },
    'expected-rejections': {operation: undefined, workers: 2, durationSeconds: 60, rates: [5], module: 'expected-rejection'},
});

const REJECTION_FAMILIES = new Set(['UNAUTHORIZED_TRANSFER', 'DUPLICATE_IDENTITY', 'BLOCKING_STATE']);

function buildProfile(options) {
    const base = PROFILES[options.scenario];
    if (!base) {
        throw new Error(`unsupported scenario ${options.scenario}`);
    }
    const rate = Number(options.rate);
    if (!base.rates.includes(rate)) {
        throw new Error(`${options.scenario} rate must be one of ${base.rates.join(', ')}`);
    }
    if (!['warmup', 'measurement'].includes(options.phase)) {
        throw new Error('phase must be warmup or measurement');
    }
    const repetition = Number(options.repetition);
    if (!Number.isInteger(repetition)
        || (options.phase === 'warmup' && repetition !== 0)
        || (options.phase === 'measurement' && (repetition < 1 || repetition > 8))) {
        throw new Error('warmup requires repetition 0; measurement requires repetition 1..8');
    }

    let operation = base.operation;
    if (options.scenario === 'expected-rejections') {
        if (!REJECTION_FAMILIES.has(options.family)) {
            throw new Error('expected-rejections requires a supported --family');
        }
        if (options.family === 'BLOCKING_STATE') {
            if (!['transfer', 'dispense'].includes(options.operation)) {
                throw new Error('BLOCKING_STATE requires --operation transfer or dispense');
            }
            operation = options.operation;
        } else {
            if (options.operation !== undefined) {
                throw new Error('--operation is only valid for BLOCKING_STATE');
            }
            operation = options.family === 'DUPLICATE_IDENTITY' ? 'register' : 'transfer';
        }
    } else if (options.family !== undefined || options.operation !== undefined) {
        throw new Error('--family and --operation only apply to expected-rejections');
    }

    const transactionsPerOperation = operation === 'transfer' ? 2 : operation === 'mixed' ? 1.55 : 1;
    return Object.freeze({
        ...base,
        scenario: options.scenario,
        phase: options.phase,
        repetition,
        rate,
        operation,
        rejectionFamily: options.family,
        transactionsPerOperation: options.scenario === 'expected-rejections' ? 1 : transactionsPerOperation,
    });
}

module.exports = {PROFILES, REJECTION_FAMILIES, buildProfile};
