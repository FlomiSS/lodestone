package node

import "strings"

// AutoconfigResult sagt, was beim Anwenden herauskam.
type AutoconfigResult struct {
	Version int  `json:"version"`
	Pending bool `json:"pending"`
	Changed bool `json:"changed"`
	// Missing sind Dateien, auf die der Satz noch wartet (sie entstehen beim ersten Start).
	Missing []string `json:"missing,omitempty"`
}

// SetAutoconfig legt den Satz fest und wendet ihn an, soweit die Dateien schon da sind. Der Rest folgt, sobald der
// Server das nächste Mal bereit ist.
func (s *Server) SetAutoconfig(a Autoconfig) (AutoconfigResult, error) {
	if err := a.Validate(); err != nil {
		return AutoconfigResult{}, err
	}
	s.mu.Lock()
	a.Version = 1
	if s.spec.Autoconfig != nil {
		a.Version = s.spec.Autoconfig.Version + 1
	}
	s.spec.Autoconfig = &a
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return AutoconfigResult{}, err
	}
	return s.tryApply(false), nil
}

// tryApply wendet einen offenen Satz an. atReady heißt: Der Server ist gerade hochgefahren, dann darf ein Neustart
// folgen, falls der Satz ihn verlangt.
func (s *Server) tryApply(atReady bool) AutoconfigResult {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	spec := s.snapshot()
	if spec.Autoconfig == nil {
		return AutoconfigResult{}
	}
	a := *spec.Autoconfig
	result := AutoconfigResult{Version: a.Version}
	if spec.AutoconfigApplied >= a.Version {
		return result
	}
	changed, missing, err := ApplyAutoconfig(s.fs, a)
	if err != nil {
		s.logf("Autoconfig fehlgeschlagen: %v", err)
		result.Pending = true
		return result
	}
	result.Changed, result.Missing = changed, missing
	if len(missing) > 0 {
		s.logf("Autoconfig wartet auf Dateien, die der Server beim Start anlegt: %s", strings.Join(missing, ", "))
		result.Pending = true
		return result
	}
	s.mu.Lock()
	s.spec.AutoconfigApplied = a.Version
	_ = s.saveLocked()
	state := s.state
	s.mu.Unlock()
	if !changed {
		s.logf("Autoconfig geprüft, es war nichts zu ändern")
		return result
	}
	s.logf("Autoconfig angewendet")
	switch {
	case atReady && a.Restart:
		s.logf("Der Server wird einmal neu gestartet, damit die Einstellungen gelten")
		go func() { _ = s.Restart() }()
	case state == StateRunning && a.Reload != "":
		_ = s.Command(a.Reload)
	}
	return result
}

func (s *Server) autoconfigPending() bool {
	return s.spec.Autoconfig != nil && s.spec.AutoconfigApplied < s.spec.Autoconfig.Version
}
