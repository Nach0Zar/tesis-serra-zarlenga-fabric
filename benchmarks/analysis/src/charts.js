'use strict';

// Figuras estaticas para el documento del PFI: SVG en modo claro, sin capa de
// interaccion. Paleta categorica validada (slots 1 y 2) y codificacion
// secundaria por forma de marcador y trazo, legible en escala de grises.
const STYLE = Object.freeze({
    surface: '#fcfcfb',
    textPrimary: '#0b0b0b',
    textSecondary: '#52514e',
    grid: '#e4e3df',
    axis: '#9b9a95',
    reference: '#9b9a95',
    font: 'Helvetica, Arial, sans-serif',
});
const SERIES = Object.freeze({
    fabric: {label: 'Fabric', color: '#2a78d6', marker: 'circle', dash: ''},
    baseline: {label: 'Baseline', color: '#eb6834', marker: 'square', dash: '6 4'},
});
// Caidas de peer: operaciones que requieren el peer caido frente a las de
// control (seccion 8.1). Mismos slots validados, con trazo distinto.
const CLASS_SERIES = Object.freeze({
    target: {label: 'Objetivo (requiere el peer caído)', color: '#2a78d6', marker: 'circle', dash: ''},
    control: {label: 'Control', color: '#eb6834', marker: 'square', dash: '6 4'},
});
const SCENARIO_LABELS = Object.freeze({
    'write-register': 'Registro',
    'write-transfer': 'Transferencia (pares/s)',
    'write-dispense': 'Dispensación',
    'read-unit': 'Consulta de unidad',
    'read-history': 'Consulta de historial',
    mixed: 'Carga mixta',
});

function escape(text) {
    return String(text).replace(/&/gu, '&amp;').replace(/</gu, '&lt;').replace(/>/gu, '&gt;');
}

function number(value, digits = 1) {
    return Number(value).toLocaleString('es-AR', {maximumFractionDigits: digits});
}

function marker(kind, x, y, color) {
    if (kind === 'square') {
        return `<rect x="${x - 4}" y="${y - 4}" width="8" height="8" rx="1" fill="${color}" stroke="${STYLE.surface}" stroke-width="2"/>`;
    }
    return `<circle cx="${x}" cy="${y}" r="4.5" fill="${color}" stroke="${STYLE.surface}" stroke-width="2"/>`;
}

// Paso redondo (1, 2, 2,5 o 5 por potencia de diez) para unas cuatro a seis marcas.
function niceTicks(maximum) {
    if (!(maximum > 0)) return [0, 1];
    const rough = maximum / 5;
    const magnitude = 10 ** Math.floor(Math.log10(rough));
    const step = [1, 2, 2.5, 5, 10].map((factor) => factor * magnitude).find((candidate) => candidate >= rough);
    const ticks = [];
    for (let tick = 0; tick < maximum + step * 0.999; tick += step) ticks.push(Number(tick.toPrecision(12)));
    return ticks;
}

function document_(width, height, title, body) {
    return `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}" role="img" aria-label="${escape(title)}">
<title>${escape(title)}</title>
<rect width="100%" height="100%" fill="${STYLE.surface}"/>
<g font-family="${STYLE.font}" font-size="12" fill="${STYLE.textPrimary}">
${body}
</g>
</svg>
`;
}

// Cada entrada empieza donde termina la anterior: la muestra (30 px), el texto
// (unos 7 px por caracter a 12 px) y 24 px de separacion.
function legend(x, y, entries) {
    let left = x;
    return entries.map((entry) => {
        const series = typeof entry === 'string' ? SERIES[entry] : entry;
        const start = left;
        left += 30 + series.label.length * 7 + 24;
        return `<line x1="${start}" y1="${y}" x2="${start + 22}" y2="${y}" stroke="${series.color}" stroke-width="2" stroke-dasharray="${series.dash}"/>
${marker(series.marker, start + 11, y, series.color)}
<text x="${start + 30}" y="${y + 4}">${series.label}</text>`;
    }).join('\n');
}

