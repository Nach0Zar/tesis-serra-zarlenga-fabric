import assert from "node:assert/strict";
import { test } from "node:test";
import { stepEvidence } from "./evidence.mjs";

const ORGS = ["anmat", "lab", "drogueria", "distribuidor", "farmacia", "centromedico", "financiador"];

function privacy(from, to, members = [from, to, "anmat"]) {
  return {
    from,
    to,
    hash: "abcd",
    organizations: ORGS.map((organization) => ({ organization, sees: members.includes(organization) })),
  };
}

// Estado de demo-server tras un ensayo correcto hasta el paso indicado.
function completeState(upTo) {
  let block = 100;
  const blocks = [];
  const tx = (fn, organization, endorsers) => {
    block += 1;
    blocks.push({ number: block, transactions: [{ transactionId: `tx-${block}`, function: fn, endorsers, valid: true }] });
    return { function: fn, organization, transactionId: `tx-${block}`, blockNumber: block };
  };
  const results = [
    { step: 1, ok: true, transactions: [tx("RegisterUnit", "lab", ["lab"])] },
    {
      step: 2,
      ok: true,
      transactions: [tx("DispatchTransfer", "lab", ["lab"]), tx("ReceiveTransfer", "drogueria", ["drogueria", "lab"])],
      privacy: privacy("lab", "drogueria"),
    },
    {
      step: 3,
      ok: true,
      transactions: [
        tx("DispatchTransfer", "drogueria", ["drogueria"]),
        tx("ReceiveTransfer", "farmacia", ["drogueria", "farmacia"]),
      ],
      privacy: privacy("drogueria", "farmacia"),
    },
    { step: 4, ok: true, transactions: [], rejection: { code: "TRANSFER_NOT_AUTHORIZED" } },
    { step: 5, ok: true, transactions: [tx("Dispense", "farmacia", ["farmacia"])] },
    { step: 6, ok: true, transactions: [tx("ReportStolen", "farmacia", ["farmacia"])] },
    { step: 7, ok: true, transactions: [], trace: { verdict: { legitima: true, motivo: "" } } },
  ].filter((result) => result.step <= upTo);
  const alerts =
    upTo >= 6 ? [{ eventName: "ReportStolen", unit: { numeroSerie: "ROBO-1" }, blockNumber: blocks.at(-1).number }] : [];
  return { scenario: { theftSerial: "ROBO-1", results }, blocks, alerts };
}

function screenFor(state, overrides = {}) {
  return {
    blocks: state.blocks.length,
    rejectionVisible: true,
    privacySees: 3,
    alerts: state.alerts.length,
    verdictOk: true,
    ...overrides,
  };
}

test("a complete rehearsal leaves the evidence of every step", () => {
  for (let step = 1; step <= 7; step++) {
    const state = completeState(step);
    const blocksBefore = step === 4 ? state.blocks.length : 0;
    assert.deepEqual(stepEvidence(step, state, screenFor(state), blocksBefore), [], `step ${step}`);
  }
});

test("a lost ANMAT alert fails step 6", () => {
  const state = completeState(6);
  state.alerts = [];
  const problems = stepEvidence(6, state, screenFor(state), 0);
  assert.ok(problems.some((p) => p.includes("ANMAT recibió 0 alertas")), problems.join("; "));
});

test("a duplicated alert fails step 6", () => {
  const state = completeState(6);
  state.alerts.push(state.alerts[0]);
  assert.ok(stepEvidence(6, state, screenFor(state), 0).some((p) => p.includes("2 alertas")));
});

test("wrong privacy fails a transfer", () => {
  const state = completeState(3);
  state.scenario.results[2].privacy = privacy("drogueria", "farmacia", ["drogueria", "farmacia", "anmat", "lab"]);
  const problems = stepEvidence(3, state, screenFor(state), 0);
  assert.ok(problems.includes("lab ve el remito y la factura"), problems.join("; "));
});

test("a negative verdict fails step 7 although the query succeeded", () => {
  const state = completeState(7);
  state.scenario.results[6].trace.verdict = { legitima: false, motivo: "TRANSFERENCIA_NO_AUTORIZADA" };
  const problems = stepEvidence(7, state, screenFor(state), 0);
  assert.ok(problems.some((p) => p.includes("TRANSFERENCIA_NO_AUTORIZADA")), problems.join("; "));
});

test("a missing signature or block fails the step", () => {
  const state = completeState(2);
  state.blocks.find((b) => b.transactions[0].function === "ReceiveTransfer").transactions[0].endorsers = ["drogueria"];
  assert.ok(stepEvidence(2, state, screenFor(state), 0).includes("ReceiveTransfer no tiene la firma de lab"));

  const missing = completeState(1);
  missing.blocks = [];
  assert.ok(stepEvidence(1, missing, screenFor(missing), 0).some((p) => p.includes("no llegó a la pantalla")));
});

test("the forbidden transfer must be rejected without a new block", () => {
  const state = completeState(4);
  const before = state.blocks.length;
  state.blocks.push({ number: 999, transactions: [] });
  assert.ok(stepEvidence(4, state, screenFor(state), before).includes("el intento prohibido agregó un bloque"));

  const accepted = completeState(4);
  accepted.scenario.results[3].rejection = undefined;
  assert.ok(
    stepEvidence(4, accepted, screenFor(accepted), accepted.blocks.length).some((p) => p.includes("no fue rechazado")),
  );
});

test("what the screen does not show fails the step", () => {
  const state = completeState(7);
  assert.ok(stepEvidence(7, state, screenFor(state, { verdictOk: false }), 0).includes("la pantalla no muestra el veredicto de PAMI"));
  assert.ok(stepEvidence(7, state, screenFor(state, { blocks: 5 }), 0).some((p) => p.includes("la pantalla muestra 5 bloques")));
});
