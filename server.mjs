import { createServer } from "node:http";
import { DatabaseSync } from "node:sqlite";
import { readFile } from "node:fs/promises";
import { extname, join, normalize } from "node:path";

const PORT = process.env.PORT ?? 8090;
const ROOT = new URL("./public/", import.meta.url).pathname;

const db = new DatabaseSync(process.env.DB ?? "data.db");
db.exec(`
  PRAGMA foreign_keys = ON;
  CREATE TABLE IF NOT EXISTS decks (
    id      INTEGER PRIMARY KEY,
    name    TEXT NOT NULL,
    created TEXT NOT NULL DEFAULT (datetime('now'))
  );
  CREATE TABLE IF NOT EXISTS cards (
    id          INTEGER PRIMARY KEY,
    deck_id     INTEGER NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    scryfall_id TEXT NOT NULL,
    name        TEXT NOT NULL,
    type_line   TEXT NOT NULL DEFAULT '',
    mana_cost   TEXT NOT NULL DEFAULT '',
    image       TEXT NOT NULL DEFAULT '',
    price_eur   REAL NOT NULL DEFAULT 0,
    purchased   INTEGER NOT NULL DEFAULT 0,
    qty         INTEGER NOT NULL DEFAULT 1,
    set_code    TEXT NOT NULL DEFAULT '',
    collector_number TEXT NOT NULL DEFAULT '',
    foil        INTEGER NOT NULL DEFAULT 0,
    UNIQUE (deck_id, scryfall_id)
  );
`);

// DB creati prima dell'import Moxfield: aggiungo le colonne mancanti.
const have = new Set(db.prepare(`PRAGMA table_info(cards)`).all().map((c) => c.name));
for (const [col, def] of [
  ["qty", "INTEGER NOT NULL DEFAULT 1"],
  ["set_code", "TEXT NOT NULL DEFAULT ''"],
  ["collector_number", "TEXT NOT NULL DEFAULT ''"],
  ["foil", "INTEGER NOT NULL DEFAULT 0"],
]) if (!have.has(col)) db.exec(`ALTER TABLE cards ADD COLUMN ${col} ${def}`);

const q = (sql) => db.prepare(sql);
const CARD_COLS = ["scryfall_id", "name", "type_line", "mana_cost", "image", "price_eur",
                   "purchased", "qty", "set_code", "collector_number", "foil"];
const INT_COLS = new Set(["purchased", "foil"]);

// SQLite non ha booleani e non accetta null in colonne NOT NULL: normalizzo qui.
const coerce = (col, v) =>
  col === "price_eur" ? Number(v) || 0
  : col === "qty" ? Math.max(1, Number(v) || 1)
  : INT_COLS.has(col) ? +!!v
  : String(v ?? "");

