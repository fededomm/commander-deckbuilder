// Anteprima grande in basso a sinistra: riclicco la stessa carta e sparisce.
// Delegato sul document perché htmx rimpiazza le righe di continuo.
addEventListener("click", (e) => {
  const preview = document.getElementById("preview");
  if (!preview) return;
  if (e.target === preview) return (preview.hidden = true);

  const src = e.target.closest("[data-preview]")?.dataset.preview;
  if (!src) return;
  if (!preview.hidden && preview.src === src) return (preview.hidden = true);
  preview.src = src;
  preview.hidden = false;
});

addEventListener("keydown", (e) => {
  const preview = document.getElementById("preview");
  if (e.key === "Escape" && preview) preview.hidden = true;
});

// Connessione che resta aperta finché la pagina è aperta: quando chiudi la
// finestra cade e il server si ferma da solo. Fra una pagina e l'altra si
// riapre da sé, e il server aspetta qualche secondo prima di arrendersi.
new EventSource("/alive");
