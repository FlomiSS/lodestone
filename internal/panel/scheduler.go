package panel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lodestone-mc/lodestone/internal/node"
)

const (
	scheduleEvery   = 15 * time.Second
	maxSteps        = 10
	maxSchedules    = 50
	lateTolerance   = time.Hour // so spät darf ein täglicher Lauf noch nachgeholt werden (z. B. nach einem Neustart des Panels)
	backupWaitLimit = 45 * time.Minute
)

var stepActions = map[string]bool{"backup": true, "command": true, "restart": true, "start": true, "stop": true, "wait": true}

// location liefert die Zeitzone, in der Zeitpläne gelten.
func (p *Panel) location() *time.Location {
	if p.cfg.Location != nil {
		return p.cfg.Location
	}
	return time.Local
}

// ---------------------------------------------------------------- Berechnung

func weekdayBit(day time.Weekday) int { return 1 << ((int(day) + 6) % 7) } // Montag = Bit 0

// nextRun liefert den nächsten Zeitpunkt nach "after".
func nextRun(s Schedule, after time.Time, loc *time.Location) time.Time {
	after = after.In(loc)
	switch s.Kind {
	case "interval":
		return after.Add(time.Duration(s.EveryMinutes) * time.Minute)
	case "daily", "weekly":
		for offset := 0; offset <= 8; offset++ {
			day := after.AddDate(0, 0, offset)
			if s.Kind == "weekly" && s.Weekdays&weekdayBit(day.Weekday()) == 0 {
				continue
			}
			candidate := time.Date(day.Year(), day.Month(), day.Day(), s.AtMinute/60, s.AtMinute%60, 0, 0, loc)
			if candidate.After(after) {
				return candidate
			}
		}
	}
	return time.Time{}
}

// dueAt ist der Zeitpunkt, zu dem ein Zeitplan als Nächstes laufen soll.
func (p *Panel) dueAt(s Schedule) time.Time {
	base := time.UnixMilli(s.CreatedAt)
	if s.LastRun > 0 {
		base = time.UnixMilli(s.LastRun)
	}
	return nextRun(s, base, p.location())
}

// ---------------------------------------------------------------- Planung

type warning struct {
	schedule Schedule
	minutes  int
}

type plan struct {
	warn []warning
	run  []Schedule
	skip []Schedule
}

func hasStopStep(steps []Step) bool {
	for _, step := range steps {
		if step.Action == "restart" || step.Action == "stop" {
			return true
		}
	}
	return false
}

// plan entscheidet, was zum Zeitpunkt now zu tun ist: Vorwarnungen senden, Läufe starten, verpasste überspringen.
func (p *Panel) plan(now time.Time, list []Schedule) plan {
	var result plan
	for _, s := range list {
		if !s.Enabled {
			continue
		}
		due := p.dueAt(s)
		if due.IsZero() {
			continue
		}
		if now.Before(due) {
			if s.WarnMinutes > 0 && hasStopStep(s.Steps) {
				for _, minutes := range warnPoints(s.WarnMinutes) {
					key := fmt.Sprintf("%s|%d|%d", s.ID, due.UnixMilli(), minutes)
					if !now.Before(due.Add(-time.Duration(minutes)*time.Minute)) && !p.warned(key) {
						result.warn = append(result.warn, warning{schedule: s, minutes: minutes})
					}
				}
			}
			continue
		}
		if s.Kind != "interval" && now.Sub(due) > lateTolerance {
			result.skip = append(result.skip, s)
			continue
		}
		result.run = append(result.run, s)
	}
	return result
}

// warnPoints sind die Zeitpunkte (Minuten vor dem Lauf), zu denen gewarnt wird: der gewählte und die letzte Minute.
func warnPoints(minutes int) []int {
	if minutes <= 1 {
		return []int{1}
	}
	return []int{minutes, 1}
}

func (p *Panel) warned(key string) bool {
	p.scheduleMu.Lock()
	defer p.scheduleMu.Unlock()
	return p.warnedKeys[key]
}

func (p *Panel) markWarned(key string) {
	p.scheduleMu.Lock()
	defer p.scheduleMu.Unlock()
	if len(p.warnedKeys) > 500 {
		p.warnedKeys = map[string]bool{}
	}
	p.warnedKeys[key] = true
}

// ---------------------------------------------------------------- Ausführung

func (p *Panel) scheduleLoop(ctx context.Context) {
	ticker := time.NewTicker(scheduleEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.scheduleTick(ctx, time.Now())
		}
	}
}

