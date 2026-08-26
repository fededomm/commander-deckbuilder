// Scarica gli SVG dei simboli di mana da Scryfall in public/symbols/
// e genera public/symbols.js con la mappa simbolo -> file.
// Da rilanciare solo quando Scryfall aggiunge simboli nuovi: node symbols.mjs
import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";

const UA = { "User-Agent": "commander-deckbuilder/0.1 (https://github.com/local)", Accept: "*/*" };
const DIR = new URL("./public/symbols/", import.meta.url).pathname;

const r = await fetch("https://api.scryfall.com/symbology", { headers: UA });
if (!r.ok) throw new Error(`symbology: HTTP ${r.status}`);
const { data } = await r.json();

await mkdir(DIR, { recursive: true });
const map = {};
let bytes = 0;
for (const s of data) {
  if (!s.svg_uri) continue;
  const file = s.svg_uri.split("/").pop().split("?")[0];   // il nome file lo decide Scryfall
  const svg = await fetch(s.svg_uri, { headers: UA });
  if (!svg.ok) { console.warn(`salto ${s.symbol}: HTTP ${svg.status}`); continue; }
  const body = Buffer.from(await svg.arrayBuffer());
  await writeFile(join(DIR, file), body);
  map[s.symbol] = file;
  bytes += body.length;
}

await writeFile(
  new URL("./public/symbols.js", import.meta.url),
  `// Generato da symbols.mjs — non modificare a mano.\nexport const SYMBOLS = ${JSON.stringify(map, null, 2)};\n`,
);
console.log(`${Object.keys(map).length} simboli, ${(bytes / 1024).toFixed(0)} KB in public/symbols/`);
