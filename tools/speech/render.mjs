// SPDX-License-Identifier: GPL-2.0-or-later

// Renders text to speech with Kokoro-82M for the speech Go package.
//
// One-shot (no arguments):
//   stdin:  {"voice": "bm_george", "speed": 1.3, "items": [{"text": "...", "out": "/abs/path.f32"}]}
//   output: each item's samples as raw little-endian float32 mono at 24 kHz.
//   An optional "blend": {"voice": "bm_fable", "weight": 0.4} mixes the
//   two voices' style vectors, (1-weight)*voice + weight*blend.voice; the
//   text is still read with the first voice's accent.
//
// Serve (`node render.mjs serve`): loads the model once, prints
// {"ready":true}, then answers one JSON request per stdin line with one
// JSON line on stdout. A request is either a one-shot job as above or
// {"op":"voices"}; the answer is {"ok":true}, {"voices":[...]} or
// {"error":"..."}.
//
// The model is fetched once from a pinned Hugging Face revision, checked
// against the hashes below, and never fetched remotely again.
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import readline from "node:readline";
import { env, Tensor } from "@huggingface/transformers";
import { KokoroTTS } from "kokoro-js";

const REPO = "onnx-community/Kokoro-82M-v1.0-ONNX";
const REVISION = "1939ad2a8e416c0acfeecc08a694d14ef25f2231";
const FILES = {
  "config.json": "df34b4f930b23447cd4dc410fabfb42eb3f24e803e6c3f97d618fb359380a36f",
  "tokenizer.json": "77a02c8e164413299b4b4c403b14f8e0e1c1b727db4d46a09d6327b861060a34",
  "tokenizer_config.json": "be1cb066d6ef6b074b3f15e6a6dd21ac88ff3cdaedf325f0aaed686c70f75d20",
  "onnx/model.onnx": "8fbea51ea711f2af382e88c833d9e288c6dc82ce5e98421ea61c058ce21a34cb",
};

const cacheRoot = process.env.ARKADE_SPEECH_CACHE ||
  path.join(process.env.XDG_CACHE_HOME || path.join(os.homedir(), ".cache"), "arkade-speech");
const modelDir = path.join(cacheRoot, REVISION, REPO);

const sha256 = file => createHash("sha256").update(fs.readFileSync(file)).digest("hex");

async function ensureModel() {
  for (const [name, want] of Object.entries(FILES)) {
    const dest = path.join(modelDir, name);
    if (fs.existsSync(dest) && sha256(dest) === want) continue;
    const url = `https://huggingface.co/${REPO}/resolve/${REVISION}/${name}`;
    process.stderr.write(`speech: fetching ${url}\n`);
    const res = await fetch(url);
    if (!res.ok) throw new Error(`${url}: HTTP ${res.status}`);
    fs.mkdirSync(path.dirname(dest), { recursive: true });
    const tmp = `${dest}.part`;
    fs.writeFileSync(tmp, Buffer.from(await res.arrayBuffer()));
    const got = sha256(tmp);
    if (got !== want) throw new Error(`${name}: sha256 ${got}, want ${want}`);
    fs.renameSync(tmp, dest);
  }
}

async function load() {
  await ensureModel();
  env.localModelPath = path.join(cacheRoot, REVISION);
  env.allowRemoteModels = false;
  return KokoroTTS.from_pretrained(REPO, { dtype: "fp32", device: "cpu" });
}

const voicesDir = new URL("../voices/", import.meta.resolve("kokoro-js"));
const styles = new Map();

function style(id) {
  if (!styles.has(id)) {
    const bytes = new Uint8Array(fs.readFileSync(new URL(`${id}.bin`, voicesDir)));
    styles.set(id, new Float32Array(bytes.buffer));
  }
  return styles.get(id);
}

function blendStyle(a, b, w) {
  const x = style(a), y = style(b);
  if (x.length !== y.length) throw new Error(`voices "${a}" and "${b}" differ in shape`);
  return Float32Array.from(x, (v, i) => (1 - w) * v + w * y[i]);
}

// kokoro-js 1.2.1 takes a voice only by name, so a blend swaps the style
// row it picked (one per token count) for the same row of the mix.
async function generateBlended(tts, text, job) {
  const { voice: other, weight } = job.blend;
  if (!(weight >= 0 && weight <= 1)) throw new Error(`blend weight ${weight} is outside 0..1`);
  const own = style(job.voice), mix = blendStyle(job.voice, other, weight);
  const model = tts.model;
  tts.model = inputs => {
    const row = 256 * Math.min(Math.max(inputs.input_ids.dims.at(-1) - 2, 0), 509);
    const picked = inputs.style.data;
    if (picked.length !== 256 || picked.some((v, i) => v !== own[row + i])) {
      throw new Error("kokoro-js picked a style row the blend does not expect; check its version");
    }
    return model({ ...inputs, style: new Tensor("float32", mix.slice(row, row + 256), [1, 256]) });
  };
  try {
    return await tts.generate(text, { voice: job.voice, speed: job.speed });
  } finally {
    tts.model = model;
  }
}

async function run(tts, job) {
  for (const v of [job.voice, job.blend?.voice].filter(v => v !== undefined)) {
    if (!Object.hasOwn(tts.voices, v)) throw new Error(`unknown voice "${v}"`);
  }
  for (const { text, out } of job.items) {
    const audio = job.blend
      ? await generateBlended(tts, text, job)
      : await tts.generate(text, { voice: job.voice, speed: job.speed });
    if (audio.sampling_rate !== 24000) throw new Error(`unexpected rate ${audio.sampling_rate}`);
    const s = audio.audio;
    fs.writeFileSync(out, Buffer.from(s.buffer, s.byteOffset, s.byteLength));
  }
}

const voices = tts => Object.entries(tts.voices).map(([id, v]) => ({
  id, name: v.name, language: v.language, gender: v.gender, grade: v.overallGrade,
}));

if (process.argv[2] === "serve") {
  // stdout carries only the protocol; libraries that log go to stderr.
  for (const k of ["log", "info", "warn", "table", "debug"]) console[k] = (...a) => console.error(...a);
  const say = obj => process.stdout.write(JSON.stringify(obj) + "\n");
  let tts;
  try {
    tts = await load();
  } catch (e) {
    say({ error: String(e?.message ?? e) });
    process.exit(1);
  }
  say({ ready: true });
  const lines = readline.createInterface({ input: process.stdin });
  for await (const line of lines) {
    if (!line.trim()) continue;
    try {
      const req = JSON.parse(line);
      if (req.op === "voices") say({ voices: voices(tts) });
      else {
        await run(tts, req);
        say({ ok: true });
      }
    } catch (e) {
      say({ error: String(e?.message ?? e) });
    }
  }
} else {
  const job = JSON.parse(fs.readFileSync(0, "utf8"));
  await run(await load(), job);
}
