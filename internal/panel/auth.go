package panel

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	cookieName    = "lodestone_session"
	sessionLength = 14 * 24 * time.Hour
	csrfHeader    = "X-Lodestone"
	minPassword   = 8
)

// ---------------------------------------------------------------- Passwörter (argon2id)

const (
	argonTime    = 2
	argonMemory  = 32 * 1024
	argonThreads = 2
	argonKeyLen  = 32
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func CheckPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash lässt Anmeldungen mit unbekanntem Benutzernamen gleich lange dauern.
var dummyHash, _ = HashPassword("lodestone-dummy-password")

func validatePassword(password string) error {
	if utf8.RuneCountInString(password) < minPassword {
		return fmt.Errorf("das Passwort braucht mindestens %d Zeichen", minPassword)
	}
	if len(password) > 256 {
		return errors.New("das Passwort ist zu lang")
	}
	return nil
}

func validateUsername(name string) error {
	if len(name) < 3 || len(name) > 32 {
		return errors.New("der Benutzername braucht 3 bis 32 Zeichen")
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
			return errors.New("im Benutzernamen sind nur Buchstaben, Ziffern, _ - . erlaubt")
		}
	}
	return nil
}

// ---------------------------------------------------------------- Sitzungen

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

type ctxKey int

const (
	userKey ctxKey = 1
	keyAuth ctxKey = 2
)

// viaKey sagt, ob die Anfrage mit einem API-Schlüssel statt mit einer Browser-Sitzung kam.
func viaKey(ctx context.Context) bool {
	value, _ := ctx.Value(keyAuth).(bool)
	return value
}

func userFrom(ctx context.Context) User {
	user, _ := ctx.Value(userKey).(User)
	return user
}

// startSession legt eine Sitzung an und setzt das Cookie.
func (p *Panel) startSession(w http.ResponseWriter, r *http.Request, user User) error {
	token, err := randomToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(sessionLength)
	agent := r.UserAgent()
	if len(agent) > 200 {
		agent = agent[:200]
	}
	if err := p.store.CreateSession(hashToken(token), user.ID, agent, expires); err != nil {
		return err
	}
	p.store.SetSessionIP(hashToken(token), p.clientIP(r))
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", Expires: expires, MaxAge: int(sessionLength.Seconds()),
		HttpOnly: true, Secure: p.cfg.SecureCookies, SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func (p *Panel) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: p.cfg.SecureCookies, SameSite: http.SameSiteStrictMode})
}

// authenticate prüft die Sitzung. Schreibende Anfragen brauchen zusätzlich den Header X-Lodestone (Schutz vor CSRF
// zusätzlich zu SameSite=Strict). Der Websocket prüft stattdessen die Herkunft.
func (p *Panel) authenticate(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var user User
		usedKey := false
		if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer "+keyPrefix) {
			// Persönlicher API-Schlüssel: kein Cookie, deshalb auch kein CSRF-Header nötig
			found, err := p.store.APIKeyUser(hashToken(strings.TrimPrefix(header, "Bearer ")))
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "ungültiger oder abgelaufener API-Schlüssel"})
				return
			}
			user, usedKey = found, true
		} else {
			cookie, err := r.Cookie(cookieName)
			if err != nil || cookie.Value == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "nicht angemeldet"})
				return
			}
			user, err = p.store.SessionUser(hashToken(cookie.Value))
			if err != nil {
				p.clearCookie(w)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "nicht angemeldet"})
				return
			}
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				if r.Header.Get(csrfHeader) == "" {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "fehlender Schutz-Header"})
					return
				}
			}
		}
		ctx := context.WithValue(context.WithValue(r.Context(), userKey, user), keyAuth, usedKey)
		request := r.WithContext(ctx)
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next(w, request)
			return
		}
		// Alles, was etwas ändert, kommt ins Protokoll (Pfad, nie der Inhalt der Anfrage)
		rec := &recorder{ResponseWriter: w}
		next(rec, request)
		action := r.Pattern
		if action == "" {
			action = r.Method + " " + r.URL.Path
		}
		if usedKey {
			action += " (API-Schlüssel)"
		}
		p.recordAudit(user.ID, user.Username, action, auditPath(r.URL.Path), rec.status, p.clientIP(r))
	}
}

// sameOrigin prüft, dass ein Websocket von der Seite des Panels selbst kommt.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // kein Browser
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

// ---------------------------------------------------------------- Drosselung der Anmeldung

type throttle struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

const (
	throttleWindow = 10 * time.Minute
	throttleLimit  = 8
)

func newThrottle() *throttle { return &throttle{failures: map[string][]time.Time{}} }

func (t *throttle) recent(key string) []time.Time {
	cutoff := time.Now().Add(-throttleWindow)
	list := t.failures[key][:0]
	for _, at := range t.failures[key] {
		if at.After(cutoff) {
			list = append(list, at)
		}
	}
	if len(list) == 0 {
		delete(t.failures, key)
	} else {
		t.failures[key] = list
	}
	return list
}

func (t *throttle) blocked(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.recent(key)) >= throttleLimit
}

func (t *throttle) fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failures[key] = append(t.recent(key), time.Now())
}

func (t *throttle) reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, key)
}

func (p *Panel) clientIP(r *http.Request) string {
	if p.cfg.TrustProxy {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			return strings.TrimSpace(strings.Split(forwarded, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
