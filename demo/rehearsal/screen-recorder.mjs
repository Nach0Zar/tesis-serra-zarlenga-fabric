// Grabación de la pantalla a tiempo real.
//
// El video de Playwright (recordVideo) se estira respecto del reloj cuando la
// página tiene actividad: en el guion completo, 187 s reales quedaban en 210 s
// de video, y la narración en vivo se desincronizaba. Este grabador toma los
// cuadros del screencast de Chromium y le entrega a ffmpeg exactamente 25 por
// segundo según el reloj monotónico, repitiendo el último cuadro cuando la
// pantalla no cambia. La duración del archivo es la del tiempo grabado.
import { spawn } from "node:child_process";
import { existsSync, readdirSync } from "node:fs";
import path from "node:path";

const FPS = 25;

// ffmpegPath usa el ffmpeg que instala `npx playwright install`, junto a los
// navegadores, o el indicado en FFMPEG.
export function ffmpegPath(browserExecutable) {
  if (process.env.FFMPEG) return process.env.FFMPEG;
  let directory = path.dirname(browserExecutable);
  while (directory !== path.dirname(directory)) {
    if (path.basename(directory) === "ms-playwright" || existsSync(path.join(directory, ".links"))) break;
    directory = path.dirname(directory);
  }
  for (const entry of readdirSync(directory).filter((name) => name.startsWith("ffmpeg-")).sort().reverse()) {
    for (const binary of ["ffmpeg-linux", "ffmpeg-mac", "ffmpeg-win64.exe"]) {
      const candidate = path.join(directory, entry, binary);
      if (existsSync(candidate)) return candidate;
    }
  }
  return "ffmpeg";
}

// startRecording empieza a grabar la página en file y devuelve stop(), que
// termina el archivo y devuelve la cantidad de segundos grabados.
export async function startRecording(page, file, { ffmpeg, width, height }) {
  const session = await page.context().newCDPSession(page);
  const first = await session.send("Page.captureScreenshot", { format: "jpeg", quality: 85 });
  let latest = Buffer.from(first.data, "base64");
  session.on("Page.screencastFrame", ({ data, sessionId }) => {
    latest = Buffer.from(data, "base64");
    session.send("Page.screencastFrameAck", { sessionId }).catch(() => {});
  });
  await session.send("Page.startScreencast", { format: "jpeg", quality: 85, maxWidth: width, maxHeight: height });

  const encoder = spawn(
    ffmpeg,
    [
      "-loglevel", "error",
      "-f", "image2pipe", "-framerate", String(FPS), "-c:v", "mjpeg", "-i", "pipe:0",
      "-vf", `scale=${width}:${height}:force_original_aspect_ratio=decrease,pad=${width}:${height}:(ow-iw)/2:(oh-ih)/2`,
      "-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-b:v", "1500k", "-row-mt", "1",
      "-y", file,
    ],
    { stdio: ["pipe", "ignore", "pipe"] },
  );
  let encoderErrors = "";
  encoder.stderr.on("data", (chunk) => (encoderErrors += chunk));
  const finished = new Promise((resolve, reject) => {
    encoder.on("error", reject);
    encoder.on("close", (code) => (code === 0 ? resolve() : reject(new Error(`ffmpeg: ${code} ${encoderErrors}`))));
  });

  const started = performance.now();
  let frames = 0;
  const writeDue = () => {
    const due = Math.floor(((performance.now() - started) / 1000) * FPS);
    while (frames < due) {
      encoder.stdin.write(latest);
      frames += 1;
    }
  };
  const timer = setInterval(writeDue, 1000 / FPS);

  return async function stop() {
    writeDue();
    clearInterval(timer);
    await session.send("Page.stopScreencast").catch(() => {});
    encoder.stdin.end();
    await finished;
    return frames / FPS;
  };
}

// videoDuration lee la duración del archivo terminado.
export function videoDuration(ffmpeg, file) {
  return new Promise((resolve) => {
    const probe = spawn(ffmpeg, ["-hide_banner", "-i", file], { stdio: ["ignore", "ignore", "pipe"] });
    let output = "";
    probe.stderr.on("data", (chunk) => (output += chunk));
    probe.on("close", () => {
      const match = /Duration: (\d+):(\d+):([\d.]+)/.exec(output);
      resolve(match ? Number(match[1]) * 3600 + Number(match[2]) * 60 + Number(match[3]) : null);
    });
  });
}
