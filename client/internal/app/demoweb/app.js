// Pantalla de la demo de la defensa (DEMO-3, #145). Todo lo que muestra llega
// de demo-server: respuestas del contrato, bloques confirmados y alertas. Los
// datos se insertan siempre como texto, nunca como HTML.
"use strict";

const ROLES = {
  lab: "fabrica",
  drogueria: "distribuye",
  distribuidor: "distribuye",
  farmacia: "dispensa",
  centromedico: "atiende pacientes",
  financiador: "paga y audita",
  anmat: "regula",
};

const OPERATIONS = {
  RegisterUnit: "Registro",
  DispatchTransfer: "Despacho",
  ReceiveTransfer: "Recepción",
  Dispense: "Dispensa",
  ReportStolen: "Robo informado",
  Quarantine: "Cuarentena",
  ReportExpired: "Vencimiento informado",
  ReportLost: "Extravío informado",
};

const ALERTS = {
  ReportStolen: "ROBO",
  ReportLost: "EXTRAVÍO",
  ReportExpired: "VENCIMIENTO",
  Quarantine: "CUARENTENA",
};

const STATES = {
  EN_LABORATORIO: "En el laboratorio",
  EN_TRANSITO: "En tránsito",
  EN_CUSTODIA: "En custodia",
  DISPENSADO: "Dispensada al paciente",
  ROBADO: "Robada",
};

const CHECKS = {
  EXISTENCIA: "Existe",
  ESTADO_DISPENSADO: "Fue dispensada",
  DISPENSADOR_HABILITADO: "Dispensador habilitado",
  SECUENCIA_ESTADOS: "Secuencia válida",
  PARES_AUTORIZADOS: "Traspasos permitidos",
};

// Qué se está mostrando mientras corre cada paso, y qué queda escrito al
// terminar. El guion completo está en docs/defense-demo.md.
const CAPTIONS = {
  1: ["El laboratorio registra la unidad…", "El laboratorio registró la unidad: un bloque firmado por su peer."],
  2: ["El laboratorio despacha a la droguería…", "La droguería recibió la unidad: el traspaso lo firmaron las dos partes."],
  3: ["La droguería despacha a la farmacia…", "La farmacia recibió la unidad. El remito y la factura sólo los ven las partes y ANMAT."],
  4: ["La farmacia intenta venderle a una droguería…", "La red rechazó la operación antes de registrarla: no se agregó ningún bloque."],
  5: ["La farmacia dispensa la unidad…", "La unidad llegó al paciente."],
  6: ["La farmacia informa el robo de otra unidad…", "ANMAT recibió la alerta en el momento."],
  7: ["PAMI verifica la traza antes de pagar…", "PAMI verificó la traza completa de la unidad."],
};

const ROUTE = [
  { key: "lab", label: "Laboratorio" },
  { key: "drogueria", label: "Droguería" },
  { key: "farmacia", label: "Farmacia" },
  { key: "paciente", label: "Paciente" },
];

const view = {
  network: null,
  scenario: null,
  blocks: [],
  alerts: [],
  running: 0,
  transit: null,
};

const $ = (id) => document.getElementById(id);

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function icon(name) {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("class", "icon");
  svg.setAttribute("aria-hidden", "true");
  const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
  use.setAttribute("href", "#i-" + name);
  svg.appendChild(use);
  return svg;
}

function label(slug) {
  const organization = view.network && view.network.organizations.find((o) => o.slug === slug);
  return organization ? organization.label : slug;
}

function slugForID(id) {
  const organization = view.network && view.network.organizations.find((o) => o.id === id);
  return organization ? organization.slug : "";
}

function setCaption(text) {
  $("caption").textContent = text;
}

// --- Red -----------------------------------------------------------------------

function renderNetwork() {
  const orgs = $("orgs");
  orgs.replaceChildren();
  for (const organization of view.network.organizations) {
    const card = el("div", "org");
    card.id = "org-" + organization.slug;
    card.append(el("div", "name", organization.label), el("div", "role", ROLES[organization.slug] || ""));
    const badges = el("div", "badges");
    badges.id = "badges-" + organization.slug;
    card.append(badges);
    orgs.append(card);
  }
  const orderers = $("orderers");
  orderers.replaceChildren();
  for (const orderer of view.network.orderers) {
    orderers.append(el("span", "orderer", label(orderer.organization)));
  }
  renderCustody();
}

