package panel

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func currentCode(t *testing.T, secret string, offset int64) string {
	t.Helper()
	raw, err := base32NoPad.DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return totpCode(raw, time.Now().Unix()/totpStep+offset)
}

func TestTotpRfc6238Vectors(t *testing.T) {
	// Testwerte aus RFC 6238 (Anhang B), SHA-1, auf sechs Stellen gekürzt
	secret := []byte("12345678901234567890")
	for unix, want := range map[int64]string{59: "287082", 1111111109: "081804", 1111111111: "050471", 1234567890: "005924", 2000000000: "279037"} {
		if got := totpCode(secret, unix/30); got != want {
			t.Errorf("t=%d: %s, erwartet %s", unix, got, want)
		}
	}
	if _, ok := totpMatch(secret, "287082", time.Unix(59, 0)); !ok {
		t.Error("der richtige Code muss passen")
	}
	if _, ok := totpMatch(secret, "287082", time.Unix(59+90, 0)); ok {
		t.Error("ein Code von vor 90 Sekunden darf nicht mehr gelten")
	}
	if _, ok := totpMatch(secret, "28708", time.Unix(59, 0)); ok {
		t.Error("zu kurze Codes passen nicht")
	}
}

func TestTwoFactorLogin(t *testing.T) {
	_, web, _, _ := setupPanel(t)
	admin := newClient(t, web.URL)
	admin.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})

	var setup struct{ Secret, URI string }
	_ = json.Unmarshal(admin.expect(200, "POST", "/api/me/totp/setup", nil), &setup)
	if !strings.HasPrefix(setup.URI, "otpauth://totp/") || !strings.Contains(setup.URI, setup.Secret) {
		t.Fatalf("Adresse für die App: %s", setup.URI)
	}
	admin.expect(400, "POST", "/api/me/totp/enable", map[string]string{"code": "000000"})
	var enabled struct{ RecoveryCodes []string }
	_ = json.Unmarshal(admin.expect(200, "POST", "/api/me/totp/enable", map[string]string{"code": currentCode(t, setup.Secret, 0)}), &enabled)
	if len(enabled.RecoveryCodes) != 8 {
		t.Fatalf("Wiederherstellungscodes: %v", enabled.RecoveryCodes)
	}
	admin.expect(409, "POST", "/api/me/totp/enable", map[string]string{"code": currentCode(t, setup.Secret, 0)})

	login := func() (*client, string) {
		c := newClient(t, web.URL)
		var step struct {
			NeedsTotp bool
			Ticket    string
		}
		_ = json.Unmarshal(c.expect(200, "POST", "/api/auth/login", map[string]string{"username": "admin", "password": "ein-gutes-passwort"}), &step)
		if !step.NeedsTotp || step.Ticket == "" {
			t.Fatalf("mit 2FA gibt es noch keine Sitzung: %+v", step)
		}
		c.expect(401, "GET", "/api/me", nil) // nach dem Passwort allein ist man nicht angemeldet
		return c, step.Ticket
	}

	// falscher Code, gefälschtes Ticket
	c, ticket := login()
	c.expect(401, "POST", "/api/auth/totp", map[string]string{"ticket": ticket, "code": "123456"})
	c.expect(401, "POST", "/api/auth/totp", map[string]string{"ticket": ticket + "x", "code": currentCode(t, setup.Secret, 0)})
	c.expect(401, "POST", "/api/auth/totp", map[string]string{"ticket": "irgendwas", "code": currentCode(t, setup.Secret, 0)})

	// richtiger Code
	code := currentCode(t, setup.Secret, 0)
	c.expect(200, "POST", "/api/auth/totp", map[string]string{"ticket": ticket, "code": code})
	c.expect(200, "GET", "/api/me", nil)

	// derselbe Code gilt nur einmal
	c2, ticket2 := login()
	c2.expect(401, "POST", "/api/auth/totp", map[string]string{"ticket": ticket2, "code": code})

	// ein Wiederherstellungscode geht einmal
	c3, ticket3 := login()
	c3.expect(200, "POST", "/api/auth/totp", map[string]string{"ticket": ticket3, "code": enabled.RecoveryCodes[0]})
	c4, ticket4 := login()
	c4.expect(401, "POST", "/api/auth/totp", map[string]string{"ticket": ticket4, "code": enabled.RecoveryCodes[0]})

	// Ausschalten braucht Passwort und Code
	c3.expect(403, "POST", "/api/me/totp/disable", map[string]string{"password": "falsch-falsch", "code": enabled.RecoveryCodes[1]})
	c3.expect(403, "POST", "/api/me/totp/disable", map[string]string{"password": "ein-gutes-passwort", "code": "000000"})
	c3.expect(204, "POST", "/api/me/totp/disable", map[string]string{"password": "ein-gutes-passwort", "code": enabled.RecoveryCodes[1]})
	plain := newClient(t, web.URL)
	plain.expect(200, "POST", "/api/auth/login", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})
	plain.expect(200, "GET", "/api/me", nil)
}

