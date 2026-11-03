package combat

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// abortingHelmID is a test-only helm whose use condition is a <not> around
// a condition the reader holds no node for, so testing it aborts.
const abortingHelmID int32 = 9990

// headSlot is the paperdoll position of a helm, ahead of the right hand.
const headSlot = 6

// TestPKKillSweepEndsOnAnAbortedUseCondition has the PvP-flagged killer
// wear, besides the shipped PK-free knife, a helm whose use condition test
// aborts (<not><game chance/></not>). The helm sits in an earlier paperdoll
// slot than the knife, so the karma gain's sweep of the worn items tests it
// first: the abort ends the sweep before the knife is tested, and the knife
// stays worn, in the world and in the database, with no S1_DISARMED. The
// rest of the kill still runs: the PvP flag task stops and the flag resets.
func TestPKKillSweepEndsOnAnAbortedUseCondition(t *testing.T) {
	t.Parallel()
	knife := shippedItemTemplate(t, apprenticeKnifeID)
	light := *knife
	light.Weight = 0
	helm := &item.Template{
		ID:          abortingHelmID,
		Name:        "Aborting Helm",
		Kind:        item.KindArmor,
		Slot:        item.SlotHead,
		Duration:    -1,
		Destroyable: true,
		Armor:       &item.ArmorDetail{Type: item.ArmorNone},
		UseConditions: []item.UseCondition{{
			Root: item.Condition{Kind: "not", Children: []item.Condition{
				{Kind: "game", Attrs: map[string]string{"chance": "50"}},
			}},
			MessageID: serverpackets.SystemMessageCannotEquipItemDueToBadCondition,
		}},
	}
	templates := append(gameservertest.ItemTemplates().All(), &light, helm)
	var helmObjID int32
	s := bootPKKillSceneSeeded(t, 0,
		[]gameservertest.Option{gameservertest.WithItemTemplates(item.NewTable(templates))},
		func(srv *gameservertest.Server, objID int32) {
			// The aborting test refuses the helm through UseItem, so it
			// enters the world already worn.
			helmObjID = srv.GiveItem(t, objID, abortingHelmID, 1)
			if _, err := srv.DB.ExecContext(context.Background(), `UPDATE items SET loc = 'PAPERDOLL', loc_data = ? WHERE object_id = ?`, headSlot, helmObjID); err != nil {
				t.Fatalf("seed worn helm: %v", err)
			}
		})
	if helm := s.inv.ItemByObjectID(helmObjID); helm == nil || !helm.Equipped() {
		t.Fatalf("helm = %+v after entering the world, want it worn", helm)
	}
	// Every swing hits, so the scenario never waits on a lucky roll.
	s.setRollSource(func(int) int { return 0 })

	selectPlayerTarget(t, s.c, s.victimID)
	s.c.Send(encodeAttackRequest(s.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertKarmaChangeFrames(t, s.c, s.objID, 240)
	s.srv.Settle(t)

	frames := readQuiet(s.c)
	if i := indexOfSystemMessage(frames, 0, serverpackets.SystemMessageS1Disarmed); i >= 0 {
		t.Fatalf("frames after the karma change = %x, want no S1_DISARMED once the helm's test aborts", opcodesOf(frames))
	}
	if !s.inv.ItemByObjectID(s.knifeObjID).Equipped() {
		t.Fatal("knife taken off after the PK kill, want the aborted sweep to leave it worn")
	}
	if !s.inv.ItemByObjectID(helmObjID).Equipped() {
		t.Fatal("helm taken off after the PK kill, want it worn")
	}
	if state := s.killer.PvPFlagState(); state != task.PvPFlagNone {
		t.Fatalf("killer PvP flag = %v after the PK kill, want none", state)
	}
	if n := s.flags.Len(); n != 0 {
		t.Fatalf("PvP flag task tracks %d players after the PK kill, want none", n)
	}

	s.srv.FlushItems(t)
	rows, err := s.srv.Items.ListByOwner(context.Background(), s.objID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	worn := map[int32]bool{}
	for _, inst := range rows {
		if inst.Location == item.LocationPaperdoll {
			worn[inst.ObjectID] = true
		}
	}
	if !worn[s.knifeObjID] || !worn[helmObjID] {
		t.Fatalf("stored worn items = %v, want the knife %d and the helm %d", worn, s.knifeObjID, helmObjID)
	}
}