// Le rotte: [metodo, regex sul path, handler(match, body)]. Un throw {code,msg} diventa la risposta d'errore.
const ROUTES = [
  ["GET", /^\/api\/decks$/, () =>
    q(`SELECT d.id, d.name,
              COALESCE(SUM(c.qty), 0)                             AS cards,
              COALESCE(SUM(c.qty * c.purchased), 0)               AS purchased,
              COALESCE(SUM(c.qty * c.price_eur), 0)               AS total,
              COALESCE(SUM(c.qty * c.price_eur * (1 - c.purchased)), 0) AS todo
       FROM decks d LEFT JOIN cards c ON c.deck_id = d.id
       GROUP BY d.id ORDER BY d.created DESC`).all()],

  ["POST", /^\/api\/decks$/, (_, b) => {
    const name = String(b.name ?? "").trim();
    if (!name) throw { code: 400, msg: "nome obbligatorio" };
    const { lastInsertRowid } = q(`INSERT INTO decks (name) VALUES (?)`).run(name);
    return { id: Number(lastInsertRowid), name };
  }],

  ["GET", /^\/api\/decks\/(\d+)$/, ([id]) => {
    const deck = q(`SELECT id, name FROM decks WHERE id = ?`).get(id);
    if (!deck) throw { code: 404, msg: "mazzo inesistente" };
    deck.cards = q(`SELECT * FROM cards WHERE deck_id = ? ORDER BY name`).all(id);
    return deck;
  }],

  ["DELETE", /^\/api\/decks\/(\d+)$/, ([id]) => {   // le carte seguono via ON DELETE CASCADE
    q(`DELETE FROM decks WHERE id = ?`).run(id);
    return null;
  }],

  ["POST", /^\/api\/decks\/(\d+)\/cards$/, ([id], b) => {
    if (!q(`SELECT 1 FROM decks WHERE id = ?`).get(id)) throw { code: 404, msg: "mazzo inesistente" };
    const rows = Array.isArray(b) ? b : [b];               // array = import Moxfield
    if (rows.some((r) => !r.scryfall_id || !r.name)) throw { code: 400, msg: "scryfall_id e name obbligatori" };
    // già nel mazzo: no-op, non un errore (l'utente ha ricliccato)
    const sql = q(`INSERT OR IGNORE INTO cards (deck_id, ${CARD_COLS}) VALUES (${["?", ...CARD_COLS.map(() => "?")]})`);
    db.exec("BEGIN");
    try {
      for (const r of rows) sql.run(Number(id), ...CARD_COLS.map((k) => coerce(k, r[k])));
      db.exec("COMMIT");
    } catch (e) {
      db.exec("ROLLBACK");
      throw e;
    }
    return { added: rows.length };
  }],

  ["PATCH", /^\/api\/cards\/(\d+)$/, ([id], b) => {
    const set = ["purchased", "price_eur", "qty"].filter((k) => k in b);
    if (!set.length) throw { code: 400, msg: "niente da aggiornare" };
    q(`UPDATE cards SET ${set.map((k) => `${k} = ?`)} WHERE id = ?`)
      .run(...set.map((k) => coerce(k, b[k])), id);
    return null;
  }],

  ["DELETE", /^\/api\/cards\/(\d+)$/, ([id]) => {
    q(`DELETE FROM cards WHERE id = ?`).run(id);
    return null;
  }],
];

const MIME = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml" };

async function serveStatic(pathname, res) {
  // join sotto ROOT + il check startsWith: blocca i path traversal tipo /../server.mjs
  const file = join(ROOT, pathname === "/" ? "index.html" : pathname);
  if (!file.startsWith(ROOT)) return res.writeHead(403).end();
  try {
    const body = await readFile(file);
    res.writeHead(200, { "content-type": MIME[extname(file)] ?? "application/octet-stream" });
    res.end(body);
  } catch {
    res.writeHead(404).end("not found");
  }
}

const handle = async (req, res) => {
  // niente new URL: su un path strano tipo "//" lancia, e qui un throw è un 500
  let pathname;
  try { pathname = normalize(decodeURIComponent(req.url.split("?")[0])) } catch { pathname = "/" }
  if (!pathname.startsWith("/api/")) return serveStatic(pathname, res);

  const route = ROUTES.find(([m, re]) => m === req.method && re.test(pathname));
  if (!route) return res.writeHead(404, { "content-type": "application/json" }).end(`{"error":"rotta sconosciuta"}`);

  let body = {};
  if (req.method !== "GET" && req.method !== "DELETE") {
    let raw = "";
    for await (const chunk of req) raw += chunk;
    try { body = raw ? JSON.parse(raw) : {}; } catch { return res.writeHead(400).end(`{"error":"json non valido"}`); }
  }

  try {
    const out = route[2](pathname.match(route[1]).slice(1), body);
    res.writeHead(out == null ? 204 : 200, { "content-type": "application/json" });
    res.end(out == null ? "" : JSON.stringify(out));
  } catch (e) {
    const code = e?.code >= 400 ? e.code : 500;
    if (code === 500) console.error(e);
    res.writeHead(code, { "content-type": "application/json" });
    res.end(JSON.stringify({ error: e?.msg ?? "errore interno" }));
  }
};

// una richiesta malformata non deve buttare giù il processo
export const server = createServer((req, res) =>
  handle(req, res).catch((e) => {
    console.error(e);
    if (!res.headersSent) res.writeHead(500);
    res.end();
  }),
);

// PORT=0 (Electron) => porta libera scelta dal sistema, la legge da server.address()
server.listen(PORT, () => console.log(`http://localhost:${server.address().port}`));
