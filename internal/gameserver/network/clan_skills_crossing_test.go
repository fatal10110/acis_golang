package network

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// crossingClan is a level 5 clan holding Clan Vitality 1 with reputation
// score.
func crossingClan(t *testing.T, score int) *clan.Clan {
	t.Helper()
	const id = 268435456
	table := clan.NewTable()
	table.Restore(clan.Snapshot{
		Clans:  []clan.Row{{ID: id, Name: "Crossing", Level: 5, Reputation: score}},
		Skills: []clan.SkillRow{{ClanID: id, Skill: clan.Skill{ID: 370, Level: 1}}},
	}, time.Now(), 1)
	cl, ok := table.Get(id)
	if !ok {
		t.Fatal("restored clan missing")
	}
	return cl
}

// TestReputationCrossingAppliesOnlyOnCurrentSide replays a member's queued
// crossings out of order: a crossing whose side the score has already left
// changes nothing, so the member ends as the current score says. Above 0 a
// stale fall leaves the given skill; at 0 a stale rise gives nothing back.
func TestReputationCrossingAppliesOnlyOnCurrentSide(t *testing.T) {
	link := &GameClientLink{
		log:    zerolog.Nop(),
		skills: skillstate.NewPersistence(nil, skillTable(modelskill.Definition{ID: 370, Level: 1})),
	}

	t.Run("score above 0", func(t *testing.T) {
		member := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
		cl := crossingClan(t, 100)
		link.applyReputationCrossing(member, cl, 1)
		if level := member.SkillLevel(370); level != 1 {
			t.Fatalf("rise above 0 left Clan Vitality at %d, want 1", level)
		}
		link.applyReputationCrossing(member, cl, -1)
		if level := member.SkillLevel(370); level != 1 {
			t.Fatalf("stale fall took Clan Vitality (level %d) while the score is above 0", level)
		}
	})

	t.Run("score at 0", func(t *testing.T) {
		member := newTestLivePlayer(t, 2, &testsupport.FrameCapture{})
		cl := crossingClan(t, 0)
		link.grantClanSkills(member, cl.Skills())
		link.applyReputationCrossing(member, cl, -1)
		if level := member.SkillLevel(370); level > 0 {
			t.Fatalf("fall to 0 left Clan Vitality at %d", level)
		}
		link.applyReputationCrossing(member, cl, 1)
		if level := member.SkillLevel(370); level > 0 {
			t.Fatalf("stale rise gave Clan Vitality %d back at score 0", level)
		}
	})
}
