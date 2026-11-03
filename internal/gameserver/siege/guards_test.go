package siege

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// The plain recording notifier leaves the guard steps out of its lines.
func (n *recordingNotifier) SpawnGuards(*castle.Castle)   {}
func (n *recordingNotifier) DespawnGuards(*castle.Castle) {}

// guardNotifier records the gate and guard steps, each guard step with the
// castle's owner and whether the battlefield is active as it runs.
type guardNotifier struct{ gateNotifier }

func (n guardNotifier) SpawnGuards(c *castle.Castle) {
	n.add("guards spawn %d owner=%#x field=%t", c.ID, c.OwnerID(), n.field.Active())
}

func (n guardNotifier) DespawnGuards(c *castle.Castle) {
	n.add("guards despawn %d field=%t", c.ID, n.field.Active())
}

// TestSiegeGuardSteps pins where the guard steps run in Siege.startSiege,
// endSiege and midVictory: at the start the guards spawn right after the
// gates close, the battlefield on, before SIEGE_STARTED (711); at
// mid-victory they leave and spawn again first, before the alliance
// notice, and also when the castle has no owner left; at the end they
// leave after the registrations are dropped (292) and before the gates
// stand again.
func TestSiegeGuardSteps(t *testing.T) {
	w := newWorldOf(t, DefaultConfig(), gatedCastles(t))
	gludio := w.siege(1)
	for _, id := range []int32{band1, rivals} {
		if _, refused := gludio.RegisterAttacker(w.clan(id)); refused {
			t.Fatalf("%#x refused", id)
		}
	}
	w.only(1)
	notes := guardNotifier{gateNotifier{recordingNotifier: w.notes, field: w.field}}
	w.clock = sim.NewInline(saturday.Add(-time.Second))
	w.e.Start(w.clock.NewQueue("sieges"), notes)
	w.notes.take()

	gludio.Start()
	expectLines(t, "start", w.notes.take(), []string{
		"state [BandOne,Rivals]=1",
		"state [Lords]=2",
		"close gludio_gate,gludio_wall field=true",
		"guards spawn 1 owner=0x10000001 field=true",
		"all 711 castle=1",
		"sound systemmsg_e.17",
		"tell [BandOne,Rivals] 1189",
	})

	if _, ok := w.castles.SetOwner(gludio.Castle(), w.clan(band1)); !ok {
		t.Fatal("BandOne did not take Gludio")
	}
	gludio.MidVictory()
	expectLines(t, "mid-victory", w.notes.take(), []string{
		"guards despawn 1 field=true",
		"guards spawn 1 owner=0x10000005 field=true",
		"tell [BandOne,Rivals] 1190",
		"respawn gludio_gate,gludio_wall weak=true field=true",
		"state [Lords,Rivals]=1",
		"state [BandOne]=2",
	})

	if _, ok := w.castles.RemoveOwner(gludio.Castle()); !ok {
		t.Fatal("Gludio lost no owner")
	}
	gludio.MidVictory()
	expectLines(t, "ownerless mid-victory", w.notes.take(), []string{
		"guards despawn 1 field=true",
		"guards spawn 1 owner=0x0 field=true",
	})

	gludio.End()
	got := w.notes.take()
	want := []string{
		"all 292 castle=1",
		"guards despawn 1 field=true",
		"respawn gludio_gate,gludio_wall weak=false field=true",
	}
	if len(got) < len(want) {
		t.Fatalf("end: %q", got)
	}
	expectLines(t, "end", got[len(got)-len(want):], want)
}
