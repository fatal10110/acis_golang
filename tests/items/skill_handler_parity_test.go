package items

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// SP Scroll: Low Grade casts Scroll of SP level 1: GIVE_SP, target SELF,
// power 500, hitTime 200 (static), reuseDelay 3000
// (aCis_datapack/data/xml/items/5500-5599.xml:679-689,
// aCis_datapack/data/xml/skills/2100-2199.xml:682-696).
const (
	spScrollItemID  = 5593
	spScrollSkillID = 2167
	spScrollPower   = 500
)

// TestUseSPScrollGrantsSPAndConsumes drives the SP scroll end to end: the
// item-carried GIVE_SP cast grants its power as SP with the self-only
// UserInfo, StatusUpdate(SP) and ACQUIRED_S1_SP message, consumes one scroll,
// and the SP survives logout into the character row.
func TestUseSPScrollGrantsSPAndConsumes(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, spScrollItemID, 2)
	startInWorld(t, c)

	c.Send(encodeUseItem(scroll, false))
	// The reuse field scales with casting speed, so only the identity and the
	// static hit time are pinned here.
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	if caster, target, skill, level, hit, _ := decodeMagicSkillUse(frame); caster != objID || target != objID || skill != spScrollSkillID || level != 1 || hit != 200 {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d-%d hit %d, want %d/%d %d-1 hit 200",
			caster, target, skill, level, hit, objID, objID, spScrollSkillID)
	}
	readSPGain(t, c, spScrollPower)

	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, scroll, 1)
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, scroll); inst.Count != 1 {
		t.Fatalf("persisted scroll count = %d, want 1", inst.Count)
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	if !c.AwaitClose(5 * time.Second) {
		t.Fatal("server kept the connection open after Logout")
	}
	srv.FlushPersistence(t)
	saved, err := srv.Chars.Get(context.Background(), objID)
	if err != nil {
		t.Fatalf("load saved character: %v", err)
	}
	if saved.SP != spScrollPower {
		t.Fatalf("persisted SP = %d, want %d", saved.SP, spScrollPower)
	}
}

// readSPGain reads until the ACQUIRED_S1_SP message, requiring the packets
// GIVE_SP's addExpAndSp(0, sp) sends ahead of it, back to back: UserInfo for
// the zero experience add (PlayerStatus.java:478-485), then
// StatusUpdate(SP) with the new total (PlayerStatus.java:881-891), then the
// message (PlayerStatus.java:501-520). The character starts with no SP, so
// the total is sp.
func readSPGain(t *testing.T, c *testsupport.ScriptedClient, sp int32) {
	t.Helper()
	var seen []string
	for range 50 {
		frame := c.ReadWithTimeout(2 * time.Second)
		if frame == nil {
			t.Fatalf("ACQUIRED_S1_SP for %d never arrived", sp)
		}
		r := wire.NewReader(frame[1:])
		kind := "other"
		switch frame[0] {
		case serverpackets.OpcodeUserInfo:
			kind = "userinfo"
		case serverpackets.OpcodeStatusUpdate:
			r.ReadInt32() // object id
			if count, typ := r.ReadInt32(), r.ReadInt32(); count == 1 && typ == int32(serverpackets.StatusSP) {
				if got := r.ReadInt32(); got != sp {
					t.Fatalf("StatusUpdate(SP) = %d, want %d", got, sp)
				}
				kind = "sp"
			}
		case serverpackets.OpcodeSystemMessage:
			if id := r.ReadInt32(); id != serverpackets.SystemMessageAcquiredS1SP {
				t.Fatalf("SystemMessage id = %d, want ACQUIRED_S1_SP (%d)", id, serverpackets.SystemMessageAcquiredS1SP)
			}
			if len(seen) < 2 || seen[len(seen)-2] != "userinfo" || seen[len(seen)-1] != "sp" {
				t.Fatalf("frames before ACQUIRED_S1_SP = %v, want ... userinfo sp", seen)
			}
			if params, typ, got := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); params != 1 || typ != serverpackets.SystemMessageParamNumber || got != sp {
				t.Fatalf("ACQUIRED_S1_SP params = %d type %d value %d, want 1 number %d", params, typ, got, sp)
			}
			return
		}
		seen = append(seen, kind)
	}
	t.Fatal("ACQUIRED_S1_SP not found within 50 frames")
}

// shotWeapon is the live caster surface this test charges and inspects.
type shotWeapon interface {
	SetChargedShot(item.ShotKind, bool)
	ChargedShot(item.ShotKind) bool
}

// TestHealCastSpendsChargedSpiritshot casts a real HEAL with a spiritshot
// charged on the equipped weapon: the cast spends it, so the next spell no
// longer carries the bonus.
func TestHealCastSpendsChargedSpiritshot(t *testing.T) {
	t.Parallel()
	const healSkillID = 1011
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: healSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, StaticHitTime: true, ReuseDelay: 60_000, SkillType: "HEAL", Power: 20,
	}}), gamesql.NewCharacterSkillStore(db))
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(skills),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, healSkillID, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	weapon := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	c.Send(encodeUseItem(weapon, false))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, weapon, 1)
	drainUntilQuiet(t, c)

	live, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("caster missing from the world")
	}
	caster, ok := live.(shotWeapon)
	if !ok {
		t.Fatalf("caster %T exposes no weapon shot charge", live)
	}
	caster.SetChargedShot(item.ShotSpirit, true)

	c.Send(encodeRequestMagicSkillUse(healSkillID))
	srv.AdvanceUntil(t, "spiritshot spent by the heal", func() bool { return !caster.ChargedShot(item.ShotSpirit) })
	drainUntilQuiet(t, c)
}

