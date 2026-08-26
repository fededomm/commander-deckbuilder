import { api, post, patch, del } from "./api.js";
import { SYMBOLS } from "./symbols.js";

// Ordine di priorità: il primo tipo che matcha vince (Land prima di Creature
// così "Land Creature — Forest Dryad" finisce tra le terre).
const CATEGORIES = ["Land", "Creature", "Planeswalker", "Battle", "Instant", "Sorcery", "Artifact", "Enchantment"];

export function categorize(typeLine = "") {
  const main = typeLine.split("—")[0];
  return CATEGORIES.find((c) => main.includes(c)) || "Altro";
}

export function price(card, foil = false) {
  const p = foil ? card.prices?.eur_foil ?? card.prices?.eur : card.prices?.eur;
  return Number(p) || null; // prezzo Cardmarket, null se non quotata
}

/* ---------- formato Moxfield: "1 Sol Ring (EOC) 57 *F*" ---------- */

// set e numero sono opzionali; i flag in coda (*F* foil, *E* etched) possono essere più d'uno.
const MOXFIELD = /^\s*(\d+)x?\s+(.+?)(?:\s+\(([\w-]+)\)\s+(\S+))?((?:\s+\*\w+\*)*)\s*$/;

export function parseMoxfield(text) {
  const cards = [], skipped = [];
  for (const line of text.split(/\r?\n/)) {
    if (!line.trim() || line.trim().startsWith("//")) continue;   // vuote e commenti
    const m = MOXFIELD.exec(line);
    if (!m) { skipped.push(line.trim()); continue; }              // intestazioni tipo "Deck", "Sideboard"
    const [, qty, name, set, number, flags] = m;
    cards.push({
      qty: Number(qty),
      name: name.trim(),
      set_code: set?.toLowerCase() ?? "",
      collector_number: number ?? "",
      foil: /\*F\*/i.test(flags),
    });
  }
  return { cards, skipped };
}

export function toMoxfield(cards) {
  return cards
    .map((c) => {
      // Scryfall usa "A // B" per le bifacciali, Moxfield "A / B"
      const name = c.name.replace(" // ", " / ");
      const printing = c.set_code ? ` (${c.set_code.toUpperCase()}) ${c.collector_number}` : "";
      return `${c.qty ?? 1} ${name}${printing}${c.foil ? " *F*" : ""}`;
    })
    .join("\n");
}

// "{2}{U}{R}" -> gli SVG scaricati da Scryfall. Simbolo sconosciuto: resta il testo.
export function manaHtml(cost = "") {
  return cost.replace(/\{[^}]+\}/g, (sym) =>
    SYMBOLS[sym] ? `<img class="sym" src="symbols/${SYMBOLS[sym]}" alt="${sym}" loading="lazy">` : sym);
}

export const eur = (n) => n.toLocaleString("it-IT", { style: "currency", currency: "EUR" });

// Totale mazzo, quanto resta da comprare, e quante carte non hanno quotazione.
export function deckTotals(items) {
  let total = 0, todo = 0, unpriced = 0;
  for (const c of items) {
    if (!c.price_eur) { unpriced++; continue; }
    const line = c.price_eur * (c.qty ?? 1);
    total += line;
    if (!c.purchased) todo += line;
  }
  return { total, todo, unpriced };
}

// Risolve le righe Moxfield in stampe Scryfall: batch da 75, set+numero se c'è, altrimenti nome.
export async function resolvePrintings(parsed) {
  const rows = [], missing = [];
  for (let i = 0; i < parsed.length; i += 75) {
    const chunk = parsed.slice(i, i + 75);
    const r = await fetch("https://api.scryfall.com/cards/collection", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        identifiers: chunk.map((c) =>
          c.set_code ? { set: c.set_code, collector_number: c.collector_number } : { name: c.name }),
      }),
    });
    if (!r.ok) throw new Error(`Scryfall ${r.status}`);
    const { data = [] } = await r.json();

    const found = new Map();
    for (const c of data) {
      found.set(`${c.set}|${c.collector_number}`, c);
      found.set(c.name.toLowerCase(), c);
    }
    for (const p of chunk) {
      // Moxfield scrive "A / B" le bifacciali, Scryfall "A // B"
      const c = found.get(`${p.set_code}|${p.collector_number}`)
             ?? found.get(p.name.replace(" / ", " // ").toLowerCase());
      if (!c) { missing.push(p); continue; }
      rows.push({
        scryfall_id: c.id,
        name: c.name,
        type_line: c.type_line ?? "",
        mana_cost: c.mana_cost ?? "",
        image: cardImage(c),
        price_eur: price(c, p.foil),
        qty: p.qty,
        set_code: c.set,
        collector_number: c.collector_number,
        foil: p.foil,
      });
    }
  }
  return { rows, missing };
}

export function cardImage(card) {
  return (card.image_uris || card.card_faces?.[0]?.image_uris)?.normal || "";
}

// Stessa immagine in formato piccolo (~10 KB invece di ~100): con 100 carte
// in pagina è la differenza fra 1 MB e 10 MB. Se il path cambia resta il grande.
export const thumbUrl = (url) => url.replace("/normal/", "/small/");

/* ---------- pagina mazzo (deck.html) ---------- */

const deckId = new URLSearchParams(globalThis.location?.search ?? "").get("id");

let current = { name: "", cards: [] };

async function loadDeck() {
  current = await api(`/decks/${deckId}`);
  document.getElementById("deck-name").textContent = current.name;
  renderDeck(current.cards);
}

// Anteprima grande in basso a sinistra: riclicco la stessa carta e sparisce.
function togglePreview(src) {
  const el = document.getElementById("preview");
  if (!el.hidden && el.src === src) return (el.hidden = true);
  el.src = src;
  el.hidden = false;
}