// La organización que tiene la unidad se marca con el borde y el texto de su
// tarjeta, sin cambiar su altura.
function renderCustody() {
  if (!view.network) return;
  const unit = view.scenario && view.scenario.unit;
  const custodian = unit && unit.estado !== "DISPENSADO" && !view.transit ? slugForID(unit.custodioActual) : "";
  for (const organization of view.network.organizations) {
    const card = $("org-" + organization.slug);
    if (!card) continue;
    const role = card.querySelector(".role");
    const holds = organization.slug === custodian;
    card.classList.toggle("custodian", holds);
    role.replaceChildren();
    if (holds) role.append(icon("box"), document.createTextNode("tiene la unidad"));
    else role.textContent = ROLES[organization.slug] || "";
  }
}

function flashSigners(endorsers) {
  for (const slug of endorsers) {
    const card = $("org-" + slug);
    const badges = $("badges-" + slug);
    if (!card || !badges) continue;
    card.classList.add("signing");
    const badge = el("span", "badge signed");
    badge.append(icon("pen"), document.createTextNode("firmó"));
    badges.append(badge);
    setTimeout(() => {
      card.classList.remove("signing");
      badge.remove();
    }, 2200);
  }
  for (const orderer of document.querySelectorAll(".orderer")) {
    orderer.classList.add("pulse");
    setTimeout(() => orderer.classList.remove("pulse"), 1200);
  }
}

// --- Unidad --------------------------------------------------------------------

function routePosition() {
  if (view.transit) return { here: -1, transitTo: view.transit.to };
  const unit = view.scenario && view.scenario.unit;
  if (!unit) return { here: -1, transitTo: -1 };
  if (unit.estado === "DISPENSADO") return { here: 3, transitTo: -1 };
  const index = ROUTE.findIndex((stop) => stop.key === slugForID(unit.custodioActual));
  return { here: index, transitTo: -1 };
}

function renderUnit() {
  const scenario = view.scenario;
  $("unit-id").textContent = scenario ? scenario.gtin + " · " + scenario.serial : "";
  const position = routePosition();
  const reached = position.here >= 0 ? position.here : position.transitTo - 1;
  const route = $("route");
  route.replaceChildren();
  ROUTE.forEach((stop, index) => {
    const item = el("div", "stop");
    if (index === position.here) item.classList.add("here");
    else if (index === position.transitTo) item.classList.add("transit-to");
    else if (index <= reached) item.classList.add("visited");
    const node = el("div", "node");
    if (index === position.here) node.append(icon("box"));
    else if (index <= reached) node.append(icon("check"));
    item.append(node, el("div", "label", stop.label));
    route.append(item);
  });

  const status = $("unit-status");
  status.replaceChildren();
  const unit = scenario && scenario.unit;
  if (view.transit) {
    status.append(document.createTextNode("En tránsito hacia " + ROUTE[view.transit.to].label));
  } else if (!unit) {
    status.append(document.createTextNode("Todavía no registrada"));
  } else {
    const where = unit.estado === "DISPENSADO" ? "" : " · " + label(slugForID(unit.custodioActual));
    status.append(document.createTextNode((STATES[unit.estado] || unit.estado) + where));
  }
  renderTrace();
  renderCustody();
}

function renderTrace() {
  const trace = view.scenario && view.scenario.trace;
  const container = $("trace");
  container.replaceChildren();
  if (!trace) {
    container.hidden = true;
    return;
  }
  container.hidden = false;
  const verdict = trace.verdict || {};
  const history = Array.isArray(trace.history) ? trace.history.length : 0;
  const title = el("div", "verdict " + (verdict.legitima ? "ok" : "bad"));
  title.append(
    icon(verdict.legitima ? "check" : "cross"),
    document.createTextNode(
      (verdict.legitima ? "PAMI: traza legítima" : "PAMI: traza no válida (" + verdict.motivo + ")") +
        " · " + history + " registros en el historial",
    ),
  );
  const list = el("ul");
  for (const check of verdict.verificaciones || []) {
    const ok = check.resultado === "OK";
    const item = el("li", ok ? "ok" : "bad");
    item.append(icon(ok ? "check" : "cross"), document.createTextNode(CHECKS[check.check] || check.check));
    list.append(item);
  }
  container.append(title, list);
}

// --- Cadena --------------------------------------------------------------------

