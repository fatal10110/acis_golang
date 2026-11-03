package clan

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// seedClanSkillRows stores the seeded clan and a level 1 clan_skills row
// of each of ids.
func seedClanSkillRows(t *testing.T, s clanSeed, ids ...int) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		seedClanRows(t, s, db)
		for _, id := range ids {
			if _, err := db.ExecContext(context.Background(), `INSERT INTO clan_skills (clan_id, skill_id, skill_level) VALUES (?,?,1)`, seededClanID, id); err != nil {
				t.Fatalf("seed clan skill %d: %v", id, err)
			}
		}
	})
}

// listedSkillIDs is the skill ids of the PledgeSkillList among frames.
func listedSkillIDs(t *testing.T, frames [][]byte) []int32 {
	t.Helper()
	var ids []int32
	for _, sk := range pledgeSkills(t, frameOf(t, frames, isPledgeSkillList)) {
		ids = append(ids, sk[0])
	}
	return ids
}

// TestClanSkillListInTableOrder pins the order the clan skill list shows
// the clan's skills in: the order of the hash table the clan keeps them
// in. Skill ids share a bucket of the 16-bucket table sixteen apart, and
// such skills list in the order the clan learnt them; the twelfth skill
// doubles the table, which keeps its 32 buckets after skills are removed.
// The list a game master's removal sends and the one a login sends show
// the same order.
func TestClanSkillListInTableOrder(t *testing.T) {
	w := bootAdminClanWorld(t, 1000, seedClanSkillRows(t, clanSeed{level: 5, reputation: 1000}, 386, 371, 384))
	clanSkill := func(cmd string, id int) [][]byte {
		t.Helper()
		gm, leader, member := w.clanSkill(t, fmt.Sprintf(cmd, id))
		if texts, _ := gmReply(t, gm); len(texts) != 1 {
			t.Fatalf("%s: game master told %q, want one line", fmt.Sprintf(cmd, id), texts)
		}
		if len(leader) == 0 || len(member) == 0 {
			t.Fatalf("%s: leader got %d frames, recruit %d, want the clan's skill change", fmt.Sprintf(cmd, id), len(leader), len(member))
		}
		return leader
	}

	// 370 shares 386's bucket and comes after it, learnt later.
	clanSkill("set %d 1", 370)
	if got, want := listedSkillIDs(t, clanSkill("remove %d", 371)), []int32{384, 386, 370}; !slices.Equal(got, want) {
		t.Fatalf("PledgeSkillList = %v, want %v", got, want)
	}

	for _, id := range []int{375, 385, 387, 388, 389, 390, 391, 371, 372} {
		clanSkill("set %d 1", id)
	}
	var leader [][]byte
	for _, id := range []int{384, 385, 386, 387, 388, 389, 390, 371, 372} {
		leader = clanSkill("remove %d", id)
	}
	// 375 and 391 share a bucket of 16 but not of 32.
	want := []int32{391, 370, 375}
	if got := listedSkillIDs(t, leader); !slices.Equal(got, want) {
		t.Fatalf("PledgeSkillList after the removals = %v, want %v", got, want)
	}

	w.leaveWorld(t, w.member)
	drainFrames(t, w.leader)
	burst := startInWorld(t, w.srv.DialClient(t, "player2", 1))
	if got := listedSkillIDs(t, burst); !slices.Equal(got, want) {
		t.Fatalf("login PledgeSkillList = %v, want %v", got, want)
	}
}
