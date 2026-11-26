package clan

import (
	"slices"
	"strings"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAdminClanSkillCatalogFollowsTree pins that //clan_skill offers the
// clan skills of the clan skill tree, not a fixed id range: with Clan
// Spirituality (371) dropped from the tree the page does not list it, setting
// it answers the usage with nothing learned, and set all grants every
// other clan skill but it.
func TestAdminClanSkillCatalogFollowsTree(t *testing.T) {
	const dropped = 371
	_, shipped := shippedSkillData(t)
	trimmed := *shipped
	trimmed.Clan = slices.DeleteFunc(slices.Clone(shipped.Clan), func(n modelskill.ClanSkill) bool { return n.ID == dropped })
	if len(trimmed.Clan) == len(shipped.Clan) {
		t.Fatalf("shipped clan tree has no skill %d", dropped)
	}
	w := bootAdminClanWorld(t, 1000, gameservertest.WithSkillTrees(&trimmed))

	for _, args := range []string{"1", "2"} {
		gm, _, _ := w.clanSkill(t, args)
		_, page := gmReply(t, gm)
		if page == "" || strings.Contains(page, "<td>371</td>") {
			t.Fatalf("//clan_skill %s page lists skill %d:\n%s", args, dropped, page)
		}
	}

	gm, leader, member := w.clanSkill(t, "set 371 1")
	texts, page := gmReply(t, gm)
	if !slices.Equal(texts, []string{"Usage: //clan_skill set id level [page]"}) || !strings.Contains(page, "<font color=LEVEL>01</font>") {
		t.Fatalf("//clan_skill set 371 1 told %q, page:\n%s", texts, page)
	}
	if len(leader) != 0 || len(member) != 0 {
		t.Fatalf("//clan_skill set 371 1 sent the members %x and %x", opcodes(leader), opcodes(member))
	}
	if stored := storedClanSkills(t, w.clanWorld); len(stored) != 0 {
		t.Fatalf("clan_skills after set 371 1 = %v, want none", stored)
	}

	gm, _, _ = w.clanSkill(t, "set all")
	if texts, _ := gmReply(t, gm); !slices.Equal(texts, []string{"You gave all available skills to Seeded clan."}) {
		t.Fatalf("//clan_skill set all told %q", texts)
	}
	stored := storedClanSkills(t, w.clanWorld)
	if _, ok := stored[dropped]; ok || len(stored) != clanSkillCount-1 {
		t.Fatalf("clan_skills after set all = %v, want the %d clan skills but %d", stored, clanSkillCount-1, dropped)
	}
}
