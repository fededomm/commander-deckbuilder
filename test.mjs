import assert from "node:assert/strict";
import { categorize, cardImage } from "./public/app.js";

assert.equal(categorize("Legendary Creature — Human Wizard"), "Creature");
assert.equal(categorize("Artifact Creature — Golem"), "Creature");
assert.equal(categorize("Land Creature — Forest Dryad"), "Land");   // Dryad Arbor
assert.equal(categorize("Artifact Land"), "Land");
assert.equal(categorize("Instant — Arcane"), "Instant");
assert.equal(categorize("Enchantment — Aura"), "Enchantment");
assert.equal(categorize("Creature — Elf Artificer"), "Creature");   // "Artificer" != Artifact
assert.equal(categorize("Kindred Sorcery — Goblin"), "Sorcery");
assert.equal(categorize(""), "Altro");

assert.equal(cardImage({ image_uris: { normal: "a" } }), "a");
assert.equal(cardImage({ card_faces: [{ image_uris: { normal: "b" } }] }), "b"); // doppia faccia
assert.equal(cardImage({}), "");

console.log("ok");

// --- prezzi Cardmarket (via Scryfall prices.eur) ---
const { price, deckTotals } = await import("./public/app.js");
assert.equal(price({ prices: { eur: "3.50" } }), 3.5);
assert.equal(price({ prices: { eur: null } }), null);   // carta non quotata
assert.equal(price({}), null);

const t = deckTotals([
  { price_eur: 10, purchased: true },
  { price_eur: 2.5, purchased: false },
  { price_eur: 0, purchased: false },   // senza quotazione
]);
assert.deepEqual(t, { total: 12.5, todo: 2.5, unpriced: 1 });
assert.deepEqual(deckTotals([]), { total: 0, todo: 0, unpriced: 0 });

// --- formato Moxfield ---
const { parseMoxfield, toMoxfield } = await import("./public/app.js");

const LISTA = `1 Inalla, Archmage Ritualist (SLD) 1639
1 Archmage Emeritus (STX) 377 *F*
1 Curiosity (PLST) A25-52
1 Ral, Crackling Wit (PBLB) 230p
1 Kefka, Court Mage / Kefka, Ruler of Ruin (FIN) 231
5 Island (SNC) 264
1 Sol Ring

// commento
Deck`;

const parsed = parseMoxfield(LISTA);
assert.equal(parsed.cards.length, 7);
assert.deepEqual(parsed.skipped, ["Deck"]);                       // intestazioni segnalate, non ingoiate
assert.equal(parsed.cards.reduce((n, c) => n + c.qty, 0), 11);    // 5 Island contano 5

assert.deepEqual(parsed.cards[1], { qty: 1, name: "Archmage Emeritus", set_code: "stx", collector_number: "377", foil: true });
assert.equal(parsed.cards[2].collector_number, "A25-52");         // numero col trattino (PLST)
assert.equal(parsed.cards[3].collector_number, "230p");           // numero con lettera
assert.equal(parsed.cards[4].name, "Kefka, Court Mage / Kefka, Ruler of Ruin"); // bifacciale
assert.deepEqual(parsed.cards[6], { qty: 1, name: "Sol Ring", set_code: "", collector_number: "", foil: false }); // senza stampa

// round-trip: quello che esporto rientra identico
const out = toMoxfield(parsed.cards.map((c) => ({ ...c, name: c.name.replace(" / ", " // ") })));
assert.equal(out, LISTA.split("\n").filter((l) => l && !l.startsWith("//") && l !== "Deck").join("\n"));
assert.deepEqual(parseMoxfield(out).cards, parsed.cards);

// prezzo foil quando c'è, altrimenti ripiega sul non foil
assert.equal(price({ prices: { eur: "1", eur_foil: "9" } }, true), 9);
assert.equal(price({ prices: { eur: "1", eur_foil: null } }, true), 1);
assert.equal(price({ prices: { eur: "1", eur_foil: "9" } }), 1);

// il totale moltiplica per la quantità
assert.deepEqual(deckTotals([{ price_eur: 0.1, qty: 5, purchased: false }]), { total: 0.5, todo: 0.5, unpriced: 0 });

// --- API + SQLite: server vero su DB temporaneo ---
const { spawn } = await import("node:child_process");
const { rmSync } = await import("node:fs");
const DB = "/tmp/deckbuilder-test.db";
rmSync(DB, { force: true });

