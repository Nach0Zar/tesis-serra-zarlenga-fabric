import assert from "node:assert/strict";
import { test } from "node:test";
import { END, SCHEDULE, rehearsalPassed, stepTiming } from "./schedule.mjs";

const onTime = () => SCHEDULE.map((start, index) => ({ ok: true, ...stepTiming(index, start, start + 2) }));

test("a rehearsal on schedule passes with and without video", () => {
  assert.equal(rehearsalPassed({ steps: onTime(), total: END, pageErrors: [] }), true);
  assert.equal(rehearsalPassed({ steps: onTime(), total: END + 0.2, pageErrors: [], videoOk: true }), true);
});

test("late evidence leaves the step out of its window", () => {
  // Paso 6: la operación termina a los 149 s pero la alerta llega a los 156 s.
  const timing = stepTiming(5, 125, 156);
  assert.equal(timing.windowEnd, 150);
  assert.equal(timing.fitsWindow, false);
  assert.equal(timing.duration, 31);

  const steps = onTime();
  steps[5] = { ok: true, ...timing };
  assert.equal(rehearsalPassed({ steps, total: END, pageErrors: [] }), false);
});

test("the last step's window ends with the script", () => {
  assert.equal(stepTiming(6, 150, 180).fitsWindow, true);
  assert.equal(stepTiming(6, 150, 183).fitsWindow, false);
});

test("a rehearsal longer than the script fails also without video", () => {
  const steps = onTime();
  assert.equal(rehearsalPassed({ steps, total: 183, pageErrors: [] }), false);
});

test("a missing step, missing evidence, page errors or a wrong video fail", () => {
  assert.equal(rehearsalPassed({ steps: onTime().slice(0, 6), total: END, pageErrors: [] }), false);
  const steps = onTime();
  steps[2].ok = false;
  assert.equal(rehearsalPassed({ steps, total: END, pageErrors: [] }), false);
  assert.equal(rehearsalPassed({ steps: onTime(), total: END, pageErrors: ["TypeError"] }), false);
  assert.equal(rehearsalPassed({ steps: onTime(), total: END, pageErrors: [], videoOk: false }), false);
});
