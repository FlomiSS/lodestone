#!/usr/bin/env bash
# ==============================================================================
# Lodestone Updater
# https://github.com/Henny263462/lodestone
# ==============================================================================
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# Zielverzeichnis ermitteln
TARGET_DIR="${1:-/opt/lodestone}"
if [ ! -d "$TARGET_DIR" ] && [ -f "./docker-compose.yml" ]; then
    TARGET_DIR="$(pwd)"
fi

if [ ! -d "$TARGET_DIR" ]; then
    echo -e "${RED}[Fehler] Lodestone-Verzeichnis ($TARGET_DIR) nicht gefunden.${NC}"
    exit 1
fi

cd "$TARGET_DIR"
echo -e "${CYAN}${BOLD}[Info] Starte Update für Lodestone in ${TARGET_DIR}...${NC}"

if [ ! -f "docker-compose.yml" ]; then
    echo -e "${RED}[Fehler] Keine docker-compose.yml in $TARGET_DIR gefunden.${NC}"
    exit 1
fi

# 1. Neueste Images von ghcr.io ziehen
echo -e "${CYAN}[1/3] Ziehe neueste Images von GitHub Container Registry (ghcr.io)...${NC}"
docker compose pull

# 2. Container mit neuen Images neu starten
echo -e "${CYAN}[2/3] Aktualisiere Container (ohne Datenverlust)...${NC}"
docker compose up -d --remove-orphans

# 3. Alte nicht mehr genutzte Images aufräumen
echo -e "${CYAN}[3/3] Bereinige alte Images...${NC}"
docker image prune -f

echo -e "\n${GREEN}${BOLD}✓ Lodestone wurde erfolgreich auf die neueste Version aktualisiert!${NC}\n"
docker compose ps
