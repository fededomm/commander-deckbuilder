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
