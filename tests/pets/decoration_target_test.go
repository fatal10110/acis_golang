package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestChristmasTreeClickAnswersActionFailed clicks a placed Christmas Tree:
// whether plain, shifted or forced, every click is answered with
// ActionFailed alone. The tree is never selected, and the player's current
// target, itself, stays selected.
func TestChristmasTreeClickAnswersActionFailed(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: treeKitID, Count: 1})
	h.client.Send(encodeUseItem(h.seededItem(t, treeKitID), false))
	frame := mustRead(t, h.client, "tree NPCInfo")
	assertFrameOpcode(t, frame, serverpackets.OpcodeNPCInfo, "tree NPCInfo")
	treeID := wire.NewReader(frame[1:]).ReadInt32()
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeAction(h.ownerID, 0, 0, 0, false))
	drainUntilQuiet(t, h.client)
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner not in the world")
	}
	owner, ok := obj.(interface{ Target() world.Tracked })
	if !ok {
		t.Fatalf("owner %T has no target", obj)
	}
	if cur := owner.Target(); cur == nil || cur.ObjectID() != h.ownerID {
		t.Fatalf("owner target = %v, want itself before clicking the tree", cur)
	}

	for _, click := range []struct {
		name    string
		payload []byte
	}{
		{"plain click", encodeAction(treeID, 0, 0, 0, false)},
		{"shift click", encodeAction(treeID, 0, 0, 0, true)},
		{"forced click", encodeAttackRequest(treeID, 0, 0, 0, false)},
		{"forced shift click", encodeAttackRequest(treeID, 0, 0, 0, true)},
	} {
		h.client.Send(click.payload)
		frames := drainFrames(t, h.client)
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
			opcodes := make([]byte, 0, len(frames))
			for _, f := range frames {
				opcodes = append(opcodes, f[0])
			}
			t.Fatalf("%s on the tree answered %x, want ActionFailed alone", click.name, opcodes)
		}
		if cur := owner.Target(); cur == nil || cur.ObjectID() != h.ownerID {
			t.Fatalf("%s on the tree changed the target to %v, want the owner kept", click.name, cur)
		}
	}
}

// TestDecorationNpcInfoCarriesItsFixedStats pins the stats a Christmas Tree
// placed by its kit is shown with once it walks again (SummonItems:
// setWalkOrRun(false)), in its last spawn NpcInfo: the standard NPC stats,
// C.Spd 333 times the WIT 20 bonus 1.0, P.Spd the template 300 times the
// DEX 30 bonus 1.1, the walking flag, the movement multiplier 30 * 1.1 over
// the walk base 30 and the attack speed multiplier (float) (1.1 * 330 /
// 300). It carries no title: the tree keeps its placer's name as its title
// but, using no server-side title, never shows it (AbstractNpcInfo).
func TestDecorationNpcInfoCarriesItsFixedStats(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: treeKitID, Count: 1})
	r := lastTreeNpcInfo(t, h)
	for range 8 { // id, template, attackable, x, y, z, heading, 0
		r.ReadInt32()
	}
	if mAtkSpd, pAtkSpd := r.ReadInt32(), r.ReadInt32(); mAtkSpd != 333 || pAtkSpd != 330 {
		t.Fatalf("tree C.Spd/P.Spd = %d/%d, want 333/330", mAtkSpd, pAtkSpd)
	}
	for range 8 { // run/walk speed pairs
		r.ReadInt32()
	}
	walk, dexBonus, pAtkSpd, base := 30.0, 1.1, 330.0, 300.0
	wantMove := float64(float32(walk*dexBonus) / float32(walk))
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
	if running := r.ReadUint8(); running != 0 {
		t.Fatalf("tree running flag = %d, want 0", running)
	}
	if title := readNpcInfoTitle(r); title != "" {
		t.Fatalf("tree title = %q, want none", title)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read tree NPCInfo: %v", err)
	}
}

// TestDecorationNpcInfoShowsAServerSideTitle places a tree whose template
// uses a server-side title: its NpcInfo shows the title it was placed with,
// its placer's name.
func TestDecorationNpcInfoShowsAServerSideTitle(t *testing.T) {
	t.Parallel()
	tree := treeTemplate()
	tree.UsingServerSideTitle = true
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), tree})),
	}, seedItem{TemplateID: treeKitID, Count: 1})
	r := lastTreeNpcInfo(t, h)
	for range 8 + 2 + 8 { // id .. heading, 0, C.Spd, P.Spd, speeds
		r.ReadInt32()
	}
	for range 4 { // multipliers, collision radius, height
		r.ReadFloat64()
	}
	for range 3 { // right hand, chest, left hand
		r.ReadInt32()
	}
	r.ReadUint8() // name above char
	r.ReadUint8() // running
	if title := readNpcInfoTitle(r); title != "Owner" || r.Err() != nil {
		t.Fatalf("tree title = %q (%v), want its placer's name Owner", title, r.Err())
	}
}

// lastTreeNpcInfo places the tree with its kit and returns a reader past
// the opcode of the last NpcInfo the placer is shown.
func lastTreeNpcInfo(t *testing.T, h *petWorld) *wire.Reader {
	t.Helper()
	h.client.Send(encodeUseItem(h.seededItem(t, treeKitID), false))
	var frame []byte
	for _, f := range drainFrames(t, h.client) {
		if f[0] == serverpackets.OpcodeNPCInfo {
			frame = f
		}
	}
	if frame == nil {
		t.Fatal("placing the tree showed no NpcInfo")
	}
	return wire.NewReader(frame[1:])
}

// readNpcInfoTitle reads, from an NpcInfo read up to its running flag, its
// title.
func readNpcInfoTitle(r *wire.Reader) string {
	for range 3 { // in combat, alike dead, summon animation
		r.ReadUint8()
	}
	r.ReadString() // name
	return r.ReadString()
}
