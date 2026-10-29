package panel

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------- Geheimer Schlüssel des Panels

// secretKey liefert den Schlüssel, mit dem TOTP-Geheimnisse verschlüsselt und kurzlebige Tickets signiert werden. Er
// liegt in der Datenordner-Datei secret.key (wird beim ersten Bedarf angelegt, nur für den Daemon lesbar).
func (p *Panel) secretKey() ([]byte, error) {
	p.keyMu.Lock()
	defer p.keyMu.Unlock()
	if p.key != nil {
		return p.key, nil
	}
	if p.cfg.DataDir != "" {
		path := filepath.Join(p.cfg.DataDir, "secret.key")
		if data, err := os.ReadFile(path); err == nil && len(data) == 32 {
			p.key = data
			return p.key, nil
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, key, 0o600); err != nil {
			return nil, err
		}
		p.key = key
		return key, nil
	}
	key := make([]byte, 32) // ohne Datenordner (Tests): nur für diesen Start
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	p.key = key
	return key, nil
}

func (p *Panel) derive(purpose string) ([]byte, error) {
	key, err := p.secretKey()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(append(append([]byte{}, key...), purpose...))
	return sum[:], nil
}

func (p *Panel) seal(plain string) (string, error) {
	key, err := p.derive("totp")
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func (p *Panel) unseal(text string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return "", err
	}
	key, err := p.derive("totp")
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("beschädigt")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	return string(plain), err
}

// ---------------------------------------------------------------- TOTP (RFC 6238)

const totpStep = 30

func totpCode(secret []byte, counter int64) string {
	mac := hmac.New(sha1.New, secret)
	var buffer [8]byte
	binary.BigEndian.PutUint64(buffer[:], uint64(counter))
	mac.Write(buffer[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff) % 1000000
	return fmt.Sprintf("%06d", value)
}

// totpMatch prüft den Code für die Zeitfenster davor, jetzt und danach und liefert das passende Fenster.
func totpMatch(secret []byte, code string, now time.Time) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return 0, false
	}
	current := now.Unix() / totpStep
	for _, counter := range []int64{current, current - 1, current + 1} {
		if subtle.ConstantTimeCompare([]byte(totpCode(secret, counter)), []byte(code)) == 1 {
			return counter, true
		}
	}
	return 0, false
}

var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

func newTotpSecret() ([]byte, string) {
	secret := make([]byte, 20)
	_, _ = rand.Read(secret)
	return secret, base32NoPad.EncodeToString(secret)
}

func totpURI(username, encoded string) string {
	label := url.PathEscape("Lodestone:" + username)
	return "otpauth://totp/" + label + "?secret=" + encoded + "&issuer=Lodestone&algorithm=SHA1&digits=6&period=30"
}

// newRecoveryCodes erzeugt Wiederherstellungscodes und liefert Klartext und Hashes.
func newRecoveryCodes(count int) (plain, hashes []string) {
	for i := 0; i < count; i++ {
		raw := make([]byte, 5)
		_, _ = rand.Read(raw)
		code := hex.EncodeToString(raw)
		code = code[:5] + "-" + code[5:]
		plain = append(plain, code)
		hashes = append(hashes, hashToken(code))
	}
	return plain, hashes
}

// consumeRecovery prüft einen Wiederherstellungscode und entfernt ihn, wenn er stimmt.
func consumeRecovery(hashes []string, code string) ([]string, bool) {
	want := hashToken(strings.ToLower(strings.TrimSpace(code)))
	for i, hash := range hashes {
		if subtle.ConstantTimeCompare([]byte(hash), []byte(want)) == 1 {
			return append(append([]string{}, hashes[:i]...), hashes[i+1:]...), true
		}
	}
	return hashes, false
}

// ---------------------------------------------------------------- Tickets für den zweiten Schritt der Anmeldung

const ticketLife = 5 * time.Minute

func (p *Panel) makeTicket(userID string) (string, error) {
	key, err := p.derive("ticket")
	if err != nil {
		return "", err
	}
	payload := userID + "|" + strconv.FormatInt(time.Now().Add(ticketLife).Unix(), 10)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (p *Panel) readTicket(ticket string) (string, bool) {
	parts := strings.Split(ticket, ".")
	if len(parts) != 2 {
		return "", false
	}
	payload, err1 := base64.RawURLEncoding.DecodeString(parts[0])
	given, err2 := base64.RawURLEncoding.DecodeString(parts[1])
	key, err3 := p.derive("ticket")
	if err1 != nil || err2 != nil || err3 != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	if !hmac.Equal(mac.Sum(nil), given) {
		return "", false
	}
	fields := strings.SplitN(string(payload), "|", 2)
	if len(fields) != 2 {
		return "", false
	}
	expires, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return "", false
	}
	return fields[0], true
}

// ---------------------------------------------------------------- Schnittstelle: Zwei-Faktor

