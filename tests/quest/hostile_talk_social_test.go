package quest

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// socialFrom counts the SocialAction frames of the NPC objectID.
func socialFrom(frames [][]byte, objectID int32) int {
	n := 0
	for _, f := range frames {
		if len(f) >= 5 && f[0] == serverpackets.OpcodeSocialAction && int32(binary.LittleEndian.Uint32(f[1:5])) == objectID {
			n++
		}
	}
	return n
}

// TestGuardTalkAnimationWaitsOutASocialHold: a guard's talk animation runs
// on the clock of the guard's social desire. While a social desire's hold
// is pending, and for the 12 s after it ends, a player's interact plays no
// animation; past that it plays one. A guard with no social played animates
// on the first interact.
func TestGuardTalkAnimationWaitsOutASocialHold(t *testing.T) {
	t.Parallel()
	w := bootDialog(t, map[string]string{"guard/30039.htm": "<html><body>Guard</body></html>"}, nil)
	held := w.spawnHostile(t, "held", guardID, "Guard", 20)
	fresh := w.spawnHostile(t, "fresh", guardID, "Guard", 40)

	if err := held.TickThink(); err != nil {
		t.Fatalf("spawn-cycle TickThink() error: %v", err)
	}
	w.srv.ReadQueued(t, w.srv.Client)
	const hold = 30 * time.Second
	script.NewNPC(held).AddSocialDesire(2, int(hold/time.Millisecond), 50)
	if err := held.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := socialFrom(w.srv.ReadQueued(t, w.srv.Client), held.ObjectID()); got != 1 {
		t.Fatalf("social desire animations = %d, want 1", got)
	}

	if got := socialFrom(w.interact(t, held.ObjectID()), held.ObjectID()); got != 0 {
		t.Fatalf("interact inside the social hold: %d animations, want none", got)
	}
	w.srv.Advance(t, hold+12*time.Second)
	if got := socialFrom(w.talkAgain(t, held), held.ObjectID()); got != 0 {
		t.Fatalf("interact 12 s after the hold ended: %d animations, want none", got)
	}
	w.srv.Advance(t, time.Second)
	if got := socialFrom(w.talkAgain(t, held), held.ObjectID()); got != 1 {
		t.Fatalf("interact past 12 s after the hold: %d animations, want 1", got)
	}

	if got := socialFrom(w.interact(t, fresh.ObjectID()), fresh.ObjectID()); got != 1 {
		t.Fatalf("first interact with a guard that played no social: %d animations, want 1", got)
	}
}

// talkAgain talks to the already selected NPC h once more.
func (w *dialogWorld) talkAgain(t *testing.T, h *npc.Hostile) [][]byte {
	t.Helper()
	w.srv.Client.Send(encodeTutorialAction(h.ObjectID(), w.at))
	return w.srv.ReadQueued(t, w.srv.Client)
}
