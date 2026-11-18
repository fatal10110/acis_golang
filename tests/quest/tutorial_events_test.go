package quest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// die kills the newbie with damage no vitals survive and returns the
// tutorial frames of the death.
func (r *levelRig) die() [][]byte {
	r.t.Helper()
	onCharacter(r.t, r.srv, r.objID, func(c *player.Character) {
		if !c.TakeDamage(1_000_000, c) {
			r.t.Error("the hit did not land")
		}
		if !c.Dead() {
			r.t.Error("the newbie survived")
		}
	})
	return tutorialFrames(r.srv.ReadQueued(r.t, r.srv.Client))
}

// revive brings the newbie back.
func (r *levelRig) revive() {
	r.t.Helper()
	onCharacter(r.t, r.srv, r.objID, func(c *player.Character) {
		if !c.Revive() {
			r.t.Error("the newbie was not revived")
		}
	})
	r.srv.ReadQueued(r.t, r.srv.Client)
}

// lowHPFrames are the frames the first lethal hit of a newbie starts with:
// the low-HP step, which runs on the hit that took the HP below a third.
func (r *levelRig) lowHPFrames() []wire.Frame {
	return []wire.Frame{
		serverpackets.FrameTutorialShowQuestionMark(10),
		serverpackets.FrameTutorialEnableClientEvent(8388608),
		serverpackets.FramePlaySound(script.SoundTutorial),
		voiceFrame(r.objID, r.at, "tutorial_voice_017"),
	}
}

// deathFrames are the death step's frames: question mark 8, the client
// event switched off, the tutorial sound and voice 016.
func (r *levelRig) deathFrames() []wire.Frame {
	return []wire.Frame{
		serverpackets.FrameTutorialShowQuestionMark(8),
		serverpackets.FrameTutorialEnableClientEvent(0),
		serverpackets.FramePlaySound(script.SoundTutorial),
		voiceFrame(r.objID, r.at, "tutorial_voice_016"),
	}
}

// TestTutorialDeathStep: a newbie below level 10 that dies is shown
// question mark 8 with voice 016 and the client event switched off, and the
// tutorial notes it in Die; dying again shows nothing.
func TestTutorialDeathStep(t *testing.T) {
	t.Parallel()
	r := bootLevelRig(t, 0)
	expectFrames(t, "first death", r.die(), append(r.lowHPFrames(), r.deathFrames()...)...)
	if got := r.srv.QuestVars(t, r.objID, tutorial)["Die"]; got != "1" {
		t.Fatalf("Die = %q, want 1", got)
	}
	r.revive()
	expectFrames(t, "second death", r.die())
}

// TestTutorialDeathStepLevelNine: level 9 is still a newbie.
func TestTutorialDeathStepLevelNine(t *testing.T) {
	t.Parallel()
	r := bootLevelRig(t, 0)
	r.gainTo(9)
	expectFrames(t, "death at 9", r.die(), r.deathFrames()...)
	if got := r.srv.QuestVars(t, r.objID, tutorial)["Die"]; got != "1" {
		t.Fatalf("Die = %q, want 1", got)
	}
}

// TestTutorialDeathStepLevelTen: from level 10 a death is not a newbie's:
// nothing is shown and Die stays unset.
func TestTutorialDeathStepLevelTen(t *testing.T) {
	t.Parallel()
	r := bootLevelRig(t, 0)
	r.gainTo(10)
	expectFrames(t, "death at 10", r.die())
	if got, ok := r.srv.QuestVars(t, r.objID, tutorial)["Die"]; ok {
		t.Fatalf("Die = %q, want unset", got)
	}
}

// pickUpAdena puts adena at the newbie's feet and picks it up, returning
// the tutorial frames of the pickup.
func (r *levelRig) pickUpAdena() [][]byte {
	r.t.Helper()
	var x, y, z int
	onCharacter(r.t, r.srv, r.objID, func(c *player.Character) { x, y, z = c.Position() })
	r.srv.SeedGroundItem(r.t, 0, item.AdenaID, 10, x, y, z)
	r.srv.ReadQueued(r.t, r.srv.Client)
	snaps := r.srv.GroundItems.Snapshots(nil)
	if len(snaps) != 1 {
		r.t.Fatalf("ground items = %d, want 1", len(snaps))
	}
	r.srv.Client.Send(encodeTutorialAction(snaps[0].ObjectID, location.Location{X: x, Y: y, Z: z}))
	got := tutorialFrames(r.srv.ReadQueued(r.t, r.srv.Client))
	// The pickup holds the player for 200 ms.
	r.srv.Advance(r.t, 250*time.Millisecond)
	return got
}

// TestTutorialAdenaStep: a newbie below level 6 that picks adena up is
// shown question mark 23 with voice 012, once.
func TestTutorialAdenaStep(t *testing.T) {
	t.Parallel()
	r := bootLevelRig(t, 0)
	expectFrames(t, "first pickup", r.pickUpAdena(),
		serverpackets.FrameTutorialShowQuestionMark(23),
		serverpackets.FramePlaySound(script.SoundTutorial),
		voiceFrame(r.objID, r.at, "tutorial_voice_012"))
	if got := r.srv.QuestVars(t, r.objID, tutorial)["Adena"]; got != "1" {
		t.Fatalf("Adena = %q, want 1", got)
	}
	expectFrames(t, "second pickup", r.pickUpAdena())
}

// TestTutorialAdenaStepLevelSix: from level 6 the pickup shows no step and
// leaves Adena unset.
func TestTutorialAdenaStepLevelSix(t *testing.T) {
	t.Parallel()
	r := bootLevelRig(t, 0)
	r.gainTo(6)
	expectFrames(t, "pickup at 6", r.pickUpAdena())
	if got, ok := r.srv.QuestVars(t, r.objID, tutorial)["Adena"]; ok {
		t.Fatalf("Adena = %q, want unset", got)
	}
}
