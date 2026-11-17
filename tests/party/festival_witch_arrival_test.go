package party

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// castOf is one MagicSkillUse among frames.
type castOf struct {
	caster, target, skill, level, hitTime, reuse int32
}

func castsIn(frames [][]byte) []castOf {
	var out []castOf
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeMagicSkillUse {
			continue
		}
		r := wire.NewReader(f[1:])
		out = append(out, castOf{r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()})
	}
	return out
}

// TestFestivalWitchArrivesWithTwoCasts runs a party of two into the Dawn
// festival for level 31 or lower, a bystander standing by the witch's
// place in the arena. As the signup period ends and the witch is placed,
// the bystander sees her cast, on herself, the skill 2003 and then 2133,
// each at level 1 with a hit time of 1 and no reuse.
func TestFestivalWitchArrivesWithTwoCasts(t *testing.T) {
	t.Parallel()
	f := bootFestival(t, []seat{{"Leader", 20}, {"Member", 20}}, 1000, 2)
	f.invite(t, 0, 1, 0)
	f.signUp(t)
	f.srv.SeedCharacterForAt(t, "watcher", "Watcher", 20, 0, location.Location{X: -79150, Y: 113052, Z: -4891})
	wc := f.srv.DialClient(t, "watcher", 1)
	startInWorld(t, wc)
	drainFrames(t, wc)

	f.timers.fire(t, 2*time.Minute)  // the first cycle begins
	f.timers.fire(t, 19*time.Minute) // the signup period ends
	var witch *npc.Folk
	for _, obj := range f.srv.State.Objects() {
		if n, ok := obj.(*npc.Folk); ok && n.Instance.Template.ID == dawnWitchID {
			witch = n
		}
	}
	if witch == nil {
		t.Fatal("the witch is not in the arena")
	}
	want := []castOf{
		{witch.ObjectID(), witch.ObjectID(), 2003, 1, 1, 0},
		{witch.ObjectID(), witch.ObjectID(), 2133, 1, 1, 0},
	}
	frames := drainFrames(t, wc)
	if got := castsIn(frames); !slices.Equal(got, want) {
		t.Fatalf("the bystander saw the casts %+v among %x, want %+v", got, opcodes(frames), want)
	}
}
