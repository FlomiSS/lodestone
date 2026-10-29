#!/usr/bin/env sh
# Baut die Oberfläche und legt sie dort ab, wo das Panel sie einbettet.
set -eu
cd "$(dirname "$0")/../web"
[ -d node_modules ] || npm ci
npx next build
rm -rf ../internal/panel/webui/dist
cp -r out ../internal/panel/webui/dist
touch ../internal/panel/webui/dist/.gitkeep
