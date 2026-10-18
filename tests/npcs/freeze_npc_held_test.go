package npcs

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The hostile NPC's weapon swap, enchant glow and last attacker of the
// script-facing API freeze (#3801): Npc.equipItem, Npc.setEnchantEffect and
// Npc.getLastAttacker, driven through a booted world as a hook calls them.

// Fixture items an NPC holds.
const (
	heldSwordID  = 9101 // the template's right-hand sword, grade C
	heldBowID    = 9102 // a bow with a 1500 ms reuse
	heldShieldID = 9103 // a shield
)

// heldItems boots with the shared catalog plus the fixture weapons and
// shield.
func heldItems() gameservertest.Option {
	templates := append(gameservertest.ItemTemplates().All(),
		&item.Template{
			ID: heldSwordID, Name: "Sword", Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: -1,
			Crystal: item.CrystalC, Weapon: &item.WeaponDetail{Type: item.WeaponSword},
		},
		&item.Template{
			ID: heldBowID, Name: "Bow", Kind: item.KindWeapon, Slot: item.SlotLRHand, Duration: -1,
			Weapon: &item.WeaponDetail{Type: item.WeaponBow, ReuseDelay: 1500},
		},
		&item.Template{
			ID: heldShieldID, Name: "Shield", Kind: item.KindArmor, Slot: item.SlotLHand, Duration: -1,
			Armor: &item.ArmorDetail{Type: item.ArmorShield},
		},
	)
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// npcLook is what an NpcInfo shows of an NPC's hands and glow.
type npcLook struct{ attackable, rightHand, leftHand, enchant int32 }

// readNPCLook decodes the look an NpcInfo frame of objectID shows.
func readNPCLook(t *testing.T, frame []byte, objectID int32) npcLook {
	t.Helper()
	if frame[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("frame % x is not an NpcInfo", frame)
	}
	u32 := func(at int) int32 { return int32(binary.LittleEndian.Uint32(frame[at : at+4])) }
	if got := u32(1); got != objectID {
		t.Fatalf("NpcInfo of %d, want %d", got, objectID)
	}
	// After the opcode: object id, template id, attackable, x, y, z,
	// heading, 0, two attack speeds and eight move speeds, four doubles,
	// then the right hand, the chest and the left hand. The glow is the
	// last int before the flying flag.
	return npcLook{attackable: u32(9), rightHand: u32(105), leftHand: u32(113), enchant: u32(len(frame) - 8)}
}

// heldLooks reads the one NpcInfo each player was sent, requiring it to be
// the only frame, and proves each player got its own copy.
func heldLooks(t *testing.T, w *cutsceneWorld, objectID int32) npcLook {
	t.Helper()
	var frames [][]byte
	for _, c := range []*testsupport.ScriptedClient{w.first, w.second} {
		got := w.srv.ReadQueued(t, c)
		if len(got) != 1 {
			t.Fatalf("frames % x, want one NpcInfo", got)
		}
		frames = append(frames, got[0])
	}
	look := readNPCLook(t, frames[1], objectID)
	if !bytes.Equal(frames[0], frames[1]) {
		t.Fatalf("the players were sent different NpcInfo: % x and % x", frames[0], frames[1])
	}
	frames[0][len(frames[0])-1] ^= 0xff
	if bytes.Equal(frames[0], frames[1]) {
		t.Fatal("the players share one NpcInfo frame")
	}
	return look
}

// TestScriptEquipItemAndEnchantShowNewLook pins Npc.equipItem (a
// broadcast NpcInfo with the new hands, shown as to no particular viewer)
// and Npc.setEnchantEffect (each viewer's NpcInfo with the glow): the
// right-hand item decides the NPC's attack style and reuse from then on,
// and its death puts the template's items back and clears the glow
// without telling anyone. A civilian NPC has neither operation.
func TestScriptEquipItemAndEnchantShowNewLook(t *testing.T) {
	t.Parallel()
	w := bootCutscene(t, heldItems())
	tmpl := spawnMonster(cutsceneMonsterID, "Royal Rush")
	tmpl.RightHand = heldSwordID
	mob := w.srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	folk := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 30868), location.Location{X: w.at.X, Y: w.at.Y + 40, Z: w.at.Z})
	w.srv.ReadQueued(t, w.first)
	w.srv.ReadQueued(t, w.second)
	if got := mob.AttackType(); got != item.WeaponSword {
		t.Fatalf("fixture: the monster attacks with %v, want its template's sword", got)
	}

	w.direct(t, func(*script.Player) { script.NPCOf(mob).EquipItem(heldBowID, heldShieldID) })
	if got, want := heldLooks(t, w, mob.ObjectID()), (npcLook{1, heldBowID, heldShieldID, 0}); got != want {
		t.Fatalf("NpcInfo after the swap = %+v, want %+v", got, want)
	}
	if mob.AttackType() != item.WeaponBow || mob.WeaponReuseDelay() != 1500*time.Millisecond || mob.WeaponGrade() != int(item.CrystalNone) {
		t.Fatalf("after the swap: %v, reuse %v, grade %d; want a bow, 1.5 s, no grade", mob.AttackType(), mob.WeaponReuseDelay(), mob.WeaponGrade())
	}
	if got, want := mob.HeldItemTypeMask(), item.WeaponBow.Mask()|item.ArmorShield.Mask(); got != want {
		t.Fatalf("held item mask = %#x, want the bow's and the shield's %#x", got, want)
	}

	w.direct(t, func(*script.Player) { script.NPCOf(mob).SetEnchantEffect(15) })
	if got, want := heldLooks(t, w, mob.ObjectID()), (npcLook{1, heldBowID, heldShieldID, 15}); got != want {
		t.Fatalf("NpcInfo after the glow = %+v, want %+v", got, want)
	}

	w.direct(t, func(*script.Player) { script.NPCOf(mob).EquipItem(0, 0) })
	if got, want := heldLooks(t, w, mob.ObjectID()), (npcLook{1, 0, 0, 15}); got != want {
		t.Fatalf("NpcInfo with empty hands = %+v, want %+v", got, want)
	}
	if mob.AttackType() != item.WeaponFist || mob.WeaponReuseDelay() != 0 || mob.HeldItemTypeMask() != 0 {
		t.Fatalf("with empty hands: %v, reuse %v, mask %#x; want unarmed", mob.AttackType(), mob.WeaponReuseDelay(), mob.HeldItemTypeMask())
	}

	w.direct(t, func(*script.Player) { script.NPCOf(mob).EquipItem(heldBowID, 0) })
	heldLooks(t, w, mob.ObjectID())
	if !mob.Die(nil, nil) {
		t.Fatal("the monster did not die")
	}
	w.srv.Settle(t)
	for name, c := range map[string]*testsupport.ScriptedClient{"first": w.first, "second": w.second} {
		if infos := pick(w.srv.ReadQueued(t, c), serverpackets.OpcodeNPCInfo); len(infos) != 0 {
			t.Fatalf("%s was sent NpcInfo % x at the death, want none", name, infos)
		}
	}
	if s := mob.NPCInfoSnapshot(); s.RightHand != heldSwordID || s.LeftHand != 0 || s.EnchantEffect != 0 {
		t.Fatalf("corpse view hands %d/%d glow %d, want the template's sword and no glow", s.RightHand, s.LeftHand, s.EnchantEffect)
	}
	if got := mob.AttackType(); got != item.WeaponSword || mob.WeaponGrade() != int(item.CrystalC) {
		t.Fatalf("corpse attack style %v grade %d, want the template's sword", got, mob.WeaponGrade())
	}

	for _, op := range []func(n *script.NPC){
		func(n *script.NPC) { n.EquipItem(heldBowID, 0) },
		func(n *script.NPC) { n.SetEnchantEffect(15) },
		func(n *script.NPC) { n.LastAttacker() },
	} {
		if w.srv.RunScript(t, w.firstID, "Director", func(_ *script.Script, _ *script.Player) { op(script.NPCOf(folk)) }) {
			t.Fatal("a civilian NPC's operation did not abort the invocation")
		}
	}
}