func (p *Panel) meDetails(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	_, enabled, recovery, _ := p.store.TotpState(user.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"id": user.ID, "username": user.Username, "isAdmin": user.IsAdmin, "disabled": user.Disabled, "createdAt": user.CreatedAt,
		"totpEnabled": enabled, "recoveryCodes": len(recovery),
	})
}

func (p *Panel) totpSetup(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	if _, enabled, _, _ := p.store.TotpState(user.ID); enabled {
		writeError(w, fail(http.StatusConflict, "die Zwei-Faktor-Anmeldung ist schon eingeschaltet"))
		return
	}
	secret, encoded := newTotpSecret()
	sealed, err := p.seal(base32NoPad.EncodeToString(secret))
	if err != nil {
		writeError(w, err)
		return
	}
	if err := p.store.SetTotp(user.ID, sealed, false, nil); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": encoded, "uri": totpURI(user.Username, encoded)})
}

func (p *Panel) pendingSecret(userID string) ([]byte, error) {
	sealed, _, _, err := p.store.TotpState(userID)
	if err != nil || sealed == "" {
		return nil, errors.New("keine Einrichtung offen")
	}
	text, err := p.unseal(sealed)
	if err != nil {
		return nil, err
	}
	return base32NoPad.DecodeString(text)
}