function blockElement(block, fresh) {
  const transactions = block.transactions.length ? block.transactions : [{ function: "", endorsers: [], valid: true }];
  const transaction = transactions[0];
  const item = el("li", "block" + (transaction.valid ? "" : " invalid") + (fresh ? " new" : ""));
  item.append(el("div", "number", "Bloque #" + block.number));
  item.append(el("div", "operation", OPERATIONS[transaction.function] || transaction.function || "Configuración"));
  if (!transaction.valid) item.append(el("div", "number", "inválida: " + transaction.validationCode));
  const signers = el("div", "signers");
  for (const slug of transaction.endorsers) {
    const badge = el("span", "badge signed");
    badge.append(icon("pen"), document.createTextNode(label(slug)));
    signers.append(badge);
  }
  item.append(signers);
  return item;
}

function renderBlocks() {
  $("chain-empty").hidden = view.blocks.length > 0;
  const list = $("blocks");
  list.replaceChildren(...view.blocks.map((block) => blockElement(block, false)));
  list.scrollLeft = list.scrollWidth;
}

function addBlock(block) {
  view.blocks.push(block);
  $("chain-empty").hidden = true;
  const list = $("blocks");
  list.append(blockElement(block, true));
  list.scrollLeft = list.scrollWidth;
  const transaction = block.transactions[0];
  if (!transaction) return;
  flashSigners(transaction.endorsers);
  // Durante una transferencia, el despacho y la recepción llegan como dos
  // bloques: la unidad se muestra en tránsito entre uno y otro.
  if (view.running === 2 || view.running === 3) {
    if (transaction.function === "DispatchTransfer") {
      view.transit = { to: view.running === 2 ? 1 : 2 };
      renderUnit();
    } else if (transaction.function === "ReceiveTransfer") {
      view.transit = null;
      renderUnit();
    }
  }
}

function renderRejection() {
  const box = $("rejection");
  const results = (view.scenario && view.scenario.results) || [];
  const rejected = results.find((result) => result.rejection);
  box.replaceChildren();
  if (!rejected) {
    box.hidden = true;
    return;
  }
  box.hidden = false;
  const title = el("div", "title");
  title.append(icon("cross"), document.createTextNode("Rechazado"));
  box.append(
    title,
    el("div", "detail", "Farmacia → Droguería no está permitido por la normativa. No se agregó ningún bloque."),
    el("div", "code mono", rejected.rejection.code),
  );
}

// --- Privacidad ----------------------------------------------------------------

function renderPrivacy() {
  const privacy = view.scenario && view.scenario.privacy;
  const latest = privacy && privacy.length ? privacy[privacy.length - 1] : null;
  const list = $("privacy");
  list.replaceChildren();
  $("privacy-empty").hidden = Boolean(latest);
  $("privacy-transfer").textContent = latest ? label(latest.from) + " → " + label(latest.to) : "";
  if (!latest) return;
  const ordered = [...latest.organizations].sort((a, b) => Number(b.sees) - Number(a.sees));
  for (const entry of ordered) {
    const item = el("li");
    item.append(el("span", "who", label(entry.organization)));
    if (entry.sees) {
      const sees = el("span", "sees");
      sees.append(icon("check"), document.createTextNode(" Remito " + entry.remito + " · Factura " + entry.factura));
      item.append(sees);
    } else if (entry.error) {
      item.append(el("span", "hidden-data", "sin respuesta del peer"));
    } else {
      const hidden = el("span", "hidden-data");
      hidden.append(icon("lock"), document.createTextNode(" sólo la huella "));
      hidden.append(el("span", "hash mono", latest.hash.slice(0, 8) + "…"));
      item.append(hidden);
    }
    list.append(item);
  }
}

// --- ANMAT ---------------------------------------------------------------------

function alertElement(alert) {
  const box = el("div", "alert");
  const title = el("div", "title");
  title.append(icon("alert"), document.createTextNode((ALERTS[alert.eventName] || alert.eventName) + " informado"));
  const unit = alert.unit || {};
  const who = unit.custodioActual ? " por " + label(slugForID(unit.custodioActual)) : "";
  box.append(
    title,
    el("div", "detail", "Unidad " + (unit.numeroSerie || "") + who),
    el("div", "detail", "Bloque #" + alert.blockNumber + " · recibida " + new Date(alert.receivedAt).toLocaleTimeString("es-AR")),
  );
  return box;
}

function renderAlerts() {
  const container = $("alerts");
  container.replaceChildren();
  if (!view.alerts.length) {
    container.append(el("p", "empty", "Sin alertas."));
    return;
  }
  for (const alert of [...view.alerts].reverse()) container.append(alertElement(alert));
}

// --- Controles -----------------------------------------------------------------

