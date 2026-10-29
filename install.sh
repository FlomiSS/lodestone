#!/usr/bin/env bash
# ==============================================================================
# Lodestone Installer
# https://github.com/Henny263462/lodestone
# ==============================================================================
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

echo -e "${BLUE}${BOLD}"
echo "  _             _           _                 "
echo " | |    ___   __| | ___ ___| |_ ___  _ __   ___ "
echo " | |   / _ \ / _\` |/ _ / __| __/ _ \| '_ \ / _ \\"
echo " | |__| (_) | (_| |  __\__ | || (_) | | | |  __/"
echo " |_____\___/ \__,_|\___|___/\__\___/|_| |_|\___|"
echo -e "${NC}"
echo -e "${CYAN}Lodestone Installation & Deployment Script${NC}\n"

# 1. Root-Check
if [ "${EUID:-$(id -u)}" -ne 0 ]; then
    echo -e "${RED}[Fehler] Dieses Skript benötigt Root-Rechte (führe es mit 'sudo' aus oder als Root).${NC}"
    exit 1
fi

# 2. Docker & Docker Compose prüfen / installieren
if ! command -v docker &> /dev/null; then
    echo -e "${CYAN}[Info] Docker ist nicht installiert. Installiere Docker via get.docker.com...${NC}"
    curl -fsSL https://get.docker.com | sh
    systemctl enable --now docker || service docker start || true
fi

if ! docker compose version &> /dev/null; then
    echo -e "${CYAN}[Info] Docker Compose Plugin wird installiert...${NC}"
    if command -v apt-get &> /dev/null; then
        apt-get update -qq && apt-get install -y -qq docker-compose-plugin
    elif command -v dnf &> /dev/null; then
        dnf install -y docker-compose-plugin
    elif command -v pacman &> /dev/null; then
        pacman -Sy --noconfirm docker-compose
    fi
fi

# 3. Verzeichnisse anlegen
INSTALL_DIR="/opt/lodestone"
DATA_DIR="/srv/lodestone"

mkdir -p "$INSTALL_DIR"
mkdir -p "$DATA_DIR"
chown 1000:1000 "$DATA_DIR"
cd "$INSTALL_DIR"

# 4. Compose-Datei & Updater laden
echo -e "${CYAN}[Info] Lade Docker Compose Konfiguration und Updater...${NC}"
curl -fsSL https://raw.githubusercontent.com/Henny263462/lodestone/main/deploy/docker-compose.release.yml -o docker-compose.yml
curl -fsSL https://raw.githubusercontent.com/Henny263462/lodestone/main/update.sh -o update.sh
chmod +x update.sh

# 5. .env Konfiguration generieren falls nicht vorhanden
if [ ! -f .env ]; then
    echo -e "${CYAN}[Info] Erstelle neue Konfiguration (.env)...${NC}"
    curl -fsSL https://raw.githubusercontent.com/Henny263462/lodestone/main/.env.example -o .env.example
    cp .env.example .env

    # Sicheres Token für Node-Panel Kommunikation generieren
    NODE_TOKEN=$(openssl rand -hex 32 2>/dev/null || tr -dc 'a-zA-Z0-9' < /dev/urandom | head -c 64)
    sed -i "s|^LODESTONE_NODE_TOKEN=.*|LODESTONE_NODE_TOKEN=${NODE_TOKEN}|" .env

    # GID des Docker-Sockets automatisch auslesen (verhindert Permission Denied)
    DOCKER_GID=$(stat -c %g /var/run/docker.sock 2>/dev/null || echo "999")
    sed -i "s|^DOCKER_GID=.*|DOCKER_GID=${DOCKER_GID}|" .env

    # Öffentliche Host-Adresse ermitteln
    PUBLIC_IP=$(curl -s -4 ifconfig.me 2>/dev/null || curl -s -4 icanhazip.com 2>/dev/null || hostname -I | awk '{print $1}')
    sed -i "s|^LODESTONE_PUBLIC_HOST=.*|LODESTONE_PUBLIC_HOST=${PUBLIC_IP}|" .env

    # Admin-Passwort generieren
    ADMIN_PASS=$(openssl rand -base64 12 2>/dev/null || tr -dc 'a-zA-Z0-9' < /dev/urandom | head -c 16)
    sed -i "s|^LODESTONE_ADMIN_PASSWORD=.*|LODESTONE_ADMIN_PASSWORD=${ADMIN_PASS}|" .env
fi

# 6. Container starten
echo -e "${CYAN}[Info] Ziehe neueste Images von ghcr.io und starte Lodestone...${NC}"
docker compose pull
docker compose up -d

# 7. Abschlussmeldung
ADMIN_PASS_CURRENT=$(grep '^LODESTONE_ADMIN_PASSWORD=' .env | cut -d '=' -f2)
PUBLIC_HOST_CURRENT=$(grep '^LODESTONE_PUBLIC_HOST=' .env | cut -d '=' -f2)
[ -z "$PUBLIC_HOST_CURRENT" ] && PUBLIC_HOST_CURRENT="<Server-IP>"

echo -e "\n${GREEN}${BOLD}✓ Lodestone wurde erfolgreich installiert und gestartet!${NC}\n"
echo -e "Weboberfläche:    ${CYAN}http://${PUBLIC_HOST_CURRENT}:8480${NC}"
echo -e "Benutzername:     ${BOLD}admin${NC}"
echo -e "Passwort:         ${BOLD}${ADMIN_PASS_CURRENT}${NC}"
echo -e "Verzeichnis:      ${BOLD}${INSTALL_DIR}${NC}"
echo -e "Server-Dateien:   ${BOLD}${DATA_DIR}${NC}"
echo -e "Auto-Updates:     ${GREEN}Aktiviert${NC} (Watchtower prüft stündlich auf neue Images)"
echo -e "Manuelles Update: ${BOLD}cd ${INSTALL_DIR} && sudo ./update.sh${NC}\n"
echo -e "${CYAN}Status prüfen:    cd ${INSTALL_DIR} && docker compose ps${NC}"
