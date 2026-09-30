package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSelectingDecorationSendsValidateLocationFirst clicks a spawned
// Christmas Tree: like any creature target it answers ValidateLocation
// before MyTargetSelected.
func TestSelectingDecorationSendsValidateLocationFirst(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: treeKitID, Count: 1})
	h.client.Send(encodeUseItem(h.seededItem(t, treeKitID), false))
	frame := mustRead(t, h.client, "tree NPCInfo")
	assertFrameOpcode(t, frame, serverpackets.OpcodeNPCInfo, "tree NPCInfo")
	treeID := wire.NewReader(frame[1:]).ReadInt32()
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeAction(treeID, 0, 0, 0, false))
	frame = mustRead(t, h.client, "tree ValidateLocation")
	assertFrameOpcode(t, frame, serverpackets.OpcodeValidateLocation, "tree ValidateLocation")
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != treeID {
		t.Fatalf("ValidateLocation object id = %d, want tree %d", got, treeID)
	}
	frame = mustRead(t, h.client, "tree MyTargetSelected")
	assertFrameOpcode(t, frame, serverpackets.OpcodeMyTargetSelected, "tree MyTargetSelected")
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != treeID {
		t.Fatalf("MyTargetSelected object id = %d, want tree %d", got, treeID)
	}
}

// TestDecorationNpcInfoCarriesItsFixedStats pins the speeds a placed
// Christmas Tree is shown with. It is a civilian NPC spawned in the run
// stance (Npc.onSpawn), so its NpcInfo carries the standard NPC stats:
// C.Spd 333 times the WIT 20 bonus 1.0, P.Spd the template 300 times the
// DEX 30 bonus 1.1, the running flag, the movement multiplier 60 * 1.1
// over the run base 60 and the attack speed multiplier
// (float) (1.1 * 330 / 300).
func TestDecorationNpcInfoCarriesItsFixedStats(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: treeKitID, Count: 1})
	h.client.Send(encodeUseItem(h.seededItem(t, treeKitID), false))
	frame := mustRead(t, h.client, "tree NPCInfo")
	assertFrameOpcode(t, frame, serverpackets.OpcodeNPCInfo, "tree NPCInfo")

	r := wire.NewReader(frame[1:])
	for range 8 { // id, template, attackable, x, y, z, heading, 0
		r.ReadInt32()
	}
	if mAtkSpd, pAtkSpd := r.ReadInt32(), r.ReadInt32(); mAtkSpd != 333 || pAtkSpd != 330 {
		t.Fatalf("tree C.Spd/P.Spd = %d/%d, want 333/330", mAtkSpd, pAtkSpd)
	}
	for range 8 { // run/walk speed pairs
		r.ReadInt32()
	}
	run, dexBonus, pAtkSpd, base := 60.0, 1.1, 330.0, 300.0
	wantMove := float64(float32(run*dexBonus) / float32(run))
	wantAttack := float64(float32(1.1 * pAtkSpd / base))
	if move, attack := r.ReadFloat64(), r.ReadFloat64(); move != wantMove || attack != wantAttack {
		t.Fatalf("tree multipliers = move %v attack %v, want %v/%v", move, attack, wantMove, wantAttack)
	}
	for range 2 { // collision radius, height
		r.ReadFloat64()
	}
	for range 3 { // right hand, chest, left hand
		r.ReadInt32()
	}
	r.ReadUint8() // name above char
	if running := r.ReadUint8(); running != 1 {
		t.Fatalf("tree running flag = %d, want 1", running)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read tree NPCInfo: %v", err)
	}
}
