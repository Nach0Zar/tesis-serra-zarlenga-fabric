'use strict';

// Escenarios de disponibilidad de Fabric (protocolo, seccion 8.1; EVAL-4, #44).
//
// Cada escenario corre la carga mixta de referencia (seccion 6.4, 20
// operaciones conceptuales/s con el ciclo 2/11/2/5) y, desde fuera de los
// workloads, da de baja uno o dos contenedores. Los fallos son simulados a nivel
// de contenedor, en un unico host: miden la tolerancia a la caida de nodos
// asignados a organizaciones distintas, no la caida fisica ni administrativa de
// una organizacion completa (ADR-007).

const PRE_FAULT_SECONDS = 60;
const FAULT_SECONDS = 60;
const RECOVERY_SECONDS = 60;
const ROUND_SECONDS = PRE_FAULT_SECONDS + FAULT_SECONDS + RECOVERY_SECONDS;

// Orden de los consenters en network/configtx.yaml: es el `node=N` de los logs
// de etcdraft.
const ORDERERS = Object.freeze([
    {node: 1, organization: 'AnmatMSP', container: 'orderer.anmat.snt.local'},
    {node: 2, organization: 'LabMSP', container: 'orderer.lab.snt.local'},
    {node: 3, organization: 'DrogueriaMSP', container: 'orderer.drogueria.snt.local'},
]);

const PEERS = Object.freeze({
    AnmatMSP: 'peer0.anmat.snt.local',
    LabMSP: 'peer0.lab.snt.local',
    DrogueriaMSP: 'peer0.drogueria.snt.local',
    DistribuidorMSP: 'peer0.distribuidor.snt.local',
    FarmaciaMSP: 'peer0.farmacia.snt.local',
    CentroMedicoMSP: 'peer0.centromedico.snt.local',
    FinanciadorMSP: 'peer0.financiador.snt.local',
});

// Con el backoff por defecto de grpc-js, un cliente que perdio la conexion con
// un peer tardaba unos 9 s mas que un cliente nuevo en volver a usarlo despues
// de restaurarlo (10,7 s frente a 1,7 s): el tiempo de recuperacion mediria el
// cliente y no la red. En las rondas de disponibilidad se acota a 1 s.
const RECONNECT_GRPC_OPTIONS = Object.freeze({
    'grpc.initial_reconnect_backoff_ms': 250,
    'grpc.max_reconnect_backoff_ms': 1000,
});

function withBoundedReconnect(networkConfig) {
    return {
        ...networkConfig,
        organizations: networkConfig.organizations.map((organization) => ({
            ...organization,
            peers: organization.peers.map((peer) => ({
                ...peer,
                grpcOptions: {...peer.grpcOptions, ...RECONNECT_GRPC_OPTIONS},
            })),
        })),
    };
}

const SIMULATION = 'Falla simulada con docker kill sobre un contenedor, en un unico host; '
    + 'no simula infraestructura distribuida ni la caida de una organizacion completa.';

// En la carga mixta las operaciones de una organizacion pasan por el Gateway de
// su propio peer; ademas exigen los endosos de sus targetMspIds. Por eso cada
// escenario de peer elige el peer cuya caida deja operaciones objetivo y de
// control dentro de la misma carga (ver el reparto en README).
const SCENARIOS = Object.freeze({
    'raft-1': {
        kind: 'orderer',
        orderers: 1,
        recovers: true,
        target: 'orderer lider del cluster Raft de 3 (1 de 3 nodos, organizaciones de ordenamiento distintas)',
    },
    'raft-2': {
        kind: 'orderer',
        orderers: 2,
        recovers: false,
        target: 'orderer lider y un seguidor del cluster Raft de 3 (2 de 3 nodos): perdida de quorum',
    },
    'peer-1a': {
        kind: 'peer',
        mspId: 'CentroMedicoMSP',
        recovers: true,
        target: 'peer0.centromedico: custodio de las unidades que dispensa y receptor de sus transferencias',
    },
    'peer-1b': {
        kind: 'peer',
        mspId: 'DistribuidorMSP',
        recovers: true,
        target: 'peer0.distribuidor: receptor de transitos activos laboratorio -> distribuidor',
    },
    'peer-1c': {
        kind: 'peer',
        mspId: 'AnmatMSP',
        recovers: true,
        probe: true,
        target: 'peer0.anmat: organizacion regulatoria',
    },
    'peer-1d': {
        kind: 'peer',
        mspId: 'LabMSP',
        recovers: true,
        target: 'peer0.lab: laboratorio que registra unidades',
    },
});