// Paneles chicos multiples: un panel por escenario, eje x categorico por tasa.
function panels(entriesByScenario, {title, yLabel, valueOf, rangeOf, scale, referenceOf, fileTitle}) {
    const scenarios = Object.keys(SCENARIO_LABELS).filter((scenario) => entriesByScenario[scenario]?.length);
    if (scenarios.length === 0) return undefined;
    const columns = Math.min(3, scenarios.length);
    const rows = Math.ceil(scenarios.length / columns);
    const panelWidth = 260;
    const panelHeight = 200;
    const margin = {top: 34, right: 16, bottom: 40, left: 58};
    const width = columns * (panelWidth + margin.left + margin.right) + 16;
    const height = rows * (panelHeight + margin.top + margin.bottom) + 64;
    const suts = ['fabric', 'baseline'].filter((sut) => scenarios.some((scenario) =>
        entriesByScenario[scenario].some((entry) => entry.sut === sut)));
    const parts = [
        `<text x="16" y="24" font-size="15" font-weight="600">${escape(title)}</text>`,
        legend(16, 46, suts),
    ];
    scenarios.forEach((scenario, index) => {
        const entries = entriesByScenario[scenario];
        const originX = 16 + (index % columns) * (panelWidth + margin.left + margin.right) + margin.left;
        const originY = 64 + Math.floor(index / columns) * (panelHeight + margin.top + margin.bottom) + margin.top;
        const rates = [...new Set(entries.map((entry) => entry.targetOperationsPerSecond))].sort((left, right) => left - right);
        const values = entries.flatMap((entry) => {
            const range = rangeOf(entry);
            return [valueOf(entry), ...(range ?? []), referenceOf?.(entry)].filter(Number.isFinite);
        });
        const y = scaleFor(scale, values, panelHeight);
        const xAt = (rate) => originX + ((rates.indexOf(rate) + 0.5) / rates.length) * panelWidth;
        parts.push(`<text x="${originX}" y="${originY - 14}" font-size="13" font-weight="600">${escape(SCENARIO_LABELS[scenario])}</text>`);
        for (const tick of y.ticks) {
            const ty = originY + y.at(tick);
            parts.push(`<line x1="${originX}" y1="${ty}" x2="${originX + panelWidth}" y2="${ty}" stroke="${STYLE.grid}" stroke-width="1"/>`);
            parts.push(`<text x="${originX - 6}" y="${ty + 4}" text-anchor="end" fill="${STYLE.textSecondary}" font-size="11">${y.format(tick)}</text>`);
        }
        parts.push(`<line x1="${originX}" y1="${originY + panelHeight}" x2="${originX + panelWidth}" y2="${originY + panelHeight}" stroke="${STYLE.axis}" stroke-width="1"/>`);
        for (const rate of rates) {
            parts.push(`<text x="${xAt(rate)}" y="${originY + panelHeight + 16}" text-anchor="middle" fill="${STYLE.textSecondary}" font-size="11">${number(rate)}</text>`);
        }
        parts.push(`<text x="${originX + panelWidth / 2}" y="${originY + panelHeight + 32}" text-anchor="middle" fill="${STYLE.textSecondary}" font-size="11">tasa objetivo (op/s)</text>`);
        parts.push(`<text transform="translate(${originX - 44} ${originY + panelHeight / 2}) rotate(-90)" text-anchor="middle" fill="${STYLE.textSecondary}" font-size="11">${escape(yLabel)}</text>`);
        if (referenceOf) {
            const points = rates.map((rate) => `${xAt(rate)},${originY + y.at(rate)}`).join(' ');
            parts.push(`<polyline points="${points}" fill="none" stroke="${STYLE.reference}" stroke-width="1" stroke-dasharray="2 3"/>`);
        }
        suts.forEach((sut, sutIndex) => {
            const series = SERIES[sut];
            const own = entries.filter((entry) => entry.sut === sut)
                .sort((left, right) => left.targetOperationsPerSecond - right.targetOperationsPerSecond);
            if (own.length === 0) return;
            const offset = (sutIndex - (suts.length - 1) / 2) * 10;
            const coordinates = own.map((entry) => ({
                x: xAt(entry.targetOperationsPerSecond) + offset,
                y: originY + y.at(valueOf(entry)),
                range: rangeOf(entry),
            }));
            if (coordinates.length > 1) {
                parts.push(`<polyline points="${coordinates.map((point) => `${point.x},${point.y}`).join(' ')}" fill="none" stroke="${series.color}" stroke-width="2" stroke-dasharray="${series.dash}"/>`);
            }
            for (const point of coordinates) {
                if (point.range) {
                    const [low, high] = point.range.map((value) => originY + y.at(value));
                    parts.push(`<line x1="${point.x}" y1="${low}" x2="${point.x}" y2="${high}" stroke="${series.color}" stroke-width="2"/>`);
                    parts.push(`<line x1="${point.x - 4}" y1="${low}" x2="${point.x + 4}" y2="${low}" stroke="${series.color}" stroke-width="2"/>`);
                    parts.push(`<line x1="${point.x - 4}" y1="${high}" x2="${point.x + 4}" y2="${high}" stroke="${series.color}" stroke-width="2"/>`);
                }
                parts.push(marker(series.marker, point.x, point.y, series.color));
            }
        });
    });
    return document_(width, height, fileTitle ?? title, parts.join('\n'));
}

