package network

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestNpcDropContentIterationsRate pins the rate each drop page shows its
// category with (DropType.getDropRate): the spoil, currency and herb rates
// for their kinds, and for a normal drop the item rate, or the raid item
// rate on a raid or grand boss.
func TestNpcDropContentIterationsRate(t *testing.T) {
	l := &GameClientLink{itemTemplates: item.NewTable([]*item.Template{{ID: 1, Name: "Pelt", Kind: item.KindEtcItem}})}
	drop := []item.Drop{{ItemID: 1, Min: 1, Max: 1, Chance: 50}}
	tmpl := &npc.Template{ID: 1, Drops: []item.DropCategory{
		{Kind: item.DropNormal, Chance: 70, Drops: drop},
		{Kind: item.DropCurrency, Chance: 100, Drops: drop},
		{Kind: item.DropHerb, Chance: 100, Drops: drop},
		{Kind: item.DropSpoil, Chance: 100, Drops: drop},
	}}
	rates := item.Rates{Item: 2, Currency: 3, Spoil: 4, ItemRaid: 5, Herb: 6}
	for _, tc := range []struct {
		name   string
		inst   *npc.Instance
		isDrop bool
		page   int
		want   string
	}{
		{"monster drop", &npc.Instance{Template: withType(tmpl, "Monster")}, true, 1, "Category: DROP - Rate: 70% - Iterations: x2<"},
		{"currency", &npc.Instance{Template: withType(tmpl, "Monster")}, true, 2, "Category: CURRENCY - Rate: 100% - Iterations: x3<"},
		{"herb", &npc.Instance{Template: withType(tmpl, "Monster")}, true, 3, "Category: HERB - Rate: 100% - Iterations: x6<"},
		{"spoil", &npc.Instance{Template: withType(tmpl, "Monster")}, false, 1, "Category: SPOIL - Rate: 100% - Iterations: x4<"},
		{"raid boss drop", &npc.Instance{Template: withType(tmpl, "RaidBoss")}, true, 1, "Category: DROP - Rate: 70% - Iterations: x5<"},
		{"grand boss drop", &npc.Instance{Template: withType(tmpl, "GrandBoss")}, true, 1, "Category: DROP - Rate: 70% - Iterations: x5<"},
		{"raid boss currency", &npc.Instance{Template: withType(tmpl, "RaidBoss")}, true, 2, "Category: CURRENCY - Rate: 100% - Iterations: x3<"},
		{"raid boss kind", &npc.Instance{Template: withType(tmpl, "Monster"), Kind: "RaidBoss"}, true, 1, "Category: DROP - Rate: 70% - Iterations: x5<"},
	} {
		content, ok := l.npcDropContent(tc.inst, rates, tc.page, 1, tc.isDrop)
		if !ok || !strings.Contains(content, tc.want) {
			t.Errorf("%s: npcDropContent ok=%v content %q, want it to hold %q", tc.name, ok, content, tc.want)
		}
	}
}

// withType is a copy of tmpl of reference class typ.
func withType(tmpl *npc.Template, typ string) *npc.Template {
	c := *tmpl
	c.Type = typ
	return &c
}