func (p *Panel) scheduleTick(ctx context.Context, now time.Time) {
	list, err := p.store.ListSchedules("")
	if err != nil {
		return
	}
	todo := p.plan(now, list)
	for _, w := range todo.warn {
		due := p.dueAt(w.schedule)
		p.markWarned(fmt.Sprintf("%s|%d|%d", w.schedule.ID, due.UnixMilli(), w.minutes))
		go p.sendWarning(ctx, w.schedule, w.minutes)
	}
	for _, s := range todo.skip {
		_ = p.store.RecordScheduleRun(s.ID, now, "skipped", "Der Zeitpunkt wurde verpasst (das Panel lief nicht) und nicht nachgeholt.")
	}
	for _, s := range todo.run {
		if !p.beginRun(s.ID) {
			continue
		}
		go func() {
			defer p.endRun(s.ID)
			p.runSchedule(ctx, s)
		}()
	}
}

func (p *Panel) beginRun(id string) bool {
	p.scheduleMu.Lock()
	defer p.scheduleMu.Unlock()
	if p.runningSchedules[id] {
		return false
	}
	p.runningSchedules[id] = true
	return true
}

func (p *Panel) endRun(id string) {
	p.scheduleMu.Lock()
	defer p.scheduleMu.Unlock()
	delete(p.runningSchedules, id)
}

