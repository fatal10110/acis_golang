package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// conditionedSwordID is the fixture D-grade sword, which the test gives a
// use condition.
const conditionedSwordID int32 = 30

func useCond(kind string, attrs map[string]string, children ...item.Condition) item.Condition {
	return item.Condition{Kind: kind, Attrs: attrs, Children: children}
}

// TestItemUseConditionsFollowConditionReader pins an item <cond> to the
// reader skill conditions use (DocumentBase.parseCondition), tested with the
// level-1 user as both sides: an unrecognized attribute beside level is
// ignored, <and>/<or> drop a child holding no condition (<game chance>, an
// attribute-less <player>), and <not> reads only its first child. A failing
// clause answers its message and leaves the sword in the bag; a <not> around
// no condition refuses the use with ActionFailed and no message, since the
// reference's check throws before it reads the clause's message.
func TestItemUseConditionsFollowConditionReader(t *testing.T) {
	t.Parallel()
	gameChance := useCond("game", map[string]string{"chance": "50"})
	level := func(n string) item.Condition { return useCond("player", map[string]string{"level": n}) }
	const (
		equips = iota
		message
		actionFailed
	)
	for _, tt := range []struct {
		name string
		root item.Condition
		want int
	}{
		{"and drops game chance", useCond("and", nil, gameChance, level("1")), equips},
		{"and drops game chance, level 2", useCond("and", nil, gameChance, level("2")), message},
		{"unrecognized attribute beside level 1", useCond("player", map[string]string{"bogus": "1", "level": "1"}), equips},
		{"unrecognized attribute beside level 2", useCond("player", map[string]string{"bogus": "1", "level": "2"}), message},
		{"or drops an empty player", useCond("or", nil, useCond("player", nil), level("99")), message},
		{"not reads its first child only", useCond("not", nil, level("99"), level("1")), equips},
		{"not of game chance", useCond("not", nil, gameChance), actionFailed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := bootConditionedSword(t, item.UseCondition{Root: tt.root, MessageID: serverpackets.SystemMessageCannotEquipItemDueToBadCondition})
			c, objID := srv.Client, srv.SoleObjectID(t)
			sword := srv.GiveItem(t, objID, conditionedSwordID, 1)
			startInWorld(t, c)

			c.Send(encodeUseItem(sword, false))
			switch tt.want {
			case equips:
				assertFrameOpcode(t, readSkippingEquipNoise(t, c, tt.name), serverpackets.OpcodeUserInfo, tt.name)
				drainUntilQuiet(t, c)
			case message:
				assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotEquipItemDueToBadCondition)
				barrier(t, c)
			case actionFailed:
				assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, tt.name)
				barrier(t, c)
			}
			srv.FlushItems(t)
			want := item.LocationInventory
			if tt.want == equips {
				want = item.LocationPaperdoll
			}
			if inst := mustFindItem(t, srv, objID, sword); inst.Location != want {
				t.Fatalf("%s: sword location = %v, want %v", tt.name, inst.Location, want)
			}
		})
	}
}

// bootConditionedSword boots a level-1 "Newbie" whose fixture sword carries
// uc as its only use condition.
func bootConditionedSword(t *testing.T, uc item.UseCondition) *gameservertest.Server {
	t.Helper()
	all := gameservertest.ItemTemplates().All()
	templates := make([]*item.Template, 0, len(all))
	for _, tmpl := range all {
		if tmpl.ID == conditionedSwordID {
			conditioned := *tmpl
			conditioned.UseConditions = []item.UseCondition{uc}
			tmpl = &conditioned
		}
		templates = append(templates, tmpl)
	}
	return gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1))
}