function scaleFor(kind, values, height) {
    if (kind === 'log') {
        const positive = values.filter((value) => value > 0);
        // Margen de una fraccion de decada para que ningun valor quede sobre el eje.
        const low = 10 ** Math.floor(Math.log10(Math.min(...positive)) - 0.1);
        const high = 10 ** Math.ceil(Math.log10(Math.max(...positive)) + 0.05);
        const span = Math.log10(high) - Math.log10(low) || 1;
        const ticks = [];
        for (let tick = low; tick <= high * 1.0001; tick *= 10) ticks.push(tick);
        return {
            at: (value) => height - ((Math.log10(Math.max(value, low)) - Math.log10(low)) / span) * height,
            ticks,
            format: (tick) => (tick >= 1000 ? `${number(tick / 1000)} s` : `${number(tick)} ms`),
        };
    }
    const ticks = niceTicks(Math.max(...values, 0) * 1.05);
    const max = ticks.at(-1);
    return {at: (value) => height - (value / max) * height, ticks, format: (tick) => number(tick)};
}

function byScenario(aggregates) {
    const grouped = {};
    for (const entry of aggregates) {
        if (entry.scenario === 'expected-rejections') continue;
        (grouped[entry.scenario] ??= []).push(entry);
    }
    return grouped;
}

function rotulo(title, options) {
    return options.nonCitable ? `NO CITABLE · ${title}` : title;
}

function throughputChart(aggregates, options = {}) {
    return panels(byScenario(aggregates), {
        title: rotulo('Throughput efectivo por tasa objetivo (media ± desvío entre repeticiones)', options),
        yLabel: 'operaciones exitosas/s',
        valueOf: (entry) => entry.throughput.mean,
        rangeOf: (entry) => (entry.throughput.n > 1
            ? [entry.throughput.mean - entry.throughput.standardDeviation, entry.throughput.mean + entry.throughput.standardDeviation]
            : undefined),
        referenceOf: (entry) => entry.targetOperationsPerSecond,
        scale: 'linear',
    });
}

function latencyChart(aggregates, options = {}) {
    return panels(byScenario(aggregates), {
        title: rotulo('Latencia por operación: p95 (marcador) y rango p50–p99 (media entre repeticiones, escala logarítmica)', options),
        yLabel: 'latencia',
        valueOf: (entry) => entry.p95Ms.mean,
        rangeOf: (entry) => [entry.p50Ms.mean, entry.p99Ms.mean],
        scale: 'log',
    });
}