function scenarioDefinition(name) {
    const definition = SCENARIOS[name];
    if (!definition) throw new Error(`unsupported availability scenario ${name}`);
    return {name, ...definition};
}

// Ventanas en segundos desde el inicio de la carga. Raft-2 no restaura los
// orderers dentro de la ronda: la falla dura hasta el final (seccion 9.5).
function schedule(definition) {
    return definition.recovers
        ? {injectAfter: PRE_FAULT_SECONDS, recoverAfter: PRE_FAULT_SECONDS + FAULT_SECONDS, durationSeconds: ROUND_SECONDS}
        : {injectAfter: PRE_FAULT_SECONDS, recoverAfter: undefined, durationSeconds: ROUND_SECONDS};
}

// Organizaciones que una operacion necesita: el Gateway por el que entra cada
// invocacion y los endosos que exige.
function requirements(invocations) {
    const gateways = new Set();
    const endorsers = new Set();
    for (const invocation of invocations) {
        gateways.add(invocation.invokerMspId);
        for (const mspId of invocation.targetMspIds ?? []) endorsers.add(mspId);
    }
    return {gateways: [...gateways].sort(), endorsers: [...endorsers].sort()};
}

// Clase de disponibilidad (seccion 8.1): `target` si la operacion necesita el
// peer caido, como Gateway o como endosante; `control` si no.
function classify(invocations, downMspId) {
    const required = requirements(invocations);
    const reasons = [];
    if (required.gateways.includes(downMspId)) reasons.push('gateway');
    if (required.endorsers.includes(downMspId)) reasons.push('endorsement');
    return {
        availabilityClass: reasons.length > 0 ? 'target' : 'control',
        availabilityRequirement: {...required, downPeerRequiredAs: reasons},
    };
}

function operationKey(operation) {
    return `${operation.workerIndex}:${operation.ordinal}`;
}

// Anota cada operacion del crudo con su clase a partir del plan determinístico,
// sin modificar el workload que la ejecuto.
function classifyOperations(operations, plan, downMspId, probeInvocations = new Map()) {
    const invocationsByKey = new Map();
    for (const worker of plan.workers) {
        for (const operation of worker.operations) {
            invocationsByKey.set(`${worker.workerIndex}:${operation.ordinal}`, operation.invocations);
        }
    }
    return operations.map((operation) => {
        const invocations = invocationsByKey.get(operationKey(operation)) ?? probeInvocations.get(operationKey(operation));
        if (!invocations) throw new Error(`operation ${operationKey(operation)} is not in the work plan`);
        return {...operation, ...classify(invocations, downMspId)};
    });
}

// Lider del canal segun los logs de etcdraft: el ultimo cambio de lider
// registrado por cualquiera de los orderers.
function leaderFromLogs(logsByContainer, channel = 'snt-channel') {
    let latest;
    for (const [container, logs] of Object.entries(logsByContainer)) {
        for (const line of logs.split(/\r?\n/u)) {
            if (!line.includes(`channel=${channel}`)) continue;
            const change = /Raft leader changed: (\d+) -> (\d+)/u.exec(line);
            const stamp = /(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:\.\d+)?)/u.exec(line)?.[1];
            if (!change || !stamp) continue;
            const at = Date.parse(`${stamp.replace(' ', 'T')}Z`);
            if (!latest || at >= latest.at) latest = {at, node: Number(change[2]), reportedBy: container};
        }
    }
    if (!latest || latest.node === 0) return undefined;
    return ORDERERS.find((orderer) => orderer.node === latest.node);
}

// Orderers a dar de baja: el lider primero, para que Raft-1 fuerce una
// re-eleccion; en Raft-2, ademas, el siguiente en el orden de consenters.
function orderersToStop(leader, count) {
    if (!leader) throw new Error('the Raft leader of snt-channel could not be determined from the orderer logs');
    const others = ORDERERS.filter((orderer) => orderer.node !== leader.node);
    return [leader, ...others].slice(0, count);
}

