// Anteprima grande in basso a destra: riclicco la stessa carta e sparisce.
// Delegato sul document perché htmx rimpiazza le righe di continuo.
addEventListener("click", (e) => {
  const preview = document.getElementById("preview");
  if (!preview) return;
  if (e.target === preview) return (preview.hidden = true);

  const el = e.target.closest("[data-preview]");
  const src = el?.dataset.preview;
  if (!src) return;
  if (!preview.hidden && preview.src === src) return (preview.hidden = true);
  preview.src = src;
  // L'anteprima è il contenuto, non una decorazione: senza nome è muta.
  preview.alt = el.dataset.previewAlt ? "Carta: " + el.dataset.previewAlt : "";
  preview.hidden = false;
});

addEventListener("keydown", (e) => {
  const preview = document.getElementById("preview");
  if (e.key === "Escape" && preview) preview.hidden = true;
});

// La <dialog> delle ristampe si apre solo da JS: showModal() è quello che porta
// backdrop, Esc e focus trap. Chiuderla invece è nativo (form method="dialog").
addEventListener("htmx:afterSwap", (e) => {
  if (e.target.id === "prints" && !e.target.open) e.target.showModal();
});

// Scelta una ristampa la dialog ha finito il suo lavoro: la chiudo. Solo sulle
// POST, altrimenti il cambio pagina — che è una GET dentro la dialog — la chiuderebbe.
addEventListener("htmx:afterRequest", (e) => {
  const dialog = e.target.closest?.("dialog");
  if (dialog?.open && e.detail.successful && e.detail.requestConfig.verb === "post") {
    dialog.close();
  }
});

// Connessione che resta aperta finché la pagina è aperta: quando chiudi la
// finestra cade e il server si ferma da solo. Fra una pagina e l'altra si
// riapre da sé, e il server aspetta qualche secondo prima di arrendersi.
new EventSource("/alive");

// Focus sulla ricerca solo quando sta accanto alla checklist. Sotto i 900px è in
// fondo alla pagina: autofocus faceva aprire il mazzo già scrollato giù, tastiera
// compresa. Stessa soglia della @media in style.css.
addEventListener("DOMContentLoaded", () => {
  if (matchMedia("(min-width: 901px)").matches) document.getElementById("q")?.focus();
});

// --- filtro e "seleziona tutte" sulle carte del mazzo ---------------------
// Il filtro è solo lato client: le righe sono già nella pagina, basta nasconderle.

function visibleChecks() {
  return [...document.querySelectorAll("#deck .row:not([hidden]) .check")];
}

// La casella "tutte" rispecchia le righe visibili: piena, vuota o a metà.
function syncCheckAll() {
  const all = document.getElementById("check-all");
  if (!all) return;
  const boxes = visibleChecks();
  const n = boxes.filter((b) => b.checked).length;
  all.checked = boxes.length > 0 && n === boxes.length;
  all.indeterminate = n > 0 && n < boxes.length;
}

function applyFilter() {
  const deck = document.getElementById("deck");
  if (!deck) return;
  const q = (document.getElementById("filter")?.value || "").trim().toLowerCase();
  for (const row of deck.querySelectorAll(".row")) {
    const name = row.querySelector(".name");
    // nel title c'è il tipo: "creature" o "instant" filtrano come il nome
    row.hidden = q !== "" && !(name.textContent + " " + name.title).toLowerCase().includes(q);
  }
  for (const s of deck.querySelectorAll("section")) {
    s.hidden = !s.querySelector(".row:not([hidden])");
  }
  document.getElementById("filter-empty").hidden = !!deck.querySelector(".row:not([hidden])");
  syncCheckAll();
}

addEventListener("input", (e) => {
  if (e.target.id === "filter") applyFilter();
});

// "Tutte": spunto subito le caselle nel browser e mando una richiesta sola con
// gli id cambiati. Stessa coda (hx-sync su #deck) delle spunte singole.
addEventListener("change", (e) => {
  const el = e.target;
  if (el.id !== "check-all") {
    if (el.classList.contains("check")) syncCheckAll();
    return;
  }
  const on = el.checked;
  const ids = visibleChecks()
    .filter((b) => b.checked !== on)
    .map((b) => ((b.checked = on), b.dataset.id));
  if (ids.length === 0) return;
  htmx.ajax("POST", el.dataset.url, {
    source: el,
    swap: "none",
    values: { ids: ids.join(","), purchased: on ? "1" : "" },
  });
});

// Aggiunta, rimozione e prezzi ridisegnano la checklist: il filtro resta (hx-preserve)
// e va riapplicato alle righe nuove.
addEventListener("htmx:afterSettle", applyFilter);
addEventListener("DOMContentLoaded", applyFilter);