// TestHealCastWithBlessedSpiritshotAddsHealSpsBonus casts a real HEAL with
// a blessed spiritshot charged: the heal adds the healSps correction on top
// of power and the caster's M.Atk term (unscaled for a fighter-class
// player), and the cast spends the shot.
func TestHealCastWithBlessedSpiritshotAddsHealSpsBonus(t *testing.T) {
	t.Parallel()
	const (
		healSkillID = 1011
		power       = 1
		correction  = 10
	)
	healSps, err := modelskill.NewHealSpsTable([]modelskill.HealSps{{MagicLevel: 1, Correction: correction}})
	if err != nil {
		t.Fatalf("NewHealSpsTable() error: %v", err)
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: healSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, StaticHitTime: true, ReuseDelay: 60_000, SkillType: "HEAL", Power: power, MagicLevel: 1,
	}}), gamesql.NewCharacterSkillStore(db))
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(skills),
		gameservertest.WithHealSps(healSps),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, healSkillID, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	weapon := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	c.Send(encodeUseItem(weapon, false))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, weapon, 1)
	drainUntilQuiet(t, c)

	live, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("caster missing from the world")
	}
	caster, ok := live.(interface {
		shotWeapon
		MAtk() float64
	})
	if !ok {
		t.Fatalf("caster %T exposes no weapon shot charge or M.Atk", live)
	}
	caster.SetChargedShot(item.ShotBlessedSpirit, true)

	// Fighter class: power + correction + sqrt(1 * M.Atk).
	want := int(power + (correction + math.Sqrt(float64(int(caster.MAtk())))))
	maxHP := srv.PlayerMaxHP(t, objID)
	if want >= maxHP-1 {
		t.Fatalf("boosted heal %d leaves no headroom above 1 HP under max HP %d", want, maxHP)
	}
	srv.DamagePlayerHP(t, objID, srv.PlayerCurrentHP(t, objID)-1)
	before := srv.PlayerCurrentHP(t, objID)

	c.Send(encodeRequestMagicSkillUse(healSkillID))
	srv.AdvanceUntil(t, "blessed spiritshot spent by the heal", func() bool { return !caster.ChargedShot(item.ShotBlessedSpirit) })
	drainUntilQuiet(t, c)
	if got := srv.PlayerCurrentHP(t, objID) - before; got != want {
		t.Fatalf("healed %d HP, want the shot-boosted %d", got, want)
	}
}

// TestContinuousAndDisablerCastsSpendChargedSpiritshot casts a real BUFF
// (continuous handler), a real NEGATE (disablers handler) and each resource
// type that runs the BUFF pass first with a spiritshot charged on the
// equipped weapon: each cast spends the charge it found at cast start,
// blessed or plain.
func TestContinuousAndDisablerCastsSpendChargedSpiritshot(t *testing.T) {
	for _, tc := range []struct {
		name      string
		skillType string
		shot      item.ShotKind
	}{
		{name: "buff blessed", skillType: "BUFF", shot: item.ShotBlessedSpirit},
		{name: "negate plain", skillType: "NEGATE", shot: item.ShotSpirit},
		{name: "heal percent blessed", skillType: "HEAL_PERCENT", shot: item.ShotBlessedSpirit},
		{name: "mana heal percent plain", skillType: "MANAHEAL_PERCENT", shot: item.ShotSpirit},
		{name: "combat point heal blessed", skillType: "COMBATPOINTHEAL", shot: item.ShotBlessedSpirit},
		{name: "balance life plain", skillType: "BALANCE_LIFE", shot: item.ShotSpirit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const skillID = 1040
			db := sqltest.SharedDB(t)
			skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
				ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 500, StaticHitTime: true, ReuseDelay: 60_000, SkillType: tc.skillType,
			}}), gamesql.NewCharacterSkillStore(db))
			srv := gameservertest.Boot(t,
				gameservertest.WithSkills(skills),
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1))
			c := srv.Client
			objID := srv.SoleObjectID(t)
			if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, skillID, 1); err != nil {
				t.Fatalf("seed known skill: %v", err)
			}
			weapon := srv.GiveItem(t, objID, 30, 1)
			startInWorld(t, c)

			c.Send(encodeUseItem(weapon, false))
			assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
			srv.InventoryUpdates.Tick()
			readInventoryUpdateFor(t, c, weapon, 1)
			drainUntilQuiet(t, c)

			live, ok := srv.State.Player(objID)
			if !ok {
				t.Fatal("caster missing from the world")
			}
			caster, ok := live.(shotWeapon)
			if !ok {
				t.Fatalf("caster %T exposes no weapon shot charge", live)
			}
			caster.SetChargedShot(tc.shot, true)

			c.Send(encodeRequestMagicSkillUse(skillID))
			srv.AdvanceUntil(t, "spiritshot spent by the "+tc.skillType, func() bool { return !caster.ChargedShot(tc.shot) })
			drainUntilQuiet(t, c)
		})
	}
}

func encodeRequestMagicSkillUse(skillID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}
