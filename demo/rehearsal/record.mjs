// Ensayo cronometrado y video de respaldo de la demo (DEMO-5, #147).
//
// Recorre el guion de docs/defense-demo.md sobre la pantalla real, iniciando
// cada paso en el segundo que le asigna el guion, y comprueba que cada paso
// dejó su evidencia en la red y en la pantalla (evidence.mjs). Deja en el
// directorio de salida el video de los 3 minutos y el registro del ensayo. El
// video es mudo: se narra en vivo con las mismas frases que la demo.
import { chromium } from "playwright";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";
import { stepEvidence } from "./evidence.mjs";
import { ffmpegPath, startRecording, videoDuration } from "./screen-recorder.mjs";

// Segundo en el que empieza cada paso y fin del guion.
const SCHEDULE = [20, 40, 65, 85, 110, 125, 150];
const END = 180;
// Diferencia admitida entre la duración del video y la del guion.
const VIDEO_TOLERANCE = 2;
// Tiempo para que los bloques y alertas de un paso lleguen a la pantalla.
const EVIDENCE_TIMEOUT = 8_000;

const here = path.dirname(fileURLToPath(import.meta.url));
const { values: options } = parseArgs({
  options: {
    url: { type: "string", default: "http://127.0.0.1:8080/" },
    out: { type: "string", default: path.resolve(here, "../../build/demo/rehearsal") },
    width: { type: "string", default: "1920" },
    height: { type: "string", default: "1080" },
    "no-video": { type: "boolean", default: false },
  },
});

const api = new URL("api/", options.url);
const getState = async () => {
  const response = await fetch(new URL("state", api));
  if (!response.ok) throw new Error(`GET /api/state: ${response.status}`);
  return response.json();
};

// Reloj monotónico: en WSL2 el reloj de pared se corrige a saltos.
const now = () => performance.now() / 1000;

// 1. Escenario nuevo, preparado y conectado, antes de abrir el navegador: la
// preparación no queda en el video ni en el cronómetro.
async function prepareScenario() {
  let state = await getState();
  if (state.scenario.nextStep !== 1 || state.scenario.preparation === "FALLIDA") {
    const response = await fetch(new URL("reset", api), { method: "POST" });
    if (!response.ok) throw new Error(`POST /api/reset: ${response.status}`);
  }
  const deadline = now() + 120;
  while (now() < deadline) {
    state = await getState();
    const { preparation, nextStep } = state.scenario;
    if (preparation === "FALLIDA") throw new Error(`preparación fallida: ${state.scenario.preparationError?.message}`);
    if (preparation === "LISTA" && nextStep === 1 && state.connected) return state;
    await sleep(500);
  }
  throw new Error("el escenario no quedó listo y conectado en 120 s");
}

const prepared = await prepareScenario();
const stamp = new Date().toISOString().replace(/[:.]/g, "-");
await mkdir(options.out, { recursive: true });
const viewport = { width: Number(options.width), height: Number(options.height) };

// 2. La pantalla se abre con el escenario listo; la grabación empieza con el
// cronómetro del guion.
const browser = await chromium.launch();
const context = await browser.newContext({ viewport });
const page = await context.newPage();
const pageErrors = [];
page.on("pageerror", (error) => pageErrors.push(String(error)));
page.on("dialog", (dialog) => dialog.accept());
await page.goto(options.url);
await page.waitForFunction(
  () =>
    document.querySelector("#overlay").hidden &&
    document.querySelector("button.next") &&
    document.querySelector("#connection").classList.contains("online"),
  null,
  { timeout: 30_000 },
);

const screen = () =>
  page.evaluate(() => ({
    blocks: document.querySelectorAll("#blocks .block").length,
    rejectionVisible: !document.querySelector("#rejection").hidden,
    privacySees: document.querySelectorAll("#privacy .sees").length,
    alerts: document.querySelectorAll("#alerts .alert").length,
    verdictOk: Boolean(document.querySelector("#trace:not([hidden]) .verdict.ok")),
  }));