func (p *Panel) totpEnable(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	var body struct {
		Code string `json:"code"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if _, enabled, _, _ := p.store.TotpState(user.ID); enabled {
		writeError(w, fail(http.StatusConflict, "die Zwei-Faktor-Anmeldung ist schon eingeschaltet"))
		return
	}
	secret, err := p.pendingSecret(user.ID)
	if err != nil {
		writeError(w, fail(http.StatusConflict, "starte die Einrichtung zuerst"))
		return
	}
	if p.throttle.blocked("totp|" + user.ID) {
		writeError(w, fail(http.StatusTooManyRequests, "zu viele Fehlversuche, bitte warte ein paar Minuten"))
		return
	}
	if _, ok := totpMatch(secret, body.Code, time.Now()); !ok {
		p.throttle.fail("totp|" + user.ID)
		writeError(w, fail(http.StatusBadRequest, "der Code stimmt nicht – prüfe die Uhrzeit deines Geräts"))
		return
	}
	plain, hashes := newRecoveryCodes(8)
	sealed, _, _, _ := p.store.TotpState(user.ID)
	if err := p.store.SetTotp(user.ID, sealed, true, hashes); err != nil {
		writeError(w, err)
		return
	}
	p.audit(r, "2FA eingeschaltet", "")
	writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": plain})
}

func (p *Panel) totpDisable(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	var body struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if !CheckPassword(body.Password, user.PasswordHash) {
		writeError(w, fail(http.StatusForbidden, "das Passwort stimmt nicht"))
		return
	}
	if ok, err := p.checkSecondFactor(user.ID, body.Code); err != nil || !ok {
		writeError(w, fail(http.StatusForbidden, "der Code stimmt nicht"))
		return
	}
	if err := p.store.SetTotp(user.ID, "", false, nil); err != nil {
		writeError(w, err)
		return
	}
	p.audit(r, "2FA ausgeschaltet", "")
	w.WriteHeader(http.StatusNoContent)
}

// checkSecondFactor prüft einen TOTP-Code (jeder Code gilt nur einmal) oder einen Wiederherstellungscode.
func (p *Panel) checkSecondFactor(userID, code string) (bool, error) {
	sealed, enabled, recovery, err := p.store.TotpState(userID)
	if err != nil || !enabled {
		return false, err
	}
	text, err := p.unseal(sealed)
	if err != nil {
		return false, err
	}
	secret, err := base32NoPad.DecodeString(text)
	if err != nil {
		return false, err
	}
	if counter, ok := totpMatch(secret, code, time.Now()); ok {
		p.keyMu.Lock()
		defer p.keyMu.Unlock()
		if p.totpUsed[userID] >= counter {
			return false, nil // derselbe Code zweimal
		}
		p.totpUsed[userID] = counter
		return true, nil
	}
	if left, ok := consumeRecovery(recovery, code); ok {
		return true, p.store.SetTotp(userID, sealed, true, left)
	}
	return false, nil
}

// totpLogin ist der zweite Schritt der Anmeldung.
func (p *Panel) totpLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ticket string `json:"ticket"`
		Code   string `json:"code"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	userID, ok := p.readTicket(body.Ticket)
	if !ok {
		writeError(w, fail(http.StatusUnauthorized, "die Anmeldung ist abgelaufen – melde dich noch einmal an"))
		return
	}
	key := "totp|" + userID
	if p.throttle.blocked(key) {
		writeError(w, fail(http.StatusTooManyRequests, "zu viele Fehlversuche, bitte warte ein paar Minuten"))
		return
	}
	valid, err := p.checkSecondFactor(userID, body.Code)
	if err != nil || !valid {
		p.throttle.fail(key)
		p.recordAudit(userID, "", "Anmeldung: falscher Code", "", http.StatusUnauthorized, p.clientIP(r))
		writeError(w, fail(http.StatusUnauthorized, "der Code stimmt nicht"))
		return
	}
	user, err := p.store.UserByID(userID)
	if err != nil || user.Disabled {
		writeError(w, fail(http.StatusUnauthorized, "nicht angemeldet"))
		return
	}
	p.throttle.reset(key)
	if err := p.startSession(w, r, user); err != nil {
		writeError(w, err)
		return
	}
	p.recordAudit(user.ID, user.Username, "Anmeldung", "", http.StatusOK, p.clientIP(r))
	writeJSON(w, http.StatusOK, user)
}

// resetTotp setzt die Zwei-Faktor-Anmeldung eines Benutzers zurück (Administrator).
func (p *Panel) resetTotp(w http.ResponseWriter, r *http.Request) {
	if err := requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	if err := p.store.SetTotp(r.PathValue("id"), "", false, nil); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- Schnittstelle: API-Schlüssel und Sitzungen

const keyPrefix = "lds_"

func (p *Panel) createKey(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	var body struct {
		Name        string `json:"name"`
		ExpiresDays int    `json:"expiresDays"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 48 || body.ExpiresDays < 0 || body.ExpiresDays > 3650 {
		writeError(w, fail(http.StatusBadRequest, "der Name braucht 1 bis 48 Zeichen"))
		return
	}
	existing, _ := p.store.ListAPIKeys(user.ID)
	if len(existing) >= 20 {
		writeError(w, fail(http.StatusConflict, "höchstens 20 Schlüssel pro Benutzer"))
		return
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		writeError(w, err)
		return
	}
	plain := keyPrefix + hex.EncodeToString(raw)
	key := APIKey{UserID: user.ID, Name: body.Name, Prefix: plain[:len(keyPrefix)+6]}
	if body.ExpiresDays > 0 {
		key.ExpiresAt = time.Now().Add(time.Duration(body.ExpiresDays) * 24 * time.Hour).UnixMilli()
	}
	created, err := p.store.CreateAPIKey(key, hashToken(plain))
	if err != nil {
		writeError(w, err)
		return
	}
	p.audit(r, "API-Schlüssel angelegt", body.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"key": plain, "info": created})
}

func (p *Panel) listKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := p.store.ListAPIKeys(userFrom(r.Context()).ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (p *Panel) deleteKey(w http.ResponseWriter, r *http.Request) {
	if err := p.store.DeleteAPIKey(userFrom(r.Context()).ID, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (p *Panel) listSessions(w http.ResponseWriter, r *http.Request) {
	current := ""
	if cookie, err := r.Cookie(cookieName); err == nil {
		current = hashToken(cookie.Value)
	}
	sessions, err := p.store.ListSessions(userFrom(r.Context()).ID, current)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (p *Panel) deleteSession(w http.ResponseWriter, r *http.Request) {
	if err := p.store.DeleteSessionByID(userFrom(r.Context()).ID, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sessionOnly lässt Schlüssel (Bearer) nicht an Einstellungen, mit denen man sich Zugang verschaffen könnte.
func (p *Panel) sessionOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if viaKey(r.Context()) {
			writeError(w, fail(http.StatusForbidden, "das geht nur mit einer Anmeldung im Browser, nicht mit einem API-Schlüssel"))
			return
		}
		next(w, r)
	}
}

// ---------------------------------------------------------------- Audit-Log

// recorder merkt sich den Status der Antwort, ohne Streaming, Flush und Websocket zu stören.
type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(data)
}

func (r *recorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (p *Panel) recordAudit(userID, username, action, target string, status int, ip string) {
	if err := p.store.AddAudit(AuditEntry{UserID: userID, Username: username, Action: action, Target: target, Status: status, IP: ip}); err != nil {
		p.log.Warn("Protokoll nicht gespeichert", "error", err)
	}
}

// audit trägt eine Aktion des angemeldeten Benutzers ein.
func (p *Panel) audit(r *http.Request, action, target string) {
	user := userFrom(r.Context())
	p.recordAudit(user.ID, user.Username, action, target, 0, p.clientIP(r))
}

// auditPath kürzt den Pfad auf das, was ein Mensch im Protokoll lesen will.
func auditPath(path string) string {
	if len(path) > 200 {
		return path[:200]
	}
	return path
}

func (p *Panel) listAudit(w http.ResponseWriter, r *http.Request) {
	if err := requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	before, _ := strconv.ParseInt(query.Get("before"), 10, 64)
	entries, err := p.store.ListAudit(limit, before, strings.TrimSpace(query.Get("user")))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}