func TestTwoFactorAttemptsAreThrottled(t *testing.T) {
	_, web, _, _ := setupPanel(t)
	admin := newClient(t, web.URL)
	admin.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})
	var setup struct{ Secret string }
	_ = json.Unmarshal(admin.expect(200, "POST", "/api/me/totp/setup", nil), &setup)
	admin.expect(200, "POST", "/api/me/totp/enable", map[string]string{"code": currentCode(t, setup.Secret, 0)})

	c := newClient(t, web.URL)
	var step struct{ Ticket string }
	_ = json.Unmarshal(c.expect(200, "POST", "/api/auth/login", map[string]string{"username": "admin", "password": "ein-gutes-passwort"}), &step)
	for i := 0; i < throttleLimit; i++ {
		c.expect(401, "POST", "/api/auth/totp", map[string]string{"ticket": step.Ticket, "code": "000000"})
	}
	// Jetzt ist auch der richtige Code gesperrt (kein Durchprobieren der eine Million Codes)
	c.expect(429, "POST", "/api/auth/totp", map[string]string{"ticket": step.Ticket, "code": currentCode(t, setup.Secret, 0)})
}

// bearer schickt eine Anfrage mit API-Schlüssel, ohne Cookie und ohne Schutz-Header.
func bearer(t *testing.T, base, method, path, key string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, _ := http.NewRequest(method, base+path, reader)
	request.Header.Set("Authorization", "Bearer "+key)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, data
}

func TestAPIKeys(t *testing.T) {
	p, web, _, _ := setupPanel(t)
	admin := newClient(t, web.URL)
	admin.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})

	var created struct {
		Key  string
		Info APIKey
	}
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/me/keys", map[string]any{"name": "Skript"}), &created)
	if !strings.HasPrefix(created.Key, "lds_") || len(created.Key) < 40 || created.Info.Prefix != created.Key[:10] {
		t.Fatalf("Schlüssel: %+v", created)
	}
	// in der Datenbank steht nur der Hash
	var stored string
	_ = p.store.db.QueryRow("SELECT hash FROM api_keys").Scan(&stored)
	if stored == created.Key || strings.Contains(stored, created.Key[4:]) {
		t.Fatal("der Klartext darf nicht gespeichert sein")
	}

	// Der Schlüssel wirkt wie der Benutzer – ohne CSRF-Header, aber nicht für Zugangs-Einstellungen
	if status, _ := bearer(t, web.URL, "GET", "/api/projects", created.Key, nil); status != 200 {
		t.Fatalf("Liste mit Schlüssel: %d", status)
	}
	if status, _ := bearer(t, web.URL, "POST", "/api/projects", created.Key, map[string]string{"name": "Per API", "kind": "server"}); status != 201 {
		t.Fatalf("Projekt per API anlegen: %d", status)
	}
	for _, path := range []string{"/api/me/keys", "/api/me/sessions", "/api/me/totp/setup"} {
		method := "GET"
		if strings.HasSuffix(path, "setup") {
			method = "POST"
		}
		if status, _ := bearer(t, web.URL, method, path, created.Key, map[string]any{"name": "x"}); status != 403 {
			t.Errorf("%s %s mit Schlüssel: %d, erwartet 403", method, path, status)
		}
	}
	if status, _ := bearer(t, web.URL, "GET", "/api/projects", "lds_"+strings.Repeat("0", 48), nil); status != 401 {
		t.Errorf("falscher Schlüssel: %d", status)
	}

	// Der Zugriff steht im Protokoll, und die Nutzung wird vermerkt
	entries, _ := p.store.ListAudit(50, 0, "")
	found := false
	for _, entry := range entries {
		found = found || (strings.Contains(entry.Action, "POST /api/projects") && strings.Contains(entry.Action, "API-Schlüssel"))
	}
	if !found {
		t.Errorf("Protokoll: %+v", entries)
	}
	keys, _ := p.store.ListAPIKeys(created.Info.UserID)
	if len(keys) != 1 || keys[0].LastUsed == 0 {
		t.Errorf("Nutzung: %+v", keys)
	}

	// Widerrufen und Ablauf
	admin.expect(204, "DELETE", "/api/me/keys/"+created.Info.ID, nil)
	if status, _ := bearer(t, web.URL, "GET", "/api/projects", created.Key, nil); status != 401 {
		t.Errorf("widerrufener Schlüssel: %d", status)
	}
	admin.expect(404, "DELETE", "/api/me/keys/"+created.Info.ID, nil)
	var expiring struct{ Key string }
	_ = json.Unmarshal(admin.expect(201, "POST", "/api/me/keys", map[string]any{"name": "kurz", "expiresDays": 1}), &expiring)
	_, _ = p.store.db.Exec("UPDATE api_keys SET expires_at = ?", time.Now().Add(-time.Minute).UnixMilli())
	if status, _ := bearer(t, web.URL, "GET", "/api/projects", expiring.Key, nil); status != 401 {
		t.Errorf("abgelaufener Schlüssel: %d", status)
	}
	admin.expect(400, "POST", "/api/me/keys", map[string]any{"name": ""})
}

