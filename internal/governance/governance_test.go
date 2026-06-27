package governance

import (
	"testing"

	"github.com/kkkksu/opentag/internal/config"
)

func newGov(dms bool, cap int) *Governor {
	return New(&config.Config{Governance: config.Governance{
		DMsEnabled: &dms,
		SpendCap:   config.SpendCap{TurnsPerChannel: cap},
	}})
}

func TestAllowDM(t *testing.T) {
	if d := newGov(false, 0).AllowDM(); d.Allowed {
		t.Errorf("DMs disabled should deny")
	}
	if d := newGov(true, 0).AllowDM(); !d.Allowed {
		t.Errorf("DMs enabled should allow")
	}
}

func TestChargeTurn_CapAndAlerts(t *testing.T) {
	g := newGov(true, 4) // thresholds: 75% -> 3, 95% -> 3 (4*95/100=3)
	var alerts []string
	for i := range 4 {
		d, alert := g.ChargeTurn("C1")
		if !d.Allowed {
			t.Fatalf("turn %d unexpectedly denied", i)
		}
		if alert != "" {
			alerts = append(alerts, alert)
		}
	}
	if len(alerts) == 0 {
		t.Errorf("expected at least one threshold alert")
	}
	// 5th turn exceeds the cap and must be declined.
	if d, _ := g.ChargeTurn("C1"); d.Allowed {
		t.Errorf("over-cap turn should be declined")
	}
	// A different channel has its own budget.
	if d, _ := g.ChargeTurn("C2"); !d.Allowed {
		t.Errorf("separate channel should have its own budget")
	}
}

func TestChargeTurn_Unlimited(t *testing.T) {
	g := newGov(true, 0)
	for range 100 {
		if d, _ := g.ChargeTurn("C1"); !d.Allowed {
			t.Fatalf("unlimited cap should never deny")
		}
	}
}
