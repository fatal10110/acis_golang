package character

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestEnterWorldRecomputesRestoredWeight is the behavior-suite regression
// test for issue #1144: RestorePlayerInventory rebuilds the inventory from
// persisted rows without queuing update notifications, so totalWeight stays
// 0 unless attachLivePlayer recomputes it, matching the reference's
// ItemList constructor invoking PcInventory.updateWeight() on every send,
// including the one EnterWorld makes at login (ItemList.java:14-24,
// PcInventory.java:101-113, EnterWorld.java:223). The client-visible proof
// is the login StatusUpdate(CUR_LOAD) arriving ahead of the fixed EnterWorld
// burst.
func TestEnterWorldRecomputesRestoredWeight(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
			objID := seedCharacter(t, chars, "Newbie", 1, 0)
			if err := items.Create(context.Background(), objID, item.Instance{
				ObjectID: 500, TemplateID: 9500, OwnerID: objID, Count: 5, Location: item.LocationInventory,
			}); err != nil {
				t.Fatalf("seed item: %v", err)
			}
		}),
	)
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected

	objID := srv.SoleObjectID(t)

	// attachLivePlayer's explicit weight recompute detects restore's still-
	// zero totalWeight changing to the real carried weight and fires the
	// weight notifier immediately — before EnterWorld's own fixed frame
	// burst — so the login StatusUpdate(CUR_LOAD) arrives first.
	c.Send(encodeEnterWorld())
	reply := c.Read()
	assertStatusAttrs(t, reply, objID, []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentLoad, Value: 50},
	})
	readEnterWorldBurst(t, c)

	if _, ok := srv.State.Player(objID); !ok {
		t.Fatalf("world.Player(%d) missing after EnterWorld", objID)
	}
}

// TestEnterWorldReGrantsFreeSkills is the behavior-suite regression test
// for issue #1149: Player.giveSkills() runs again on every login, right
// after restoreCharData() (Player.java:4139), so a free level-unlocked
// grant — which a prior in-session level-up handed out in memory only, per
// GiveSkills's own doc comment — comes back on relog instead of staying
// dropped. The shared test template grants skill 900001 for free from
// level 50; no other flow's character reaches that level, so this is the
// only enter-world path affected by the added grant.
func TestEnterWorldReGrantsFreeSkills(t *testing.T) {
	skills := skillstate.NewPersistence(nil, modelskill.NewTable([]modelskill.Definition{{ID: 900001, Level: 1}}))
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(skills),
		gameservertest.WithCharacter("Newbie", 50, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c)

	skillList := frames[5]
	if skillList[0] != serverpackets.OpcodeSkillList {
		t.Fatalf("frame[5] opcode = %#x, want SkillList (%#x)", skillList[0], serverpackets.OpcodeSkillList)
	}
	r := wire.NewReader(skillList[1:])
	count := r.ReadInt32()
	for range count {
		if _, level, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id == 900001 {
			if level != 1 {
				t.Fatalf("skill 900001 level = %d, want 1", level)
			}
			return
		}
	}
	t.Fatalf("SkillList (%d entries) missing free grant skill 900001 re-derived on login", count)
}

func seedCharacter(t *testing.T, chars *gamesql.CharacterStore, name string, level, sp int) int32 {
	t.Helper()
	tmpl, ok := gameservertest.Templates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	ch, err := player.NewCharacter(100, tmpl, "player1", name, 1, 0, 0, player.SexMale)
	if err != nil {
		t.Fatalf("seed character: %v", err)
	}
	ch.CharLevel = level
	ch.SP = sp
	if err := chars.Create(context.Background(), ch); err != nil {
		t.Fatalf("seed character store: %v", err)
	}
	return ch.ID
}
