# 02 – Aufbau und Ablauf der Oberfläche

Wie sich Lodestone **bedient**. Das Aussehen steht in `01`, der Code in `03`.

## 1. Informationsarchitektur

```
Übersicht (/)                      Projekte als Karten, Kennzahlen, Nodes
Projekt (/project/?id=)            Tabs: Server · Netzwerk · Gruppen (nur Netzwerke) · Zeitpläne · Ereignisse · Mitglieder · Einstellungen
  └─ Server (/server/?id=&tab=)    Zweite Seitenleiste: Konsole · Statistiken · Dateien · Plugins/Mods · Spieler · Backups · Einstellungen
Nodes (/nodes/)                    Maschinen, Speicher, playit
Benutzer (/users/)                 nur Administratoren
Protokoll (/audit/)                nur Administratoren
Konto (/account/)                  Passwort, Zwei-Faktor, API-Schlüssel, Sitzungen
Status (/status/?p=)               öffentlich, ohne Anmeldung, ohne Shell
Anmeldung (/login/)                ohne Shell
```

- **Projekt** ist die Ebene, auf der Rechte, Netzwerk, Gruppen, Zeitpläne und Webhooks liegen. **Server** ist die Ebene, auf der man
  mit einem einzelnen Minecraft-Server arbeitet. Neue Funktionen gehören auf die Ebene, zu der ihre Daten gehören.
- Neue Hauptseiten sind selten. Bevor du eine anlegst: Passt es als Tab im Projekt, als Bereich im Server oder als Karte im Konto?
  Eine neue Hauptseite braucht einen Eintrag in `shell.tsx` (Seitenleiste) **und** in `palette.tsx` (Strg+K).
- Navigation innerhalb eines Servers/Projekts geschieht über Tabs bzw. die zweite Seitenleiste; der aktive Tab steht in der Adresse
  (`?tab=…`), damit Neuladen und Links an derselben Stelle landen (siehe `server/page.tsx`).

## 2. Seitenaufbau (Schablone)

```
<PageHeader title="…" description="ein Satz, wofür die Seite da ist" action={<Button variant="primary">…</Button>} />
[Hinweis-/Fehlerkästen, falls nötig]
[Kennzahlen: grid mit StatCard]
[Hauptinhalt: Cards mit CardHeader]
```

- Reihenfolge von oben nach unten: **Was ist das → was ist der Zustand → was kann ich tun.**
- Pro Karte **ein** Thema. Lange Seiten in Karten gliedern, jede mit `CardHeader` (Titel + ein Satz Erklärung).
- Einstellungs-Karten: Titel, Beschreibung, Eingaben, **Speichern-Knopf unten rechts** in der Karte. Schalter (`Switch`) wirken sofort
  und brauchen keinen Speichern-Knopf; das steht in der Beschreibung.

## 3. Die vier Zustände jeder Datenansicht

```tsx
const { data, error, mutate } = useApi<Thing[]>("/things");
if (error) return <ErrorState error={error} retry={() => mutate()} />;   // 1. Fehler
if (!data) return <Loading />;                                             // 2. lädt
if (data.length === 0) return <EmptyState icon={…} title="Noch keine …" text="…" />;  // 3. leer
return …                                                                   // 4. voll
```

- **Leer** erklärt in einem Satz, was das hier ist, und bietet nach Möglichkeit den Knopf zum Anlegen an (nur wenn der Benutzer darf;
  sonst: „Ein Administrator muss …“).
- **Fehler** zeigt die Meldung des Backends (die ist schon deutsch und erklärt). Nie selbst „Fehler 500“ ausgeben.
- Live-Daten (Zustand, Konsole, Spielerzahl) aktualisieren sich selbst: `useApi(…, { refreshInterval: 3000–15000 })`, Websocket nur für
  die Konsole. Nie die ganze Seite neu laden.

## 4. Aktionen und Rückmeldung

| Art | Ablauf |
| --- | --- |
| Harmlos, sofort (Start, Backup, Speichern) | Knopf → `toast.run(() => api.post(…), "Kurze Erfolgsmeldung")` → `mutate()` |
| Dauert (Installation, Modpack, Backup) | Knopf zeigt `loading`; danach Zustand per `StateBadge`/Fortschrittskasten; nicht blockieren |
| Gefährlich (Löschen, Zurücksetzen, Überschreiben, Kick/Ban) | `ConfirmDialog danger`: Titel nennt das Objekt („„Survival“ löschen?“), Text sagt, **was verloren geht**, Knopf heißt wie die Tat („Löschen“, nicht „Ja“) |
| Eingabe nötig (Anlegen, Umbenennen) | `Dialog` mit Formular, Primärknopf rechts unten, `Abbrechen` als `ghost` links davon |
| Sofort wirksamer Schalter | `Switch` + `toast.run`, bei Fehler springt er zurück (Daten neu laden) |

- Erfolgsmeldungen sind kurz und im Perfekt/Zustand: „Gespeichert“, „Node entfernt“, „Backup gestartet“.
- Nach jeder Änderung die betroffenen Daten neu laden (`mutate()`), nicht lokal raten.
- Nie `alert`, `confirm`, `prompt`. Nie eine Aktion ohne sichtbare Rückmeldung.