function renderSteps() {
  const scenario = view.scenario;
  const steps = $("steps");
  steps.replaceChildren();
  if (!scenario) return;
  const ready = scenario.preparation === "LISTA";
  for (const step of scenario.steps) {
    const button = el("button", "", step.number + ". " + step.title);
    button.type = "button";
    if (step.done) button.classList.add("done");
    if (view.running === step.number) {
      button.classList.add("running");
      button.textContent = step.number + ". Firmando…";
    } else if (step.number === scenario.nextStep) {
      button.classList.add("next");
    }
    button.disabled = !ready || view.running !== 0 || step.number !== scenario.nextStep;
    button.addEventListener("click", () => runStep(step.number));
    steps.append(button);
  }
  $("reset").disabled = view.running !== 0;
}

function renderOverlay() {
  const scenario = view.scenario;
  const overlay = $("overlay");
  const box = $("overlay-text");
  box.replaceChildren();
  if (!scenario || scenario.preparation === "LISTA") {
    overlay.hidden = true;
    return;
  }
  overlay.hidden = false;
  if (scenario.preparation === "FALLIDA") {
    box.append(document.createTextNode("No se pudo preparar el escenario."));
    const detail = el("span", "detail", scenario.preparationError ? scenario.preparationError.message : "");
    box.append(detail);
  } else {
    box.append(document.createTextNode("Preparando el escenario en la red…"));
  }
}

function renderScenario() {
  renderUnit();
  renderRejection();
  renderPrivacy();
  renderSteps();
  renderOverlay();
}

function renderAll() {
  renderNetwork();
  renderBlocks();
  renderAlerts();
  renderScenario();
}

async function runStep(number) {
  if (view.running) return;
  view.running = number;
  setCaption(CAPTIONS[number][0]);
  renderSteps();
  try {
    const response = await fetch("api/steps/" + number, { method: "POST" });
    const result = await response.json();
    if (result.ok) {
      setCaption(CAPTIONS[number][1]);
    } else {
      setCaption("Error: " + (result.error ? result.error.message : "respuesta inesperada"));
    }
  } catch (error) {
    setCaption("Error de conexión con el servidor de la demo.");
  } finally {
    view.running = 0;
    view.transit = null;
    renderScenario();
  }
}

async function reset() {
  if (view.running) return;
  if (!window.confirm("¿Empezar un ensayo nuevo?")) return;
  setCaption("Preparando un ensayo nuevo…");
  await fetch("api/reset", { method: "POST" });
}

// --- Eventos del servidor ------------------------------------------------------

function setConnection(connected) {
  const box = $("connection");
  box.classList.toggle("online", connected);
  box.classList.toggle("offline", !connected);
  $("connection-text").textContent = connected ? "red en línea" : "sin conexión con la red";
}

function connect() {
  const events = new EventSource("api/events");
  events.addEventListener("snapshot", (event) => {
    const snapshot = JSON.parse(event.data);
    view.network = snapshot.network;
    view.scenario = snapshot.scenario;
    view.blocks = snapshot.blocks;
    view.alerts = snapshot.alerts;
    setConnection(snapshot.connected);
    if (snapshot.scenario.preparation === "LISTA" && !view.running) {
      setCaption(snapshot.scenario.nextStep === 1 ? "Listo para empezar." : "");
    }
    renderAll();
  });
  events.addEventListener("scenario", (event) => {
    const previous = view.scenario && view.scenario.preparation;
    view.scenario = JSON.parse(event.data);
    if (previous !== "LISTA" && view.scenario.preparation === "LISTA" && view.scenario.nextStep === 1) {
      setCaption("Listo para empezar.");
    }
    renderScenario();
  });
  events.addEventListener("block", (event) => addBlock(JSON.parse(event.data)));
  events.addEventListener("alert", (event) => {
    view.alerts.push(JSON.parse(event.data));
    renderAlerts();
  });
  events.addEventListener("connection", (event) => setConnection(JSON.parse(event.data).connected));
  events.onerror = () => setConnection(false);
}

// Atajos para quien maneja la pantalla: espacio o flecha derecha ejecutan el
// paso siguiente.
document.addEventListener("keydown", (event) => {
  if (event.target instanceof HTMLButtonElement) return;
  if ((event.key === " " || event.key === "ArrowRight") && view.scenario && !view.running) {
    event.preventDefault();
    if (view.scenario.preparation === "LISTA" && view.scenario.nextStep <= view.scenario.steps.length) {
      runStep(view.scenario.nextStep);
    }
  }
});

$("reset").addEventListener("click", reset);
connect();
