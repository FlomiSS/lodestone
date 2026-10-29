package panel

import (
	"net/http"
	"strings"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (p *Panel) authStatus(w http.ResponseWriter, r *http.Request) {
	count, err := p.store.CountUsers()
	if err != nil {
		writeError(w, err)
		return
	}
	result := map[string]any{"setupRequired": count == 0, "version": p.cfg.Version}
	if cookie, err := r.Cookie(cookieName); err == nil {
		if user, err := p.store.SessionUser(hashToken(cookie.Value)); err == nil {
			result["user"] = user
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// setup legt den ersten Administrator an. Das geht nur, solange es noch keinen Benutzer gibt.
func (p *Panel) setup(w http.ResponseWriter, r *http.Request) {
	var body credentials
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	if err := validateUsername(body.Username); err != nil {
		writeError(w, fail(http.StatusBadRequest, err.Error()))
		return
	}
	if err := validatePassword(body.Password); err != nil {
		writeError(w, fail(http.StatusBadRequest, err.Error()))
		return
	}
	count, err := p.store.CountUsers()
	if err != nil {
		writeError(w, err)
		return
	}
	if count > 0 {
		writeError(w, fail(http.StatusConflict, "die Einrichtung ist schon abgeschlossen"))
		return
	}
	hash, err := HashPassword(body.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	user, err := p.store.CreateUser(body.Username, hash, true)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := p.startSession(w, r, user); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (p *Panel) login(w http.ResponseWriter, r *http.Request) {
	var body credentials
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	key := p.clientIP(r) + "|" + strings.ToLower(body.Username)
	ipKey := p.clientIP(r)
	if p.throttle.blocked(key) || p.throttle.blocked(ipKey+"|*") {
		writeError(w, fail(http.StatusTooManyRequests, "zu viele Fehlversuche, bitte warte ein paar Minuten"))
		return
	}
	user, err := p.store.UserByName(strings.TrimSpace(body.Username))
	hash := dummyHash
	if err == nil {
		hash = user.PasswordHash
	}
	valid := CheckPassword(body.Password, hash) && err == nil && !user.Disabled
	if !valid {
		p.throttle.fail(key)
		p.throttle.fail(ipKey + "|*")
		writeError(w, fail(http.StatusUnauthorized, "Benutzername oder Passwort stimmt nicht"))
		return
	}
	p.throttle.reset(key)
	if err := p.startSession(w, r, user); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (p *Panel) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(cookieName); err == nil {
		_ = p.store.DeleteSession(hashToken(cookie.Value))
	}
	p.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (p *Panel) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, userFrom(r.Context()))
}

func (p *Panel) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Current string `json:"current"`
		Next    string `json:"next"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	user := userFrom(r.Context())
	if !CheckPassword(body.Current, user.PasswordHash) {
		writeError(w, fail(http.StatusForbidden, "das aktuelle Passwort stimmt nicht"))
		return
	}
	if err := validatePassword(body.Next); err != nil {
		writeError(w, fail(http.StatusBadRequest, err.Error()))
		return
	}
	hash, err := HashPassword(body.Next)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := p.store.SetPassword(user.ID, hash); err != nil {
		writeError(w, err)
		return
	}
	// Alle anderen Sitzungen enden, die eigene bleibt
	_ = p.store.DeleteUserSessions(user.ID)
	if err := p.startSession(w, r, user); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- Benutzerverwaltung (Administratoren)

// listUsers liefert alle Benutzer; Nicht-Administratoren bekommen nur Namen (zum Einladen in Projekte ist das nötig).
func (p *Panel) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := p.store.ListUsers()
	if err != nil {
		writeError(w, err)
		return
	}
	if !userFrom(r.Context()).IsAdmin {
		type brief struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		}
		list := make([]brief, 0, len(users))
		for _, user := range users {
			if !user.Disabled {
				list = append(list, brief{user.ID, user.Username})
			}
		}
		writeJSON(w, http.StatusOK, list)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (p *Panel) createUser(w http.ResponseWriter, r *http.Request) {
	if err := requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		credentials
		IsAdmin bool `json:"isAdmin"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	if err := validateUsername(body.Username); err != nil {
		writeError(w, fail(http.StatusBadRequest, err.Error()))
		return
	}
	if err := validatePassword(body.Password); err != nil {
		writeError(w, fail(http.StatusBadRequest, err.Error()))
		return
	}
	hash, err := HashPassword(body.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	user, err := p.store.CreateUser(body.Username, hash, body.IsAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (p *Panel) updateUser(w http.ResponseWriter, r *http.Request) {
	if err := requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		IsAdmin  *bool `json:"isAdmin"`
		Disabled *bool `json:"disabled"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	user, err := p.store.UserByID(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	admin, disabled := user.IsAdmin, user.Disabled
	if body.IsAdmin != nil {
		admin = *body.IsAdmin
	}
	if body.Disabled != nil {
		disabled = *body.Disabled
	}
	self := userFrom(r.Context())
	if user.ID == self.ID && (!admin || disabled) {
		writeError(w, fail(http.StatusConflict, "du kannst dir die Administratorrechte nicht selbst nehmen oder dich sperren"))
		return
	}
	if err := p.store.UpdateUser(user.ID, admin, disabled); err != nil {
		writeError(w, err)
		return
	}
	if disabled {
		_ = p.store.DeleteUserSessions(user.ID)
	}
	user.IsAdmin, user.Disabled = admin, disabled
	writeJSON(w, http.StatusOK, user)
}

func (p *Panel) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	id := r.PathValue("id")
	if id == userFrom(r.Context()).ID {
		writeError(w, fail(http.StatusConflict, "du kannst dich nicht selbst löschen"))
		return
	}
	// Projekte, deren einziger Eigentümer er ist, würden verwaisen
	projects, err := p.store.ListProjects(id, false)
	if err != nil {
		writeError(w, err)
		return
	}
	for _, project := range projects {
		if role, _ := p.store.MemberRole(project.ID, id); role == "owner" {
			if owners, _ := p.store.CountOwners(project.ID); owners <= 1 {
				writeError(w, fail(http.StatusConflict, "er ist der einzige Eigentümer von „"+project.Name+"“ – übertrage das Projekt zuerst"))
				return
			}
		}
	}
	if err := p.store.DeleteUser(id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (p *Panel) resetPassword(w http.ResponseWriter, r *http.Request) {
	if err := requireAdmin(r); err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := validatePassword(body.Password); err != nil {
		writeError(w, fail(http.StatusBadRequest, err.Error()))
		return
	}
	user, err := p.store.UserByID(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	hash, err := HashPassword(body.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := p.store.SetPassword(user.ID, hash); err != nil {
		writeError(w, err)
		return
	}
	if user.ID != userFrom(r.Context()).ID {
		_ = p.store.DeleteUserSessions(user.ID)
	}
	w.WriteHeader(http.StatusNoContent)
}