// Lineas de los logs que explican la condicion de falla (seccion 8.1).
const RAFT_EVENT = /Raft leader changed|elected leader|became (leader|candidate|follower|pre-candidate)|lost leader|leader \d+ (lost|elected)|campaign|No leader|no Raft leader|leader is not available|is starting a new election|failed to send|Failed to send|connection refused|unreachable/iu;
const PEER_EVENT = /endorse|ENDORSEMENT|failed to|Failed to|deliver|unavailable|UNAVAILABLE|Starting peer|Gateway|connection/iu;

function eventLines(logs, pattern, fromMs, toMs) {
    const lines = [];
    for (const line of logs.split(/\r?\n/u)) {
        if (!pattern.test(line)) continue;
        const stamp = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)/u.exec(line)?.[1];
        const at = stamp ? Date.parse(stamp) : undefined;
        if (at !== undefined && (at < fromMs || at > toMs)) continue;
        lines.push(line.replace(/\u001b\[[0-9;]*m/gu, ''));
    }
    return lines;
}

// Ventanas medidas, en los terminos exactos que verifica el validador de
// metadata: las duraciones derivan de los instantes registrados.
function faultWindows({startedAt, endedAt, injectedAt, recoveredAt, durationSeconds}) {
    const start = Date.parse(startedAt);
    const end = Date.parse(endedAt);
    const injected = Date.parse(injectedAt);
    const preFaultSeconds = (injected - start) / 1000;
    if (!(preFaultSeconds > 0) || injected > end) throw new Error('fault injection falls outside the measured window');
    if (recoveredAt === undefined) {
        return {
            injectedAt,
            preFaultSeconds,
            faultSeconds: Math.min(durationSeconds - preFaultSeconds, (end - injected) / 1000),
        };
    }
    const recovered = Date.parse(recoveredAt);
    const faultSeconds = (recovered - injected) / 1000;
    if (!(faultSeconds > 0) || recovered > end) throw new Error('fault recovery falls outside the measured window');
    return {
        injectedAt,
        recoveredAt,
        preFaultSeconds,
        faultSeconds,
        recoverySeconds: Math.min(durationSeconds - preFaultSeconds - faultSeconds, (end - recovered) / 1000),
    };
}

// Unidades del flujo regulatorio de Peer-1c: unidades filler (registradas y en
// custodia del laboratorio, fuera de todos los planes de medicion), en orden de
// secuencia, salteando la del smoke.
function probeSequences(fillerSequences, smokeSequence, count) {
    const selected = fillerSequences.filter((sequence) => sequence !== smokeSequence).slice(0, count);
    if (selected.length < count) throw new Error(`snapshot has only ${selected.length} filler units for the regulatory probe`);
    return selected;
}

// Plan de la carga mixta extendida a 180 s. El snapshot canonico prepara las
// unidades de la mixta de 120 s y las de los escenarios de una sola operacion a
// su tasa maxima; una ronda de disponibilidad, que parte de su propia
// restauracion, no usa estas ultimas. El plan conserva el ciclo 2/11/2/5 y la
// rotacion por worker de la mixta y toma, por tipo de operacion, primero las
// unidades de la mixta y despues las de su escenario dedicado, que el snapshot
// dejo en el estado que esa operacion necesita.
const EXTRA_SOURCES = Object.freeze({
    register: {scenario: 'write-register', rate: 20},
    transfer: {scenario: 'write-transfer', rate: 20},
    dispense: {scenario: 'write-dispense', rate: 20},
    'query-unit': {scenario: 'read-unit', rate: 50},
});

