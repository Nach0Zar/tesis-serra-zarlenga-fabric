// Evidencia que cada paso del guion tiene que dejar en la red y en la
// pantalla (docs/defense-demo.md). Un ensayo sólo es correcto si, además de
// terminar sin error, cada paso produjo lo que la demo dice que muestra.

// Por paso: transacciones esperadas, con la función y las organizaciones que
// tienen que haber firmado el bloque como mínimo.
export const EXPECTED_TRANSACTIONS = {
  1: [{ function: "RegisterUnit", signers: ["lab"] }],
  2: [
    { function: "DispatchTransfer", signers: ["lab"] },
    { function: "ReceiveTransfer", signers: ["lab", "drogueria"] },
  ],
  3: [
    { function: "DispatchTransfer", signers: ["drogueria"] },
    { function: "ReceiveTransfer", signers: ["drogueria", "farmacia"] },
  ],
  4: [],
  5: [{ function: "Dispense", signers: ["farmacia"] }],
  6: [{ function: "ReportStolen", signers: ["farmacia"] }],
  7: [],
};

const PRIVACY = { 2: ["lab", "drogueria"], 3: ["drogueria", "farmacia"] };

// stepEvidence compara el estado de demo-server (GET /api/state) y lo que la
// pantalla muestra después del paso. blocksBefore es la cantidad de bloques
// antes de ejecutarlo. Devuelve la lista de problemas; vacía si el paso dejó
// toda su evidencia.
export function stepEvidence(number, state, screen, blocksBefore) {
  const problems = [];
  const scenario = state.scenario;
  const result = [...scenario.results].reverse().find((r) => r.step === number);
  if (!result || !result.ok) {
    problems.push(`el paso ${number} no terminó bien`);
    return problems;
  }

  const expected = EXPECTED_TRANSACTIONS[number];
  if (result.transactions.length !== expected.length) {
    problems.push(`se esperaban ${expected.length} transacciones y hubo ${result.transactions.length}`);
  }
  expected.forEach((want, index) => {
    const transaction = result.transactions[index];
    if (!transaction || transaction.function !== want.function) {
      problems.push(`falta la transacción ${want.function}`);
      return;
    }
    const block = state.blocks.find((b) => b.number === transaction.blockNumber);
    const onChain = block && block.transactions.find((t) => t.transactionId === transaction.transactionId);
    if (!onChain) {
      problems.push(`el bloque #${transaction.blockNumber} de ${want.function} no llegó a la pantalla`);
      return;
    }
    if (!onChain.valid) problems.push(`${want.function} quedó inválida: ${onChain.validationCode}`);
    for (const signer of want.signers) {
      if (!onChain.endorsers.includes(signer)) problems.push(`${want.function} no tiene la firma de ${signer}`);
    }
  });

  if (number === 4) {
    if (!result.rejection || result.rejection.code !== "TRANSFER_NOT_AUTHORIZED") {
      problems.push("el intento prohibido no fue rechazado con TRANSFER_NOT_AUTHORIZED");
    }
    if (state.blocks.length !== blocksBefore) problems.push("el intento prohibido agregó un bloque");
    if (!screen.rejectionVisible) problems.push("la pantalla no muestra el rechazo");
  }

  if (PRIVACY[number]) {
    const privacy = result.privacy;
    const [from, to] = PRIVACY[number];
    const members = new Set([from, to, "anmat"]);
    if (!privacy || !privacy.hash) {
      problems.push("falta el panel de privacidad o la huella del bloque");
    } else {
      for (const entry of privacy.organizations) {
        if (entry.error) problems.push(`el peer de ${entry.organization} no respondió`);
        else if (entry.sees !== members.has(entry.organization)) {
          problems.push(`${entry.organization} ${entry.sees ? "ve" : "no ve"} el remito y la factura`);
        }
      }
      if (privacy.organizations.length !== 7) problems.push("el panel de privacidad no tiene las 7 organizaciones");
    }
    if (screen.privacySees !== 3) problems.push(`la pantalla muestra ${screen.privacySees} organizaciones que ven la factura`);
  }

  if (number === 6) {
    const alerts = state.alerts.filter(
      (alert) => alert.eventName === "ReportStolen" && alert.unit && alert.unit.numeroSerie === scenario.theftSerial,
    );
    if (alerts.length !== 1) problems.push(`ANMAT recibió ${alerts.length} alertas del robo, no 1`);
    if (screen.alerts !== 1) problems.push(`la pantalla muestra ${screen.alerts} alertas`);
  }

  if (number === 7) {
    const verdict = result.trace && result.trace.verdict;
    if (!verdict || verdict.legitima !== true) {
      problems.push(`PAMI no verificó la traza: ${verdict ? verdict.motivo : "sin veredicto"}`);
    }
    if (!screen.verdictOk) problems.push("la pantalla no muestra el veredicto de PAMI");
  }

  if (screen.blocks !== state.blocks.length) {
    problems.push(`la pantalla muestra ${screen.blocks} bloques y la red confirmó ${state.blocks.length}`);
  }
  return problems;
}
