package duel

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestDuelEndAnnouncesRestoredEffects: a normal duel end puts back the
// effects saved at its start as they are put on when cast: the challenger
// is told it feels the saved buff again, ahead of the icon refresh.
func TestDuelEndAnnouncesRestoredEffects(t *testing.T) {
	t.Parallel()
	a := conditionArena(t)
	a.cast(t, 0, buffBefore)
	a.challenge(t, 0, 1)

	a.players[1].c.Send(encodeDuelSurrender())
	a.srv.AdvanceUntil(t, "duel ends", func() bool {
		_, back := a.effectsOf(t, 0)[buffBefore]
		return back && a.standing(t, 0).DuelID() == 0 && a.standing(t, 1).DuelID() == 0
	})
	frames := drainFrames(t, a.players[0].c)

	felt := -1
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != serverpackets.SystemMessageYouFeelS1Effect {
			continue
		}
		if felt >= 0 {
			t.Fatalf("duel end frames = %x, want one felt message", testsupport.FrameOpcodes(frames))
		}
		felt = i
		r.ReadInt32() // parameter count
		if kind, skill := r.ReadInt32(), r.ReadInt32(); kind != serverpackets.SystemMessageParamSkillName || skill != buffBefore {
			t.Fatalf("felt message parameter = type %d skill %d, want skill %d", kind, skill, buffBefore)
		}
	}
	if felt < 0 {
		t.Fatalf("duel end frames = %x, want the felt message for the restored buff", testsupport.FrameOpcodes(frames))
	}
	icons := -1
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeAbnormalStatusUpdate {
			icons = i
		}
	}
	if icons < felt {
		t.Fatalf("duel end frames = %x, want an icon refresh after the felt message", testsupport.FrameOpcodes(frames))
	}
}
