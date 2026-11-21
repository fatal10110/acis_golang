package ai

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/frintezza"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// fzTeleporterID is the teleporter at the tomb's entrance.
const fzTeleporterID = int32(32011)

// fzTeleporterTomb adds the teleporter beside the character, its maker
// placed before Frintezza's when first is set, with the pages it answers,
// the first linking its entry.
func fzTeleporterTomb(first bool) fzTomb {
	pages := map[string]string{}
	for _, name := range []string{"001", "002", "003", "007"} {
		page := "frintessa_teleporter" + name + ".htm"
		pages["script/ai/boss/frintezza/FrintezzaTeleporter/"+page] = "<html><body>" + page + "</body></html>"
	}
	pages["script/ai/boss/frintezza/FrintezzaTeleporter/frintessa_teleporter001.htm"] = `<html><body>frintessa_teleporter001.htm<a action="bypass -h Quest FrintezzaTeleporter enter">Enter</a></body></html>`
	return fzTomb{
		makers: fmt.Sprintf(`	<npcmaker name="fz_teleporter" territory="fr_tomb" maximumNpcs="1"><npc id="%d" pos="40;20;30;0" total="1" respawn="1min"/></npcmaker>
`, fzTeleporterID),
		first:   first,
		scripts: script.Catalog{"script.ai.boss.frintezza.FrintezzaTeleporter": frintezza.FrintezzaTeleporter},
		folk:    []int32{fzTeleporterID},
		opts:    []gameservertest.Option{gameservertest.WithHTMLPages(pages)},
	}
}

// teleporter returns the one teleporter in the world.
func (w *frWorld) teleporter(t *testing.T) *npc.Folk {
	t.Helper()
	var out []*npc.Folk
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && int32(f.Instance.Template.ID) == fzTeleporterID && !f.Decayed() {
			out = append(out, f)
		}
	}
	if len(out) != 1 {
		t.Fatalf("teleporters in the world = %d, want 1", len(out))
	}
	return out[0]
}

// askTeleporter talks to the teleporter until it answers, then follows its
// link to event, and returns the pages of the answer.
func (w *frWorld) askTeleporter(t *testing.T, event string) []string {
	t.Helper()
	c := w.srv.Client
	f := w.teleporter(t)
	var frames [][]byte
	for range 2 {
		c.Send(encodeFolkAction(f.ObjectID(), w.at))
		w.srv.Settle(t)
		frames = append(frames, w.srv.ReadQueued(t, c)...)
		if len(baiumPageNames(frames)) > 0 {
			break
		}
	}
	if got := baiumPageNames(frames); !slices.Equal(got, []string{"frintessa_teleporter001.htm"}) {
		t.Fatalf("first talk: pages = %q, want frintessa_teleporter001.htm", got)
	}
	c.Send(encodeManagerBypass("Quest FrintezzaTeleporter " + event))
	w.srv.Settle(t)
	return baiumPageNames(w.srv.ReadQueued(t, c))
}

// TestFrintezzaStartsOverFromTheEntryCountdown: a restart inside the entry
// countdown boots Frintezza from a row saved at stage 1, whose countdown
// timers lived in memory only. He starts the tomb over idle, as from no
// row: db value 0, maker event 1001 on frintessa_2515_m01, the tomb sealed
// and his follower and its transform told 0. The teleporter, appearing
// before or after him, keeps no entry: asked to enter by a character
// leading no command channel it answers 003, not the busy page 007.
// Shutdown saves 0, so a second restart is clean. An owner-approved
// divergence (#4683).
func TestFrintezzaStartsOverFromTheEntryCountdown(t *testing.T) {
	t.Parallel()
	for _, first := range []bool{false, true} {
		t.Run(fmt.Sprintf("teleporter first=%v", first), func(t *testing.T) {
			t.Parallel()
			w := bootFrintezzaTomb(t, fzLiveRow(1), &ind1Draws{}, fzTeleporterTomb(first))
			f := w.frintezza(t)
			n := script.NPCOf(f)
			id := f.ObjectID()
			if !n.DBLoaded() || n.DBValue() != 0 {
				t.Fatalf("Frintezza loaded=%v value=%d, want loaded at 0", n.DBLoaded(), n.DBValue())
			}
			if got, want := w.makers.take(), []string{"frintessa_2515_m01 1001"}; !slices.Equal(got, want) {
				t.Fatalf("maker events = %q, want %q", got, want)
			}
			want := []string{fmt.Sprintf("29996 event=%d arg1=0 arg2=0", id), fmt.Sprintf("29997 event=%d arg1=0 arg2=0", id)}
			if got := w.heard.take(); !slices.Equal(got, want) {
				t.Fatalf("heard as Frintezza appeared = %q, want %q", got, want)
			}
			w.wantDoors(t, "restored at 1", fzSealed)
			if got := script.NPCOf(w.teleporter(t)).Int(script.SlotIntAI0); got != 0 {
				t.Fatalf("the teleporter's entry mark = %d, want 0", got)
			}
			if got := w.askTeleporter(t, "enter"); !slices.Equal(got, []string{"frintessa_teleporter003.htm"}) {
				t.Fatalf("enter: pages = %q, want frintessa_teleporter003.htm", got)
			}
			w.srv.Shutdown(t)
			if got := fzRowValue(t, w.srv); got != 0 {
				t.Fatalf("saved db value = %d, want 0", got)
			}
		})
	}
}
