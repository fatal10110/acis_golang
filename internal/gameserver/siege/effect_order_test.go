package siege

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// holdingNotifier is a guardNotifier whose next guard despawn, once armed,
// signals held and waits for release before it runs.
type holdingNotifier struct {
	guardNotifier
	armed   *atomic.Bool
	held    chan struct{}
	release chan struct{}
}

func (n holdingNotifier) DespawnGuards(c *castle.Castle) {
	if n.armed.CompareAndSwap(true, false) {
		close(n.held)
		<-n.release
	}
	n.guardNotifier.DespawnGuards(c)
}

// TestSiegeEffectsKeepDecisionOrder holds a mid-victory's world steps after
// its lock was released, runs the siege's end to completion meanwhile, then
// releases them: the end's steps still run after the mid-victory's, so the
// guards are gone and the gates stand at full HP once both are done.
func TestSiegeEffectsKeepDecisionOrder(t *testing.T) {
	w := newWorldOf(t, DefaultConfig(), gatedCastles(t))
	gludio := w.siege(1)
	for _, id := range []int32{band1, rivals} {
		if _, refused := gludio.RegisterAttacker(w.clan(id)); refused {
			t.Fatalf("%#x refused", id)
		}
	}
	w.only(1)
	notes := holdingNotifier{
		guardNotifier: guardNotifier{gateNotifier{recordingNotifier: w.notes, field: w.field}},
		armed:         new(atomic.Bool),
		held:          make(chan struct{}),
		release:       make(chan struct{}),
	}
	w.clock = sim.NewInline(saturday.Add(-time.Second))
	w.e.Start(w.clock.NewQueue("sieges"), notes)
	gludio.Start()
	w.notes.take()

	if _, ok := w.castles.SetOwner(gludio.Castle(), w.clan(band1)); !ok {
		t.Fatal("BandOne did not take Gludio")
	}
	notes.armed.Store(true)
	midDone := make(chan struct{})
	go func() {
		defer close(midDone)
		gludio.MidVictory()
	}()
	<-notes.held

	endDone := make(chan struct{})
	go func() {
		defer close(endDone)
		gludio.End()
	}()
	<-endDone
	close(notes.release)
	<-midDone

	got := w.notes.take()
	want := []string{
		"guards despawn 1 field=true",
		"guards spawn 1 owner=0x10000005 field=true",
		"tell [BandOne,Rivals] 1190",
		"respawn gludio_gate,gludio_wall weak=true field=true",
		"state [Lords,Rivals]=1",
		"state [BandOne]=2",
		"all 712 castle=1",
	}
	if len(got) < len(want) {
		t.Fatalf("steps: %q", got)
	}
	expectLines(t, "mid-victory then end", got[:len(want)], want)
	expectLines(t, "end last", got[len(got)-2:], []string{
		"guards despawn 1 field=true",
		"respawn gludio_gate,gludio_wall weak=false field=true",
	})
	if w.field.Active() {
		t.Fatal("the battlefield is still active after the end")
	}
}
