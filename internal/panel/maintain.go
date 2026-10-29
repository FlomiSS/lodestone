package panel

import (
	"context"
	"time"
)

// Maintain räumt regelmäßig abgelaufene Sitzungen weg, bis ctx endet.
func (p *Panel) Maintain(ctx context.Context) {
	go p.sampleLoop(ctx)
	go p.scaleLoop(ctx)
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if err := p.store.PurgeSessions(); err != nil {
			p.log.Warn("Sitzungen aufräumen fehlgeschlagen", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