// Tasa de exito por intervalo alrededor de la falla; las lineas verticales marcan
// la inyeccion y la restauracion.
function availabilityChart(title, timelines, options = {}) {
    const decorated = rotulo(options.sut ? `${title} (${SERIES[options.sut].label})` : title, options);
    const width = 720;
    const height = 300;
    const margin = {top: 64, right: 24, bottom: 44, left: 58};
    const plotWidth = width - margin.left - margin.right;
    const plotHeight = height - margin.top - margin.bottom;
    const offsets = timelines.flatMap((timeline) => timeline.points.map((point) => point.midpointSeconds ?? point.offsetSeconds));
    if (offsets.length === 0) return undefined;
    const minX = Math.min(...offsets, 0);
    const maxX = Math.max(...offsets, ...timelines.map((timeline) => timeline.recoveredOffset ?? 0));
    const xAt = (offset) => margin.left + ((offset - minX) / Math.max(maxX - minX, 1)) * plotWidth;
    const yAt = (rate) => margin.top + (1 - rate) * plotHeight;
    // Una sola serie no lleva leyenda: el titulo y el SUT la identifican.
    const seriesOf = (timeline) => timeline.series ?? SERIES[timeline.sut];
    const entries = timelines.map(seriesOf);
    const single = entries.length === 1 && timelines[0].sut;
    const parts = [
        `<text x="16" y="24" font-size="15" font-weight="600">${escape(single ? `${decorated} (${entries[0].label})` : decorated)}</text>`,
    ];
    if (entries.length > 1) parts.push(legend(16, 46, entries));
    for (let tick = Math.ceil(minX / 30) * 30; tick <= maxX; tick += 30) {
        parts.push(`<line x1="${xAt(tick)}" y1="${margin.top + plotHeight}" x2="${xAt(tick)}" y2="${margin.top + plotHeight + 4}" stroke="${STYLE.axis}"/>`);
        parts.push(`<text x="${xAt(tick)}" y="${margin.top + plotHeight + 16}" text-anchor="middle" fill="${STYLE.textSecondary}" font-size="11">${tick}</text>`);
    }
    for (const tick of [0, 0.25, 0.5, 0.75, 1]) {
        parts.push(`<line x1="${margin.left}" y1="${yAt(tick)}" x2="${margin.left + plotWidth}" y2="${yAt(tick)}" stroke="${STYLE.grid}"/>`);
        parts.push(`<text x="${margin.left - 6}" y="${yAt(tick) + 4}" text-anchor="end" fill="${STYLE.textSecondary}" font-size="11">${tick * 100} %</text>`);
    }
    const markers = [{offset: 0, label: 'falla'}];
    for (const timeline of timelines) {
        if (timeline.recoveredOffset !== undefined) markers.push({offset: timeline.recoveredOffset, label: 'restauración'});
    }
    for (const {offset, label} of markers) {
        parts.push(`<line x1="${xAt(offset)}" y1="${margin.top}" x2="${xAt(offset)}" y2="${margin.top + plotHeight}" stroke="${STYLE.axis}" stroke-dasharray="4 3"/>`);
        parts.push(`<text x="${xAt(offset) + 4}" y="${margin.top + 12}" fill="${STYLE.textSecondary}" font-size="11">${label}</text>`);
    }
    for (const timeline of timelines) {
        const series = seriesOf(timeline);
        const points = timeline.points
            .map((point) => `${xAt(point.midpointSeconds ?? point.offsetSeconds)},${yAt(point.successRate)}`).join(' ');
        parts.push(`<polyline points="${points}" fill="none" stroke="${series.color}" stroke-width="2" stroke-dasharray="${series.dash}"/>`);
    }
    parts.push(`<text x="${margin.left + plotWidth / 2}" y="${height - 10}" text-anchor="middle" fill="${STYLE.textSecondary}" font-size="11">segundos desde la inyección de la falla</text>`);
    parts.push(`<text transform="translate(16 ${margin.top + plotHeight / 2}) rotate(-90)" text-anchor="middle" fill="${STYLE.textSecondary}" font-size="11">tasa de éxito</text>`);
    return document_(width, height, decorated, parts.join('\n'));
}

module.exports = {CLASS_SERIES, SCENARIO_LABELS, SERIES, availabilityChart, latencyChart, throughputChart};