function exportDeck() {
  const blob = new Blob([toMoxfield(current.cards)], { type: "text/plain" });
  const a = Object.assign(document.createElement("a"), {
    href: URL.createObjectURL(blob),
    download: `${current.name.replace(/[^\w -]/g, "") || "mazzo"}.txt`,
  });
  a.click();
  URL.revokeObjectURL(a.href);
}

async function search(q) {
  const results = document.getElementById("results");
  if (!q.trim()) return (results.innerHTML = "");
  const r = await fetch(`https://api.scryfall.com/cards/search?unique=cards&q=${encodeURIComponent(q)}`);
  if (!r.ok) return (results.innerHTML = "<p class='empty'>Nessuna carta trovata.</p>");
  const { data } = await r.json();
  results.innerHTML = "";
  for (const card of data.slice(0, 30)) {
    const el = document.createElement("button");
    el.className = "hit";
    el.innerHTML = `<img src="${cardImage(card)}" alt="" loading="lazy">
      <span><strong>${card.name}</strong><br><small>${card.type_line || ""}</small></span>
      <span class="cost">${price(card) ? eur(price(card)) : "—"}</span>`;
    el.onclick = () => add(card);
    results.append(el);
  }
}

async function add(card) {
  await post(`/decks/${deckId}/cards`, {
    scryfall_id: card.id,
    name: card.name,
    type_line: card.type_line || "",
    mana_cost: card.mana_cost || "",
    image: cardImage(card),
    price_eur: price(card),
  }); // già nel mazzo: il server fa INSERT OR IGNORE
  loadDeck();
}

// Riallinea i prezzi a Scryfall: endpoint batch, max 75 identifiers per richiesta.
async function refreshPrices() {
  const btn = document.getElementById("refresh");
  btn.disabled = true;
  const { cards: items } = await api(`/decks/${deckId}`);
  for (let i = 0; i < items.length; i += 75) {
    const chunk = items.slice(i, i + 75);
    const r = await fetch("https://api.scryfall.com/cards/collection", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ identifiers: chunk.map((c) => ({ id: c.scryfall_id })) }),
    });
    const { data = [] } = await r.json();
    const fresh = new Map(data.map((c) => [c.id, price(c) || 0]));
    await Promise.all(
      chunk
        .filter((c) => fresh.has(c.scryfall_id) && fresh.get(c.scryfall_id) !== c.price_eur)
        .map((c) => patch(`/cards/${c.id}`, { price_eur: fresh.get(c.scryfall_id) })),
    );
  }
  btn.disabled = false;
  loadDeck();
}

function renderDeck(items) {
  const deck = document.getElementById("deck");
  const qty = (cs) => cs.reduce((n, c) => n + (c.qty ?? 1), 0);
  document.getElementById("count").textContent =
    `${qty(items.filter((c) => c.purchased))} / ${qty(items)} acquistate`;

  const { total, todo, unpriced } = deckTotals(items);
  document.getElementById("total").textContent = items.length
    ? `Totale ${eur(total)} · da comprare ${eur(todo)}` + (unpriced ? ` · ${unpriced} senza quotazione` : "")
    : "";
  deck.innerHTML = "";

  const groups = new Map();
  for (const c of items) {
    const cat = categorize(c.type_line);
    if (!groups.has(cat)) groups.set(cat, []);
    groups.get(cat).push(c);
  }

  for (const cat of [...CATEGORIES, "Altro"]) {
    const group = groups.get(cat);
    if (!group) continue;
    const section = document.createElement("section");
    section.innerHTML = `<h3>${cat} <span>${qty(group)} · ${eur(deckTotals(group).total)}</span></h3>`;
    for (const c of group) {
      const row = document.createElement("div");
      row.className = "row" + (c.purchased ? " done" : "");
      row.innerHTML = `<input type="checkbox" ${c.purchased ? "checked" : ""}
                         title="Segna come acquistata" aria-label="Acquistata: ${c.name}">
        ${c.image ? `<img class="thumb" src="${thumbUrl(c.image)}" alt="" loading="lazy">` : `<span class="thumb"></span>`}
        <span class="name" title="${c.type_line || ""}">${c.qty > 1 ? `${c.qty}× ` : ""}${c.name}${c.foil ? " ✦" : ""}</span>
        <span class="cost mana">${manaHtml(c.mana_cost)}</span>
        <span class="cost price">${c.price_eur ? eur(c.price_eur) : "—"}</span>
        <button class="del" title="Rimuovi">×</button>`;
      row.querySelector("input").onchange = (e) =>
        patch(`/cards/${c.id}`, { purchased: e.target.checked }).then(loadDeck);
      if (c.image) {
        for (const el of row.querySelectorAll(".thumb, .name")) {
          el.classList.add("zoom");
          el.onclick = () => togglePreview(c.image);
        }
      }
      row.querySelector(".del").onclick = () => del(`/cards/${c.id}`).then(loadDeck);
      section.append(row);
    }
    deck.append(section);
  }
}

// Solo su deck.html: la home importa questo modulo per eur/deckTotals.
if (typeof document !== "undefined" && document.getElementById("q")) {
  if (!deckId) location.replace("index.html");

  let t;
  document.getElementById("q").oninput = (e) => {
    clearTimeout(t);
    t = setTimeout(() => search(e.target.value), 300); // rate limit Scryfall
  };
  document.getElementById("refresh").onclick = refreshPrices;
  document.getElementById("export").onclick = exportDeck;

  const preview = document.getElementById("preview");
  preview.onclick = () => (preview.hidden = true);
  addEventListener("keydown", (e) => e.key === "Escape" && (preview.hidden = true));
  loadDeck();
}
