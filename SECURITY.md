# Sicherheit

## Sicherheitslücken melden

Bitte melde Sicherheitslücken **nicht** als öffentliches Issue. Nutze stattdessen auf GitHub
**Security → Report a vulnerability** (privater Sicherheitshinweis). Beschreibe, was passiert, wie man es nachstellt und welche Version betroffen ist. Wir antworten, sobald wir können, und stimmen eine Veröffentlichung mit dir ab.

## Hinweise zum Betrieb

- Der Node-Daemon hat Zugriff auf `docker.sock` und damit weitreichende Rechte auf der Maschine. Mache seine API nie öffentlich zugänglich, sondern betreibe sie im internen Netz oder hinter TLS/VPN.
- Das gemeinsame Token von Panel und Node ist ein Geheimnis (`openssl rand -hex 32`).
- Stelle das Panel im Internet nur hinter einem Reverse-Proxy mit HTTPS bereit und setze `LODESTONE_SECURE_COOKIES=true` und `LODESTONE_TRUST_PROXY=true`.
- Der playit-Schlüssel wird auf dem Node gespeichert und nie ausgegeben.
- Vor dem Öffnen für fremde Benutzer: unabhängige Sicherheitsprüfung (siehe `docs/architecture.md`).