// Ensayo cronometrado y video de respaldo de la demo (DEMO-5, #147).
//
// Recorre el guion de docs/defense-demo.md sobre la pantalla real, iniciando
// cada paso en el segundo que le asigna el guion, y deja en el directorio de
// salida el video de los 3 minutos y el registro de tiempos de cada paso. El
// video es mudo: se narra en vivo con las mismas frases que la demo.
import { chromium } from "playwright";
import { mkdir, rename, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as sleep } from "node:timers/promises";
import { parseArgs } from "node:util";

// Segundo en el que empieza cada paso y fin del guion.
const SCHEDULE = [20, 40, 65, 85, 110, 125, 150];
const END = 180;

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

const viewport = { width: Number(options.width), height: Number(options.height) };
const stamp = new Date().toISOString().replace(/[:.]/g, "-");
await mkdir(options.out, { recursive: true });

const browser = await chromium.launch();
const context = await browser.newContext({
  viewport,
  recordVideo: options["no-video"] ? undefined : { dir: options.out, size: viewport },
});
const page = await context.newPage();
const pageErrors = [];
page.on("pageerror", (error) => pageErrors.push(String(error)));
page.on("dialog", (dialog) => dialog.accept());

const scenario = () => page.evaluate(async () => (await (await fetch("api/state")).json()).scenario);
const waitReady = () =>
  page.waitForFunction(() => document.querySelector("#overlay").hidden && document.querySelector("button.next"), null, {
    timeout: 120_000,
  });

await page.goto(options.url);
await waitReady().catch(() => {});
// Un ensayo empieza siempre desde el paso 1, con un escenario nuevo.
if ((await scenario()).nextStep !== 1) {
  await page.click("#reset");
  await page.waitForFunction(() => !document.querySelector("#overlay").hidden, null, { timeout: 10_000 });
}
await waitReady();
const { serial } = await scenario();

// Reloj monotónico: en WSL2 el reloj de pared se corrige a saltos.
const started = performance.now();
const elapsed = () => (performance.now() - started) / 1000;
const steps = [];
for (const [index, plannedStart] of SCHEDULE.entries()) {
  const number = index + 1;
  const wait = plannedStart - elapsed();
  if (wait > 0) await sleep(wait * 1000);
  const start = elapsed();
  await page.click("button.next");
  await page.waitForFunction((n) => document.querySelectorAll("button.done").length >= n || /^Error/.test(document.querySelector("#caption").textContent), number, {
    timeout: 60_000,
  });
  const end = elapsed();
  const caption = await page.textContent("#caption");
  const nextStart = SCHEDULE[index + 1] ?? END;
  steps.push({
    step: number,
    plannedStart,
    start: Number(start.toFixed(1)),
    duration: Number((end - start).toFixed(1)),
    ok: !caption.startsWith("Error"),
    fitsWindow: end <= nextStart,
    caption,
  });
  if (caption.startsWith("Error")) break;
}
const remaining = END - elapsed();
if (remaining > 0) await sleep(remaining * 1000);
const total = Number(elapsed().toFixed(1));

const video = page.video();
await context.close();
await browser.close();

let videoFile = null;
if (video) {
  videoFile = path.join(options.out, `demo-${stamp}.webm`);
  await rename(await video.path(), videoFile);
}
const passed = steps.length === SCHEDULE.length && steps.every((s) => s.ok && s.fitsWindow) && pageErrors.length === 0;
const record = { date: new Date().toISOString(), url: options.url, viewport, serial, total, passed, steps, pageErrors, video: videoFile };
const recordFile = path.join(options.out, `ensayo-${stamp}.json`);
await writeFile(recordFile, JSON.stringify(record, null, 2) + "\n");

console.log("paso  inicio  plan  duración  entra  resultado");
for (const s of steps) {
  console.log(
    `${String(s.step).padStart(4)}  ${s.start.toFixed(1).padStart(6)}  ${String(s.plannedStart).padStart(4)}  ${s.duration
      .toFixed(1)
      .padStart(8)}  ${s.fitsWindow ? "  sí" : "  NO"}   ${s.caption}`,
  );
}
console.log(`total ${total} s · ${passed ? "ensayo correcto" : "ensayo con problemas"}`);
if (pageErrors.length) console.log("errores de la pantalla:", pageErrors);
console.log("registro:", recordFile);
if (videoFile) console.log("video:", videoFile);
process.exitCode = passed ? 0 : 1;