const port = 8099;
const srv = spawn("node", ["server.mjs"], { env: { ...process.env, DB, PORT: port }, stdio: "ignore" });
const base = `http://localhost:${port}/api`;
const call = async (path, method = "GET", body) => {
  const r = await fetch(base + path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body && JSON.stringify(body),
  });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
};

for (let i = 0; i < 50; i++) {                       // attesa avvio
  try { await fetch(base + "/decks"); break } catch { await new Promise((r) => setTimeout(r, 100)) }
}

try {
  assert.deepEqual((await call("/decks")).body, []);
  assert.equal((await call("/decks", "POST", { name: "  " })).status, 400);   // nome vuoto

  const deck = (await call("/decks", "POST", { name: "Krenko" })).body;
  const card = { scryfall_id: "abc", name: "Sol Ring", type_line: "Artifact", price_eur: 1.55 };

  assert.deepEqual((await call(`/decks/${deck.id}/cards`, "POST", card)).body, { added: 1 });
  assert.equal((await call(`/decks/${deck.id}/cards`, "POST", card)).status, 200); // doppione ignorato
  assert.equal((await call(`/decks/999/cards`, "POST", card)).status, 404);        // mazzo inesistente
  assert.equal((await call(`/decks/${deck.id}`)).body.cards.length, 1);

  let [row] = (await call(`/decks/${deck.id}`)).body.cards;
  assert.equal(row.purchased, 0);
  assert.equal(row.price_eur, 1.55);

  await call(`/cards/${row.id}`, "PATCH", { purchased: true });
  assert.deepEqual((await call("/decks")).body[0], {
    id: deck.id, name: "Krenko", cards: 1, purchased: 1, total: 1.55, todo: 0,
  });

  // import in blocco + quantità: 5 Island contano 5 carte e 5x il prezzo
  assert.deepEqual((await call(`/decks/${deck.id}/cards`, "POST", [
    { scryfall_id: "isl", name: "Island", type_line: "Basic Land — Island", price_eur: 0.1, qty: 5,
      set_code: "snc", collector_number: "264" },
    { scryfall_id: "emeritus", name: "Archmage Emeritus", price_eur: 2, foil: true,
      set_code: "stx", collector_number: "377" },
  ])).body, { added: 2 });
  const home = (await call("/decks")).body[0];
  assert.equal(home.cards, 7);                    // 1 + 5 + 1
  assert.equal(home.total.toFixed(2), "4.05");    // 1.55 + 5*0.10 + 2.00
  assert.equal(home.todo.toFixed(2), "2.50");     // solo le non acquistate

  assert.equal((await call(`/decks/${deck.id}/cards`, "POST", [{ name: "senza id" }])).status, 400);
  assert.equal((await call(`/decks/${deck.id}`)).body.cards.length, 3); // il rollback non ha aggiunto nulla

  await call(`/decks/${deck.id}`, "DELETE");
  assert.deepEqual((await call("/decks")).body, []);
  assert.equal((await call(`/decks/${deck.id}`)).status, 404);

  // le carte se ne vanno col mazzo (ON DELETE CASCADE)
  const { DatabaseSync } = await import("node:sqlite");
  assert.equal(new DatabaseSync(DB).prepare("SELECT COUNT(*) n FROM cards").get().n, 0);

  console.log("api ok");
} finally {
  srv.kill();
}

// --- miniature ---
const { thumbUrl } = await import("./public/app.js");
assert.equal(thumbUrl("https://cards.scryfall.io/normal/front/5/8/abc.jpg"),
                      "https://cards.scryfall.io/small/front/5/8/abc.jpg");
assert.equal(thumbUrl("https://esempio/altro.jpg"), "https://esempio/altro.jpg"); // path diverso: invariata

// --- simboli di mana ---
const { manaHtml } = await import("./public/app.js");
assert.equal(manaHtml("{2}{U}{R}"),
  ['{2}', '{U}', '{R}'].map((s) => `<img class="sym" src="symbols/${s[1]}.svg" alt="${s}" loading="lazy">`).join(""));
assert.match(manaHtml("{W/P}"), /src="symbols\/WP\.svg"/);   // ibrido/phyrexian
assert.match(manaHtml("{∞}"), /src="symbols\/INFINITY\.svg"/); // nome file non derivabile dal simbolo
assert.equal(manaHtml("{NOPE}"), "{NOPE}");                   // sconosciuto: resta il testo
assert.equal(manaHtml(""), "");                               // terre: nessun costo
assert.equal(manaHtml(undefined), "");