function buildAvailabilityPlan(dataset, profile, seed, dependencies) {
    const {buildPlan, buildProfile, candidatePools, MIX_PATTERN, plannedCalls} = dependencies;
    const pools = candidatePools(dataset);
    const sourcePlan = (scenario, rate) => buildPlan(dataset, buildProfile({
        scenario, phase: 'measurement', repetition: '1', rate: String(rate),
    }), seed, {pools});
    const mixed = sourcePlan('mixed', 20);
    // Cada worker conserva, en el mismo orden, las operaciones que la mixta de
    // 120 s le asigna; las unidades de los escenarios dedicados se reparten
    // despues, en orden, entre los workers que las necesiten.
    const types = ['register', 'transfer', 'dispense', 'query-unit'];
    const ownQueues = mixed.workers.map((worker) => Object.fromEntries(types.map((type) => [
        type, worker.operations.filter((operation) => operation.type === type),
    ])));
    const queues = Object.fromEntries(types.map((type) => [type, []]));
    const preparations = new Map(mixed.preparations.map((entry) => [entry.datasetSequence, entry]));
    const extraPreparations = new Map();
    for (const [type, source] of Object.entries(EXTRA_SOURCES)) {
        const plan = sourcePlan(source.scenario, source.rate);
        for (const entry of plan.preparations) extraPreparations.set(entry.datasetSequence, entry);
        for (const worker of plan.workers) {
            for (const operation of worker.operations) {
                if (operation.type !== type) throw new Error(`${source.scenario} produced a ${operation.type} operation`);
                queues[type].push(operation);
            }
        }
    }
    const count = plannedCalls(profile);
    const fromMixed = Object.fromEntries(types.map((type) => [type, 0]));
    const fromDedicated = Object.fromEntries(types.map((type) => [type, 0]));
    const used = new Set();
    const workers = Array.from({length: profile.workers}, (_, workerIndex) => ({workerIndex, operations: []}));
    for (const worker of workers) {
        const rotation = (seed + worker.workerIndex) % MIX_PATTERN.length;
        const own = ownQueues[worker.workerIndex];
        const ownNext = Object.fromEntries(types.map((type) => [type, 0]));
        for (let ordinal = 0; ordinal < count; ordinal += 1) {
            const type = MIX_PATTERN[(ordinal + rotation) % MIX_PATTERN.length];
            let candidate = own?.[type]?.[ownNext[type]];
            if (candidate) {
                ownNext[type] += 1;
                fromMixed[type] += 1;
            } else {
                candidate = queues[type][fromDedicated[type]];
                fromDedicated[type] += 1;
            }
            if (!candidate) throw new Error(`snapshot has insufficient prepared ${type} units for ${profile.durationSeconds} s`);
            if (used.has(candidate.datasetSequence)) throw new Error(`unit ${candidate.datasetSequence} would be used twice`);
            used.add(candidate.datasetSequence);
            const preparation = extraPreparations.get(candidate.datasetSequence);
            if (preparation) preparations.set(candidate.datasetSequence, preparation);
            worker.operations.push({
                ordinal,
                datasetSequence: candidate.datasetSequence,
                type: candidate.type,
                invocations: candidate.invocations,
                expectedRejection: candidate.expectedRejection,
            });
        }
    }
    const required = new Set();
    for (const worker of workers) {
        for (const operation of worker.operations) for (const invocation of operation.invocations) required.add(invocation.invokerMspId);
    }
    const usedPreparations = [...preparations.values()].filter((entry) => used.has(entry.datasetSequence))
        .sort((left, right) => left.datasetSequence - right.datasetSequence);
    for (const entry of usedPreparations) for (const invocation of entry.invocations) required.add(invocation.invokerMspId);
    return {
        seed,
        scenario: 'mixed',
        profile,
        preparations: usedPreparations,
        workers,
        requiredMspIds: [...required].sort(),
        sources: Object.fromEntries(types.map((type) => [type, {
            fromMixed: fromMixed[type],
            fromDedicated: fromDedicated[type],
            dedicatedScenario: EXTRA_SOURCES[type].scenario,
        }])),
    };
}

module.exports = {
    EXTRA_SOURCES,
    FAULT_SECONDS,
    buildAvailabilityPlan,
    ORDERERS,
    PEERS,
    PEER_EVENT,
    PRE_FAULT_SECONDS,
    RAFT_EVENT,
    RECONNECT_GRPC_OPTIONS,
    RECOVERY_SECONDS,
    ROUND_SECONDS,
    SCENARIOS,
    SIMULATION,
    classify,
    classifyOperations,
    eventLines,
    faultWindows,
    leaderFromLogs,
    orderersToStop,
    probeSequences,
    requirements,
    scenarioDefinition,
    schedule,
    withBoundedReconnect,
};
