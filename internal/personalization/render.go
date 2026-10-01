package personalization

import (
	"fmt"
	"math"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/feed"
)

// render turns an Event into user-facing text. Kept separate from the Event so
// the event model stays free of UI concerns.
func render(e event.Event) (title, body string, sev feed.Severity) {
	switch e.Type {
	case event.PriceChanged:
		c := e.Float("change_pct")
		dir := "up"
		if c < 0 {
			dir = "down"
		}
		sev = feed.Info
		if math.Abs(c) >= 2 {
			sev = feed.Notable
		}
		return fmt.Sprintf("%s %s %.2f%%", e.Entity, dir, math.Abs(c)), fmt.Sprintf("Last price %.2f", e.Float("ltp")), sev
	case event.VolumeSpike:
		return fmt.Sprintf("⚠ %s unusual volume", e.Entity), fmt.Sprintf("Volume is %.1fx normal", e.Float("relative_volume")), feed.Notable
	case event.UnusualActivity:
		return fmt.Sprintf("🔥 %s unusual activity", e.Entity),
			fmt.Sprintf("Price %+.2f%% with volume %.1fx normal", e.Float("change_pct"), e.Float("relative_volume")), feed.High
	case event.NewsPublished:
		return e.Entity + ": " + e.String("headline"), e.String("summary"), feed.Info
	}
	return e.Entity + " update", string(e.Type), feed.Info
}