func TestSessionsCanBeListedAndEnded(t *testing.T) {
	_, web, _, _ := setupPanel(t)
	first := newClient(t, web.URL)
	first.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})
	second := newClient(t, web.URL)
	second.expect(200, "POST", "/api/auth/login", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})

	var list []SessionInfo
	_ = json.Unmarshal(first.expect(200, "GET", "/api/me/sessions", nil), &list)
	if len(list) != 2 {
		t.Fatalf("zwei Sitzungen erwartet: %+v", list)
	}
	var other SessionInfo
	current := 0
	for _, info := range list {
		if info.Current {
			current++
		} else {
			other = info
		}
		if info.IP == "" {
			t.Errorf("die IP fehlt: %+v", info)
		}
	}
	if current != 1 {
		t.Fatalf("genau eine ist die eigene: %+v", list)
	}
	first.expect(204, "DELETE", "/api/me/sessions/"+other.ID, nil)
	second.expect(401, "GET", "/api/me", nil)
	first.expect(200, "GET", "/api/me", nil)
	first.expect(404, "DELETE", "/api/me/sessions/"+other.ID, nil)
	first.expect(404, "DELETE", "/api/me/sessions/zu-kurz", nil)
}

func TestAuditLog(t *testing.T) {
	p, web, _, _ := setupPanel(t)
	admin := newClient(t, web.URL)
	admin.expect(201, "POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "ein-gutes-passwort"})
	admin.expect(201, "POST", "/api/users", map[string]any{"username": "anna", "password": "ein-gutes-passwort"})
	admin.expect(201, "POST", "/api/projects", map[string]string{"name": "Geheim", "kind": "server"})
	admin.expect(400, "POST", "/api/projects", map[string]string{"name": "", "kind": "server"})
	other := newClient(t, web.URL)
	other.expect(401, "POST", "/api/auth/login", map[string]string{"username": "anna", "password": "falsch-falsch"})
	other.expect(200, "POST", "/api/auth/login", map[string]string{"username": "anna", "password": "ein-gutes-passwort"})

	var entries []AuditEntry
	_ = json.Unmarshal(admin.expect(200, "GET", "/api/audit", nil), &entries)
	has := func(action string, status int) bool {
		for _, entry := range entries {
			if strings.Contains(entry.Action, action) && (status == 0 || entry.Status == status) {
				return true
			}
		}
		return false
	}
	if !has("POST /api/users", 201) || !has("POST /api/projects", 201) || !has("POST /api/projects", 400) || !has("Anmeldung fehlgeschlagen", 401) || !has("Anmeldung", 200) {
		t.Fatalf("Protokoll unvollständig: %+v", entries)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Target, "passwort") || strings.Contains(entry.Action, "ein-gutes-passwort") {
			t.Errorf("im Protokoll steht etwas Geheimes: %+v", entry)
		}
		if entry.Username == "" && entry.Action != "Anmeldung fehlgeschlagen" {
			t.Errorf("ohne Benutzer: %+v", entry)
		}
	}
	// nach Benutzer filtern, seitenweise lesen, Rechte
	var anna []AuditEntry
	_ = json.Unmarshal(admin.expect(200, "GET", "/api/audit?user=anna", nil), &anna)
	if len(anna) == 0 || anna[0].Username != "anna" {
		t.Errorf("Filter: %+v", anna)
	}
	var page []AuditEntry
	_ = json.Unmarshal(admin.expect(200, "GET", "/api/audit?limit=2", nil), &page)
	if len(page) != 2 {
		t.Fatalf("Seite: %+v", page)
	}
	var next []AuditEntry
	_ = json.Unmarshal(admin.expect(200, "GET", "/api/audit?limit=2&before="+itoaInt64(page[1].ID), nil), &next)
	if len(next) == 0 || next[0].ID >= page[1].ID {
		t.Errorf("nächste Seite: %+v", next)
	}
	other.expect(403, "GET", "/api/audit", nil)

	// Aufräumen
	_, _ = p.store.db.Exec("UPDATE audit SET ts = 1")
	_ = p.store.PurgeAudit(time.Now().UnixMilli())
	if left, _ := p.store.ListAudit(10, 0, ""); len(left) != 0 {
		t.Errorf("alte Einträge müssen weg: %+v", left)
	}
}

func itoaInt64(n int64) string {
	data, _ := json.Marshal(n)
	return string(data)
}
