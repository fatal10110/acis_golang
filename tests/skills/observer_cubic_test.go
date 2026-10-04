package skills

import (
	"slices"
	"strconv"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestObserverEntryStopsCubics pins observer entry with a cubic out: every
// cubic is stopped and removed, and the owner is shown once without it
// (one UserInfo) before it jumps to the viewpoint.
func TestObserverEntryStopsCubics(t *testing.T) {
	t.Parallel()
	groups, err := gamexml.LoadObserverGroups(datapack.Path(t, "data", "xml", "observerGroups.xml"))
	if err != nil {
		t.Fatalf("load observer groups: %v", err)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, cubicSummonSkills(900*time.Second, 900*time.Second))),
		gameservertest.WithObserverGroups(groups),
		gameservertest.WithReuseDelays(3*time.Second, 0),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, summonStormCubicSkill, 1)
	startInWorld(t, c)
	c.Send(encodeRequestMagicSkillUse(summonStormCubicSkill, false, false))
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)
	if got := liveCubicIDs(t, srv, objID); !slices.Equal(got, []int{int(cubic.Storm)}) {
		t.Fatalf("cubics before observing = %v, want [%d]", got, cubic.Storm)
	}

	x, y, z := srv.PlayerPosition(t, objID)
	tower := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 31031), location.Location{X: x + 30, Y: y, Z: z})
	tower.SetObserverGroups([]int{618})
	drainUntilQuiet(t, c)
	c.Send(encodeAction(tower.ObjectID(), int32(x), int32(y), int32(z), false))
	c.Send(encodeAction(tower.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, c)
	prefix := "npc_" + strconv.Itoa(int(tower.ObjectID())) + "_"
	c.Send(encodeBypass(prefix + "observe_group 618"))
	drainUntilQuiet(t, c)
	c.Send(encodeBypass(prefix + "observe 632"))
	frames := queueFrames(t, c)

	if got := liveCubicIDs(t, srv, objID); len(got) != 0 {
		t.Fatalf("cubics while observing = %v, want none", got)
	}
	userInfos, jumped := 0, false
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeUserInfo:
			if jumped {
				t.Fatal("UserInfo after the jump, want the cubic removal shown before it")
			}
			userInfos++
		case serverpackets.OpcodeTeleportToLocation:
			jumped = true
		}
	}
	if userInfos != 1 || !jumped {
		t.Fatalf("UserInfo before the jump = %d (jumped %v), want 1", userInfos, jumped)
	}
}
