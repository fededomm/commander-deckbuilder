#!/usr/bin/env bash
# Costruisce i pacchetti distribuibili in dist/.
#   Windows: installer NSIS per utente singolo (serve makensis)
#   macOS:   bundle .app zippato per arm64 e amd64; il .dmg solo se giri su un Mac
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-0.2.0}"
APPNAME="Commander Deckbuilder"
OUT=dist

rm -rf "$OUT"
mkdir -p "$OUT/windows"
go generate ./...

# --- Windows -----------------------------------------------------------------
# -H windowsgui: niente finestra nera del prompt all'avvio. In cambio non c'è
# stderr, per questo il binario scrive deckbuilder.log accanto al database.
go tool go-winres simply --arch amd64 --icon packaging/icon.png --manifest gui \
  --product-name "$APPNAME" --file-description "$APPNAME" \
  --product-version "$VERSION" --file-version "$VERSION" --original-filename deckbuilder.exe
trap 'rm -f rsrc_windows_*.syso' EXIT
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H windowsgui" \
  -o "$OUT/windows/deckbuilder.exe"
rm -f rsrc_windows_*.syso

# MAKENSIS=/percorso/makensis se non è installato a sistema (serve anche NSISDIR).
MAKENSIS="${MAKENSIS:-makensis}"
if command -v "$MAKENSIS" >/dev/null 2>&1; then
  "$MAKENSIS" -V2 "-DVERSION=$VERSION" packaging/installer.nsi
else
  echo "!! makensis non trovato, salto l'installer Windows (sudo apt install nsis)" >&2
fi

# --- macOS -------------------------------------------------------------------
for arch in arm64 amd64; do
  app="$OUT/macos/$arch/$APPNAME.app"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
  CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -ldflags="-s -w" \
    -o "$app/Contents/MacOS/deckbuilder"
  cp packaging/icon.icns "$app/Contents/Resources/icon.icns"
  cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>              <string>$APPNAME</string>
	<key>CFBundleDisplayName</key>       <string>$APPNAME</string>
	<key>CFBundleIdentifier</key>        <string>it.local.commander-deckbuilder</string>
	<key>CFBundleVersion</key>           <string>$VERSION</string>
	<key>CFBundleShortVersionString</key><string>$VERSION</string>
	<key>CFBundleExecutable</key>        <string>deckbuilder</string>
	<key>CFBundleIconFile</key>          <string>icon</string>
	<key>CFBundlePackageType</key>       <string>APPL</string>
	<key>LSMinimumSystemVersion</key>    <string>11.0</string>
	<key>NSHighResolutionCapable</key>   <true/>
</dict>
</plist>
PLIST
  # zip conservando il bit di esecuzione: senza, il .app non parte.
  python3 - "$OUT/macos/$arch" "$APPNAME.app" \
    "$OUT/CommanderDeckbuilder-$VERSION-macos-$arch.zip" <<'PY'
import os, sys, zipfile
root, name, out = sys.argv[1], sys.argv[2], sys.argv[3]
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
    for dirpath, _, files in os.walk(os.path.join(root, name)):
        for f in files:
            full = os.path.join(dirpath, f)
            info = zipfile.ZipInfo(os.path.relpath(full, root))
            info.external_attr = (os.stat(full).st_mode & 0xFFFF) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            with open(full, "rb") as fh:
                z.writestr(info, fh.read())
PY
done

# Il .dmg vuole hdiutil, che esiste solo su macOS: se sei lì lo faccio, altrimenti
# resta lo zip. Vedi il README per firma e notarizzazione.
if [ "$(uname)" = "Darwin" ]; then
  for arch in arm64 amd64; do
    hdiutil create -volname "$APPNAME" -srcfolder "$OUT/macos/$arch" -ov -format UDZO \
      "$OUT/CommanderDeckbuilder-$VERSION-macos-$arch.dmg"
  done
fi

echo
echo "Pronto in $OUT/:"
ls -1sh "$OUT" | grep -v '^total' | grep -vE 'windows|macos' || true