func (p *Panel) sendWarning(ctx context.Context, s Schedule, minutes int) {
	row, err := p.store.ServerByID(s.ServerID)
	if err != nil || row.Software == "velocity" {
		return
	}
	client, _, err := p.client(row.NodeID)
	if err != nil {
		return
	}
	text := fmt.Sprintf("Der Server wird in %d Minuten neu gestartet.", minutes)
	if minutes == 1 {
		text = "Der Server wird in 1 Minute neu gestartet."
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = client.Do(callCtx, http.MethodPost, "/api/servers/"+row.ID+"/command", map[string]string{"command": "say " + text}, nil)
}

// runSchedule führt die Schritte nacheinander aus und merkt sich das Ergebnis.
func (p *Panel) runSchedule(ctx context.Context, s Schedule) {
	start := time.Now()
	row, err := p.store.ServerByID(s.ServerID)
	if err != nil {
		_ = p.store.RecordScheduleRun(s.ID, start, "failed", "Der Server existiert nicht mehr.")
		return
	}
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	for index, step := range s.Steps {
		if err := p.execStep(runCtx, row, step); err != nil {
			message := fmt.Sprintf("Schritt %d (%s): %v", index+1, step.Action, err)
			_ = p.store.RecordScheduleRun(s.ID, start, "failed", message)
			level := "error"
			p.emit(s.ProjectID, row.ID, "schedule_failed", level, fmt.Sprintf("Der Zeitplan „%s“ ist fehlgeschlagen. %s", s.Name, message))
			if step.Action == "backup" {
				p.emit(s.ProjectID, row.ID, "backup_failed", level, fmt.Sprintf("Das Backup von %s ist fehlgeschlagen: %v", row.Name, err))
			}
			return
		}
	}
	_ = p.store.RecordScheduleRun(s.ID, start, "ok", fmt.Sprintf("%d Schritt(e) in %s", len(s.Steps), time.Since(start).Round(time.Second)))
}

var errAlreadyDone = errors.New("schon erledigt")

func (p *Panel) execStep(ctx context.Context, row ServerRow, step Step) error {
	client, _, err := p.client(row.NodeID)
	if err != nil {
		return err
	}
	base := "/api/servers/" + row.ID
	switch step.Action {
	case "command":
		return client.Do(ctx, http.MethodPost, base+"/command", map[string]string{"command": step.Value}, nil)
	case "start":
		err := client.Do(ctx, http.MethodPost, base+"/power", map[string]string{"action": "start"}, nil)
		if isConflict(err) {
			return nil // läuft schon
		}
		if err != nil {
			return err
		}
		return p.waitState(ctx, client, row.ID, node.StateRunning, 10*time.Minute)
	case "stop":
		err := client.Do(ctx, http.MethodPost, base+"/power", map[string]string{"action": "stop"}, nil)
		if isConflict(err) {
			return nil // ist schon aus
		}
		if err != nil {
			return err
		}
		return p.waitState(ctx, client, row.ID, node.StateOffline, 5*time.Minute)
	case "restart":
		if err := client.Do(ctx, http.MethodPost, base+"/power", map[string]string{"action": "restart"}, nil); err != nil {
			return err
		}
		time.Sleep(restartSettle)
		return p.waitState(ctx, client, row.ID, node.StateRunning, 10*time.Minute)
	case "wait":
		seconds, _ := strconv.Atoi(step.Value)
		select {
		case <-time.After(time.Duration(seconds) * waitUnit):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	case "backup":
		return p.runBackup(ctx, client, row, step.Value)
	}
	return fmt.Errorf("unbekannte Aktion %q", step.Action)
}

// Zeiteinheiten, die Tests verkürzen können.
var (
	waitUnit      = time.Second
	restartSettle = 3 * time.Second
	pollEvery     = 2 * time.Second
)

func isConflict(err error) bool {
	var nodeErr *NodeError
	return errors.As(err, &nodeErr) && nodeErr.Status == http.StatusConflict
}

func (p *Panel) waitState(ctx context.Context, client *NodeClient, id string, want node.State, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		var view node.View
		if err := client.Do(ctx, http.MethodGet, "/api/servers/"+id, nil, &view); err != nil {
			return err
		}
		if view.State == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("der Server erreicht den Zustand „%s“ nicht (jetzt: %s)", want, view.State)
		}
		select {
		case <-time.After(pollEvery):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (p *Panel) runBackup(ctx context.Context, client *NodeClient, row ServerRow, label string) error {
	if label == "" {
		label = "geplant"
	}
	var created node.Backup
	if err := client.Do(ctx, http.MethodPost, "/api/servers/"+row.ID+"/backups", map[string]string{"label": label}, &created); err != nil {
		return err
	}
	deadline := time.Now().Add(backupWaitLimit)
	for {
		var list []node.Backup
		if err := client.Do(ctx, http.MethodGet, "/api/servers/"+row.ID+"/backups", nil, &list); err != nil {
			return err
		}
		creating := false
		for _, backup := range list {
			if backup.Name == created.Name {
				if !backup.Creating {
					p.emit(row.ProjectID, row.ID, "backup", "info", fmt.Sprintf("Backup von %s erstellt (%s).", row.Name, byteSize(backup.Size)))
					return nil
				}
				creating = true
			}
		}
		if !creating {
			return errors.New("das Backup ist nicht entstanden (siehe Konsole des Servers)")
		}
		if time.Now().After(deadline) {
			return errors.New("das Backup dauert zu lange")
		}
		select {
		case <-time.After(pollEvery):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func byteSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%d KB", n>>10)
}

// ---------------------------------------------------------------- Schnittstelle

type scheduleDTO struct {
	Schedule
	ServerName string `json:"serverName"`
	NextRun    int64  `json:"nextRun"`
}

func (p *Panel) scheduleDTOs(list []Schedule) []scheduleDTO {
	names := map[string]string{}
	result := make([]scheduleDTO, 0, len(list))
	for _, s := range list {
		name, ok := names[s.ServerID]
		if !ok {
			if row, err := p.store.ServerByID(s.ServerID); err == nil {
				name = row.Name
			}
			names[s.ServerID] = name
		}
		dto := scheduleDTO{Schedule: s, ServerName: name}
		if s.Enabled {
			if due := p.dueAt(s); !due.IsZero() {
				dto.NextRun = due.UnixMilli()
			}
		}
		result = append(result, dto)
	}
	return result
}

type scheduleRequest struct {
	ServerID     string  `json:"serverId"`
	Name         *string `json:"name"`
	Steps        *[]Step `json:"steps"`
	Kind         *string `json:"kind"`
	EveryMinutes *int    `json:"everyMinutes"`
	AtMinute     *int    `json:"atMinute"`
	Weekdays     *int    `json:"weekdays"`
	WarnMinutes  *int    `json:"warnMinutes"`
	Enabled      *bool   `json:"enabled"`
}

func (r scheduleRequest) apply(s *Schedule) error {
	if r.Name != nil {
		s.Name = strings.TrimSpace(*r.Name)
	}
	if r.Steps != nil {
		s.Steps = *r.Steps
	}
	if r.Kind != nil {
		s.Kind = *r.Kind
	}
	if r.EveryMinutes != nil {
		s.EveryMinutes = *r.EveryMinutes
	}
	if r.AtMinute != nil {
		s.AtMinute = *r.AtMinute
	}
	if r.Weekdays != nil {
		s.Weekdays = *r.Weekdays
	}
	if r.WarnMinutes != nil {
		s.WarnMinutes = *r.WarnMinutes
	}
	if r.Enabled != nil {
		s.Enabled = *r.Enabled
	}
	return s.validate()
}

func (s *Schedule) validate() error {
	if s.Name == "" || len(s.Name) > 48 {
		return fail(http.StatusBadRequest, "der Name braucht 1 bis 48 Zeichen")
	}
	if len(s.Steps) == 0 || len(s.Steps) > maxSteps {
		return fail(http.StatusBadRequest, fmt.Sprintf("ein Zeitplan braucht 1 bis %d Schritte", maxSteps))
	}
	for i, step := range s.Steps {
		if !stepActions[step.Action] {
			return fail(http.StatusBadRequest, fmt.Sprintf("Schritt %d: unbekannte Aktion %q", i+1, step.Action))
		}
		switch step.Action {
		case "command":
			if strings.TrimSpace(step.Value) == "" || len(step.Value) > 200 || strings.ContainsAny(step.Value, "\r\n\x00") {
				return fail(http.StatusBadRequest, fmt.Sprintf("Schritt %d: ungültiger Befehl", i+1))
			}
		case "wait":
			if seconds, err := strconv.Atoi(step.Value); err != nil || seconds < 1 || seconds > 600 {
				return fail(http.StatusBadRequest, fmt.Sprintf("Schritt %d: Wartezeit 1 bis 600 Sekunden", i+1))
			}
		case "backup":
			if len(step.Value) > 40 {
				return fail(http.StatusBadRequest, fmt.Sprintf("Schritt %d: die Bezeichnung ist zu lang", i+1))
			}
		}
	}
	switch s.Kind {
	case "interval":
		if s.EveryMinutes < 5 || s.EveryMinutes > 7*24*60 {
			return fail(http.StatusBadRequest, "das Intervall muss zwischen 5 Minuten und 7 Tagen liegen")
		}
	case "daily", "weekly":
		if s.AtMinute < 0 || s.AtMinute > 1439 {
			return fail(http.StatusBadRequest, "ungültige Uhrzeit")
		}
		if s.Kind == "weekly" && (s.Weekdays < 1 || s.Weekdays > 127) {
			return fail(http.StatusBadRequest, "wähle mindestens einen Wochentag")
		}
	default:
		return fail(http.StatusBadRequest, "die Art muss interval, daily oder weekly sein")
	}
	if s.WarnMinutes < 0 || s.WarnMinutes > 60 {
		return fail(http.StatusBadRequest, "die Vorwarnung darf höchstens 60 Minuten betragen")
	}
	return nil
}

func (p *Panel) listSchedules(w http.ResponseWriter, r *http.Request) {
	project, _, err := p.requireProject(r, r.PathValue("id"), rankViewer)
	if err != nil {
		writeError(w, err)
		return
	}
	list, err := p.store.ListSchedules(project.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p.scheduleDTOs(list))
}

func (p *Panel) createSchedule(w http.ResponseWriter, r *http.Request) {
	project, _, err := p.requireProject(r, r.PathValue("id"), rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	var body scheduleRequest
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	row, err := p.store.ServerByID(body.ServerID)
	if err != nil || row.ProjectID != project.ID {
		writeError(w, fail(http.StatusBadRequest, "wähle einen Server dieses Projekts"))
		return
	}
	existing, _ := p.store.ListSchedules(project.ID)
	if len(existing) >= maxSchedules {
		writeError(w, fail(http.StatusConflict, fmt.Sprintf("höchstens %d Zeitpläne pro Projekt", maxSchedules)))
		return
	}
	schedule := Schedule{ProjectID: project.ID, ServerID: row.ID, Enabled: true, Weekdays: 127}
	if err := body.apply(&schedule); err != nil {
		writeError(w, err)
		return
	}
	created, err := p.store.CreateSchedule(schedule)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p.scheduleDTOs([]Schedule{created})[0])
}

func (p *Panel) requireSchedule(r *http.Request, minimum int) (Schedule, error) {
	schedule, err := p.store.ScheduleByID(r.PathValue("id"))
	if err != nil {
		return Schedule{}, errNotFound
	}
	if _, _, err := p.requireProject(r, schedule.ProjectID, minimum); err != nil {
		return Schedule{}, err
	}
	return schedule, nil
}

func (p *Panel) updateSchedule(w http.ResponseWriter, r *http.Request) {
	schedule, err := p.requireSchedule(r, rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	var body scheduleRequest
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	body.ServerID = ""
	if err := body.apply(&schedule); err != nil {
		writeError(w, err)
		return
	}
	if err := p.store.UpdateSchedule(schedule); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p.scheduleDTOs([]Schedule{schedule})[0])
}

func (p *Panel) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	schedule, err := p.requireSchedule(r, rankAdmin)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := p.store.DeleteSchedule(schedule.ID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// runScheduleNow führt einen Zeitplan sofort aus (zusätzlich zu seinen festen Zeiten).
func (p *Panel) runScheduleNow(w http.ResponseWriter, r *http.Request) {
	schedule, err := p.requireSchedule(r, rankOperator)
	if err != nil {
		writeError(w, err)
		return
	}
	if !p.beginRun(schedule.ID) {
		writeError(w, fail(http.StatusConflict, "der Zeitplan läuft gerade"))
		return
	}
	go func() {
		defer p.endRun(schedule.ID)
		p.runSchedule(context.Background(), schedule)
	}()
	w.WriteHeader(http.StatusAccepted)
}