// Bloques y alertas llegan por SSE poco después de que el paso responde: se
// reintenta la comprobación hasta EVIDENCE_TIMEOUT.
async function verify(number, blocksBefore) {
  const deadline = now() + EVIDENCE_TIMEOUT / 1000;
  let problems;
  do {
    problems = stepEvidence(number, await getState(), await screen(), blocksBefore);
    if (problems.length === 0) return problems;
    await sleep(250);
  } while (now() < deadline);
  return problems;
}

// 3. El guion, cronometrado y grabado desde el mismo instante.
const ffmpeg = ffmpegPath(chromium.executablePath());
const videoFile = options["no-video"] ? null : path.join(options.out, `demo-${stamp}.webm`);
const stopRecording = videoFile ? await startRecording(page, videoFile, { ffmpeg, ...viewport }) : null;
const started = now();
const elapsed = () => now() - started;
const steps = [];
for (const [index, plannedStart] of SCHEDULE.entries()) {
  const number = index + 1;
  const wait = plannedStart - elapsed();
  if (wait > 0) await sleep(wait * 1000);
  const blocksBefore = (await getState()).blocks.length;
  const start = elapsed();
  await page.click("button.next");
  await page.waitForFunction(
    (n) => document.querySelectorAll("button.done").length >= n || /^Error/.test(document.querySelector("#caption").textContent),
    number,
    { timeout: 60_000 },
  );
  const end = elapsed();
  const caption = await page.textContent("#caption");
  const problems = caption.startsWith("Error") ? [caption] : await verify(number, blocksBefore);
  const nextStart = SCHEDULE[index + 1] ?? END;
  steps.push({
    step: number,
    plannedStart,
    start: Number(start.toFixed(1)),
    duration: Number((end - start).toFixed(1)),
    ok: problems.length === 0,
    fitsWindow: end <= nextStart,
    problems,
    caption,
  });
  if (caption.startsWith("Error")) break;
}
const remaining = END - elapsed();
if (remaining > 0) await sleep(remaining * 1000);
const total = Number(elapsed().toFixed(1));

const recordedSeconds = stopRecording ? await stopRecording() : null;
await context.close();
await browser.close();
// La duración se lee del archivo terminado, no del grabador.
const videoSeconds = videoFile ? await videoDuration(ffmpeg, videoFile) : null;
const videoOk = !videoFile || (videoSeconds !== null && Math.abs(videoSeconds - END) <= VIDEO_TOLERANCE);
const passed =
  steps.length === SCHEDULE.length && steps.every((s) => s.ok && s.fitsWindow) && pageErrors.length === 0 && videoOk;
const record = {
  date: new Date().toISOString(),
  url: options.url,
  viewport,
  serial: prepared.scenario.serial,
  total,
  recordedSeconds,
  videoSeconds,
  passed,
  steps,
  pageErrors,
  video: videoFile,
};
const recordFile = path.join(options.out, `ensayo-${stamp}.json`);
await writeFile(recordFile, JSON.stringify(record, null, 2) + "\n");

console.log("paso  inicio  plan  duración  entra  evidencia");
for (const s of steps) {
  console.log(
    `${String(s.step).padStart(4)}  ${s.start.toFixed(1).padStart(6)}  ${String(s.plannedStart).padStart(4)}  ${s.duration
      .toFixed(1)
      .padStart(8)}  ${s.fitsWindow ? "  sí" : "  NO"}   ${s.ok ? "completa" : s.problems.join("; ")}`,
  );
}
console.log(`guion ${total} s${videoFile ? ` · video ${videoSeconds} s${videoOk ? "" : " (no dura lo que el guion)"}` : ""}`);
console.log(passed ? "ensayo correcto" : "ensayo con problemas");
if (pageErrors.length) console.log("errores de la pantalla:", pageErrors);
console.log("registro:", recordFile);
if (videoFile) console.log("video:", videoFile);
process.exitCode = passed ? 0 : 1;