## 5. Formulare

- Beschriftung immer über dem Feld (`label`), Platzhalter nur als Beispiel („Lobby“), nie als Ersatz für die Beschriftung.
- Primärknopf ist deaktiviert (`disabled`), solange Pflichtfelder fehlen; **kein** roter Fehler, bevor jemand etwas getan hat.
- Prüfe im Browser nur zur Hilfe (Länge, Muster); die Pflichtprüfung macht das Backend. Dessen Fehlermeldung wird per Toast gezeigt.
- Auswahlfelder: `<select className="field">`. Mehrwahl: `Checkbox`. Zahl mit Einheit: Einheit im Label („Arbeitsspeicher (MB)“) oder
  Auswahlliste mit schon formatierten Werten (`megabytes()` aus `lib/format`).
- Geheimnisse (Tokens, Schlüssel, Passwörter): `type="password"` bzw. einmalig anzeigen mit Kopierknopf und dem Hinweis, dass sie
  nicht wieder erscheinen. Nie in der Adresse, nie im Log.

## 6. Rechte in der Oberfläche

Rollen pro Projekt: `viewer` < `operator` < `admin` < `owner`; globale Administratoren (`user.isAdmin`) sind überall Eigentümer.
`roleRank` aus `lib/types.ts` vergleicht sie.

| Darf | Mindestens |
| --- | --- |
| Konsole/Spieler/Statistiken ansehen | Betrachter |
| Start/Stopp, Befehle, Dateien, Backups, Spieleraktionen | Operator |
| Einstellungen, Zeitpläne, Webhooks, Mitglieder, Gruppen | Admin |
| Projekt löschen | Eigentümer |
| Benutzer, Protokoll, Nodes anlegen | globaler Administrator |

Ausblenden, wenn die Funktion nie in Frage kommt (Tab „Einstellungen“ für Operatoren); deaktivieren mit Begründung, wenn es nur gerade
nicht geht („Der Server muss laufen“). Das Backend lehnt trotzdem ab.

## 7. Sprache und Texte (Wörterliste)

Immer **du**, aktiv, konkret. Vorhandene Begriffe benutzen, keine Synonyme einführen:

| Begriff | Nicht |
| --- | --- |
| Server, Projekt, Netzwerk, Gruppe, Node | Instanz (außer „Instanzen“ einer Gruppe), Cluster, Workspace |
| Konsole, Dateien, Plugins / Mods, Spieler, Backups, Einstellungen | Terminal, File Manager, Addons (nur im Code) |
| Arbeitsspeicher (kurz „RAM“ ist ok in Zahlen) | Speicher allein (mehrdeutig mit Festplatte) |
| Anlegen, Hinzufügen, Entfernen, Löschen | Erstellen/Create, Remove, Delete |
| Zeitplan, Webhook, Protokoll, API-Schlüssel | Cronjob, Hook, Audit-Log |
| Absturz, Neustart, Auto-Restart | Crash, Reboot |

- Zahlen/Zeit nur über `lib/format.ts` (`bytes`, `megabytes`, `ago`, `compact` …), Datum deutsch (`28.10.2026, 14:05`), Dezimalkomma.
- Titel: Substantiv oder Tätigkeit, ohne Punkt. Beschreibungen: ganzer Satz mit Punkt. Knöpfe: ein Verb oder „Verb + Objekt“.
- Fehler: **Ursache + nächster Schritt.** „Der Node ist nicht erreichbar. Prüfe, ob er läuft.“
- Leerzustand: „Noch kein/e …“ + Erklärung. Kein „Keine Daten“.

## 8. Typische Abläufe als Vorlage

**Server anlegen** (`components/add-server.tsx`): Dialog mit Tabs (Software · Modrinth-Modpack · ZIP importieren), Vorlagen als Chips,
Name, Software, Version, Arbeitsspeicher, Node, EULA; Primärknopf „Anlegen“ erst aktiv, wenn alles Nötige gewählt ist; danach Toast
und Weiterleitung zum Server.

**Gefährliche Aktion** (`components/server/settings.tsx`, Löschen/Neu installieren): eigene Karte mit rotem Knopf (`variant="danger"`) → `ConfirmDialog` mit Name und Folgen.

**Liste mit Aktionen** (`app/(app)/users/page.tsx`): `PageHeader` mit „Hinzufügen“, `Card` mit Zeilen, `RowMenu` pro Zeile, `ConfirmDialog` fürs Entfernen.

**Verlauf/Protokoll** (`app/(app)/audit/page.tsx`): Filter oben, Zeilen mit Zeit, Person, Tat, „Mehr laden“ unten.

**Live-Seite** (`components/server/console.tsx`): Websocket, Werkzeugleiste (Suche, Filter), Hinweise zu bekannten Fehlern als Kästen darüber.

Wenn deine Aufgabe einem dieser Abläufe ähnelt: Datei öffnen, Aufbau übernehmen, Inhalte austauschen.
