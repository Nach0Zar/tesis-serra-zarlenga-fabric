'use strict';

const {percentile} = require('./stats');

const STABLE_SECONDS = 30;
const RECOVERY_SHARE = 0.95;

function operationTimes(operation) {
    return {started: Date.parse(operation.startedAt), ended: Date.parse(operation.endedAt)};
}

function succeeded(operation) {
    return operation.outcome === 'success';
}

function windowSummary(operations, from, to) {
    const inside = operations.filter((operation) => {
        const {started} = operationTimes(operation);
        return started >= from && started < to;
    });
    const successes = inside.filter(succeeded);
    const seconds = Math.max((to - from) / 1000, Number.EPSILON);
    return {
        startedAt: new Date(from).toISOString(),
        endedAt: new Date(to).toISOString(),
        seconds,
        operations: inside.length,
        successes: successes.length,
        successRate: inside.length ? successes.length / inside.length : undefined,
        throughput: successes.length / seconds,
        p95Ms: percentile(successes.map((operation) => operation.latencyMs), 0.95),
        failuresByCode: inside.filter((operation) => !succeeded(operation)).reduce((codes, operation) => {
            const failed = (operation.transactions ?? []).filter((transaction) => transaction.status === 'failed').at(-1);
            const code = failed?.errorCode ?? failed?.transportError ?? 'sin código';
            codes[code] = (codes[code] ?? 0) + 1;
            return codes;
        }, {}),
    };
}

// Tiempo de recuperacion (seccion 3.3): desde la inyeccion hasta el primer tramo
// estable de 30 s cuya tasa de exito vuelve al 95 % de la previa a la falla.
function recoveryTime(operations, injectedAt, preFaultRate, endAt) {
    if (!Number.isFinite(preFaultRate)) return undefined;
    const starts = operations.map((operation) => operationTimes(operation).started)
        .filter((started) => started >= injectedAt).sort((left, right) => left - right);
    for (const candidate of starts) {
        const to = candidate + STABLE_SECONDS * 1000;
        if (to > endAt) break;
        const stretch = windowSummary(operations, candidate, to);
        if (stretch.operations > 0 && stretch.successRate >= RECOVERY_SHARE * preFaultRate) {
            return (candidate - injectedAt) / 1000;
        }
    }
    return undefined;
}

// En las caidas de peer (seccion 8.1) la ronda mezcla operaciones que necesitan
// el peer caido y operaciones de control: el agregado global ocultaria el 0 % de
// la clase afectada. Cada operacion del crudo debe declarar su clase.
const PEER_FAULT_SCENARIOS = new Set(['peer-1a', 'peer-1b', 'peer-1c', 'peer-1d']);
const AVAILABILITY_CLASSES = Object.freeze(['target', 'control']);

function classWindows(run, operations) {
    return availabilityWindowsFor({...run, raw: operations});
}

function availabilityWindows(run) {
    const result = availabilityWindowsFor(run);
    if (!result || !PEER_FAULT_SCENARIOS.has(run.scenario)) return result;
    const operations = run.raw ?? [];
    const unclassified = operations.filter((operation) => !AVAILABILITY_CLASSES.includes(operation.availabilityClass));
    if (unclassified.length > 0) {
        return {
            ...result,
            blocked: `${unclassified.length} operaciones sin availabilityClass (target/control): no se informa disponibilidad global sola`,
        };
    }
    result.byClass = Object.fromEntries(AVAILABILITY_CLASSES.map((name) => [
        name, classWindows(run, operations.filter((operation) => operation.availabilityClass === name)),
    ]));
    return result;
}

function availabilityWindowsFor(run) {
    const fault = run.faultInjection;
    const operations = run.raw ?? [];
    if (!fault || operations.length === 0) return undefined;
    const injectedAt = Date.parse(fault.injectedAt);
    const startAt = injectedAt - fault.preFaultSeconds * 1000;
    const recoveredAt = fault.recoveredAt ? Date.parse(fault.recoveredAt) : undefined;
    const faultEnd = recoveredAt ?? injectedAt + fault.faultSeconds * 1000;
    const endAt = recoveredAt && fault.recoverySeconds
        ? recoveredAt + fault.recoverySeconds * 1000
        : Math.max(...operations.map((operation) => operationTimes(operation).ended));
    const preFault = windowSummary(operations, startAt, injectedAt);
    const windows = {
        preFault,
        fault: windowSummary(operations, injectedAt, faultEnd),
        recovery: recoveredAt ? windowSummary(operations, recoveredAt, endAt) : undefined,
    };
    return {
        target: fault.target,
        injectedAt: fault.injectedAt,
        recoveredAt: fault.recoveredAt,
        windows,
        recoverySeconds: recoveryTime(operations, injectedAt, preFault.successRate, endAt),
    };
}

// Serie temporal de la tasa de exito por intervalo, para el grafico.
function successTimeline(run, bucketSeconds = 5) {
    const operations = run.raw ?? [];
    if (operations.length === 0) return [];
    const starts = operations.map((operation) => operationTimes(operation).started);
    const origin = run.faultInjection ? Date.parse(run.faultInjection.injectedAt) : Math.min(...starts);
    const first = Math.min(...starts);
    const last = Math.max(...starts);
    const buckets = [];
    const firstIndex = Math.floor((first - origin) / (bucketSeconds * 1000));
    const lastIndex = Math.floor((last - origin) / (bucketSeconds * 1000));
    for (let index = firstIndex; index <= lastIndex; index += 1) {
        const from = origin + index * bucketSeconds * 1000;
        const summary = windowSummary(operations, from, from + bucketSeconds * 1000);
        if (summary.operations > 0) {
            buckets.push({
                offsetSeconds: index * bucketSeconds,
                midpointSeconds: (index + 0.5) * bucketSeconds,
                successRate: summary.successRate,
            });
        }
    }
    return buckets;
}

module.exports = {AVAILABILITY_CLASSES, PEER_FAULT_SCENARIOS, RECOVERY_SHARE, STABLE_SECONDS, availabilityWindows, recoveryTime, successTimeline, windowSummary};
