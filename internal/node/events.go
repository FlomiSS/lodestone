package node

import "sync"

// State ist der Zustand eines Servers.
type State string

const (
	StateInstalling State = "installing"
	StateOffline    State = "offline"
	StateStarting   State = "starting"
	StateRunning    State = "running"
	StateStopping   State = "stopping"
	StateRestoring  State = "restoring"
)

// Line ist eine Zeile der Konsole. Seq zählt je Server fortlaufend, damit Empfänger Doppelte erkennen.
type Line struct {
	Seq  int64  `json:"seq"`
	Time int64  `json:"time"`
	Text string `json:"text"`
}

// Event ist eine Meldung an alle, die einen Server beobachten.
type Event struct {
	Type  string `json:"type"` // line, state, stats
	Line  *Line  `json:"line,omitempty"`
	State State  `json:"state,omitempty"`
	Error string `json:"error,omitempty"`
	Stats *Stats `json:"stats,omitempty"`
}

// hub verteilt Meldungen an Beobachter. Wer nicht mitkommt, wird abgehängt (der Empfänger verbindet sich neu und
// holt den Verlauf), damit ein langsamer Browser nie den Server aufhält.
type hub struct {
	mu   sync.Mutex
	next int
	subs map[int]chan Event
}

func (h *hub) subscribe() (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		h.subs = map[int]chan Event{}
	}
	id := h.next
	h.next++
	channel := make(chan Event, 1024)
	h.subs[id] = channel
	return channel, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if existing, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(existing)
		}
	}
}

func (h *hub) publish(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, channel := range h.subs {
		select {
		case channel <- event:
		default:
			delete(h.subs, id)
			close(channel)
		}
	}
}

// ring merkt sich die letzten Konsolenzeilen.
type ring struct {
	mu    sync.Mutex
	lines []Line
	limit int
	seq   int64
}

func (r *ring) add(time int64, text string) Line {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	line := Line{Seq: r.seq, Time: time, Text: text}
	r.lines = append(r.lines, line)
	if r.limit > 0 && len(r.lines) > r.limit {
		r.lines = append([]Line(nil), r.lines[len(r.lines)-r.limit:]...)
	}
	return line
}

func (r *ring) tail(n int) []Line {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n <= 0 || n > len(r.lines) {
		n = len(r.lines)
	}
	return append([]Line(nil), r.lines[len(r.lines)-n:]...)
}