// TestScriptLastAttackerFollowsTheLatestHit pins Npc.getLastAttacker: no
// one before the first hit, then the creature whose damage landed last,
// whichever player it is; a hit on the corpse changes nothing.
func TestScriptLastAttackerFollowsTheLatestHit(t *testing.T) {
	t.Parallel()
	w := bootCutscene(t)
	mob := w.srv.SpawnHostileNPCTemplateAt(t, spawnMonster(cutsceneMonsterID, "Target"), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	viewer, ok := w.srv.State.PlayerByName("Viewer")
	if !ok {
		t.Fatal("the second player is not in the world")
	}
	first, ok := w.srv.State.Player(w.firstID)
	if !ok {
		t.Fatal("the first player is not in the world")
	}
	hit := func(from attackable.Combatant, damage int) {
		t.Helper()
		done := make(chan struct{})
		if !w.srv.PlayerQueue(t, from.ObjectID()).Post(func() {
			defer close(done)
			mob.TakeDamage(damage, from)
		}) {
			t.Fatal("the attacker's queue is closed")
		}
		<-done
		w.srv.Settle(t)
	}
	last := func() int32 {
		t.Helper()
		var id int32 = -1
		w.direct(t, func(*script.Player) {
			if c := script.NPCOf(mob).LastAttacker(); c != nil {
				id = c.ObjectID()
			} else {
				id = 0
			}
		})
		return id
	}

	if got := last(); got != 0 {
		t.Fatalf("last attacker before any hit = %d, want none", got)
	}
	hit(first.(attackable.Combatant), 10)
	if got := last(); got != w.firstID {
		t.Fatalf("last attacker after the first player's hit = %d, want %d", got, w.firstID)
	}
	hit(viewer.(attackable.Combatant), 10)
	if got := last(); got != viewer.ObjectID() {
		t.Fatalf("last attacker after the second player's hit = %d, want %d", got, viewer.ObjectID())
	}
	if !mob.Die(nil, nil) {
		t.Fatal("the monster did not die")
	}
	hit(first.(attackable.Combatant), 10)
	if got := last(); got != viewer.ObjectID() {
		t.Fatalf("last attacker after a hit on the corpse = %d, want still %d", got, viewer.ObjectID())
	}
}
