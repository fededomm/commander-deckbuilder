import { app, BrowserWindow, shell } from "electron";
import { once } from "node:events";
import { join } from "node:path";

// Il DB sta nella cartella dati dell'utente: dentro l'app pacchettizzata la
// directory di installazione è di sola lettura.
process.env.DB = join(app.getPath("userData"), "data.db");
process.env.PORT = "0"; // porta libera: niente conflitti con un `node server.mjs` già aperto

const { server } = await import("./server.mjs");
await once(server, "listening");
const url = `http://localhost:${server.address().port}`;

app.whenReady().then(() => {
  const win = new BrowserWindow({
    width: 1280,
    height: 860,
    backgroundColor: "#16151a", // evita il lampo bianco all'avvio
    title: "Commander Deckbuilder",
    webPreferences: { nodeIntegration: false, contextIsolation: true },
  });
  win.removeMenu();
  win.loadURL(url);

  // i link esterni (Scryfall, Cardmarket) vanno nel browser, non in una finestra Electron
  win.webContents.setWindowOpenHandler(({ url }) => (shell.openExternal(url), { action: "deny" }));

  app.on("activate", () => BrowserWindow.getAllWindows().length === 0 && win.show());
});

app.on("window-all-closed", () => process.platform !== "darwin" && app.quit());
