package items

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	sqltest "github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The rigged sword: the fixture's D-grade sword (template 30) carrying a
// +100 maxHp equip modifier and one passive item skill, so its equip
// side effects show both in UserInfo's MaxHP and in SkillList. The
// character knows Expertise, which a D-grade weapon's item skills need, and
// Crystallize.
const riggedSwordID int32 = 30

const (
	riggedSwordHPBonus    = 100
	riggedSwordSkillID    = 3099
	expertiseSkillID      = 239
	crystallizeSkillID    = 248
	crystallizeSkillLevel = 3
)

// bootRiggedSword boots a character that knows Expertise and Crystallize
// and carries the rigged sword, and returns the server and the sword's
// object id.
func bootRiggedSword(t *testing.T) (*gameservertest.Server, int32) {
	t.Helper()
	var templates []*item.Template
	for _, tmpl := range gameservertest.ItemTemplates().All() {
		if tmpl.ID == riggedSwordID {
			rigged := *tmpl
			rigged.Modifiers = []item.StatModifier{{Op: item.FuncAdd, Stat: "maxHp", Value: riggedSwordHPBonus}}
			rigged.AttachedSkills = []item.SkillRef{{ID: riggedSwordSkillID, Level: 1}}
			tmpl = &rigged
		}
		templates = append(templates, tmpl)
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: expertiseSkillID, Level: 1},
		{ID: crystallizeSkillID, Level: crystallizeSkillLevel},
		{ID: riggedSwordSkillID, Level: 1, Activation: modelskill.ActivationPassive},
	}), gamesql.NewCharacterSkillStore(db))
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithSkills(skills),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	for _, known := range [][2]int{{expertiseSkillID, 1}, {crystallizeSkillID, crystallizeSkillLevel}} {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, known[0], known[1]); err != nil {
			t.Fatalf("grant skill %d: %v", known[0], err)
		}
	}
	return srv, srv.GiveItem(t, objID, riggedSwordID, 1)
}

// userInfoMaxHP decodes MaxHP out of a UserInfo frame.
func userInfoMaxHP(t *testing.T, frame []byte) int32 {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeUserInfo, "UserInfo")
	r := wire.NewReader(frame[1:])
	for range 5 { // x, y, z, heading, object id
		r.ReadInt32()
	}
	r.ReadString()
	for range 4 { // race, sex, class, level
		r.ReadInt32()
	}
	r.ReadInt64()
	for range 6 { // STR..MEN
		r.ReadInt32()
	}
	return r.ReadInt32()
}

// equipSideEffects is what one equip-changing request sent: whether its
// SkillList (ahead of the first UserInfo) lists the rigged sword's skill,
// and the MaxHP that UserInfo carries.
type equipSideEffects struct {
	skillList  bool
	swordSkill bool
	userInfoHP int32
	frameOrder []byte
}

func readEquipSideEffects(t *testing.T, frames [][]byte) equipSideEffects {
	t.Helper()
	var got equipSideEffects
	userInfo := false
	for _, f := range frames {
		got.frameOrder = append(got.frameOrder, f[0])
		switch f[0] {
		case serverpackets.OpcodeSkillList:
			if userInfo {
				t.Fatal("SkillList after UserInfo, want it ahead of the refresh")
			}
			got.skillList = true
			for _, e := range readSkillList(t, f) {
				got.swordSkill = got.swordSkill || e.id == riggedSwordSkillID
			}
		case serverpackets.OpcodeUserInfo:
			if !userInfo {
				got.userInfoHP = userInfoMaxHP(t, f)
			}
			userInfo = true
		}
	}
	if !userInfo {
		t.Fatalf("no UserInfo among opcodes %x", got.frameOrder)
	}
	return got
}

// enterWithRiggedSword brings the character in world and equips the sword
// through UseItem, requiring the modifier and the item skill to land, and
// returns the MaxHP the character had before.
func enterWithRiggedSword(t *testing.T, srv *gameservertest.Server, sword int32) int32 {
	t.Helper()
	c := srv.Client
	baseHP := int32(-1)
	for _, f := range startInWorld(t, c) {
		if f[0] == serverpackets.OpcodeUserInfo {
			baseHP = userInfoMaxHP(t, f)
		}
	}
	if baseHP < 0 {
		t.Fatal("EnterWorld burst has no UserInfo")
	}
	c.Send(encodeUseItem(sword, false))
	on := readEquipSideEffects(t, collectUntilQuiet(t, c))
	if !on.skillList || !on.swordSkill {
		t.Fatalf("equip SkillList sent=%v lists item skill=%v, want both", on.skillList, on.swordSkill)
	}
	if on.userInfoHP != baseHP+riggedSwordHPBonus {
		t.Fatalf("UserInfo MaxHP after equip = %d, want %d", on.userInfoHP, baseHP+riggedSwordHPBonus)
	}
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	return baseHP
}

// TestDropEquippedItemDropsEquipSideEffects pins RequestDropItem on a worn
// item: the unequip runs the same side effects as any other, so the sword's
// maxHp modifier leaves UserInfo and a SkillList without its item skill goes
// out ahead of that UserInfo.
func TestDropEquippedItemDropsEquipSideEffects(t *testing.T) {
	t.Parallel()
	srv, sword := bootRiggedSword(t)
	c := srv.Client
	baseHP := enterWithRiggedSword(t, srv, sword)

	c.Send(encodeRequestDropItem(sword, 1, spawnX, spawnY, spawnZ))
	off := readEquipSideEffects(t, collectUntilQuiet(t, c))
	if !off.skillList || off.swordSkill {
		t.Fatalf("drop SkillList sent=%v lists item skill=%v, want sent without it", off.skillList, off.swordSkill)
	}
	if off.userInfoHP != baseHP {
		t.Fatalf("UserInfo MaxHP after drop = %d, want base %d", off.userInfoHP, baseHP)
	}
	if got := len(srv.GroundItems.Snapshots(nil)); got != 1 {
		t.Fatalf("ground items after drop = %d, want 1", got)
	}
}

// TestCrystallizeEquippedItemDropsEquipSideEffects pins
// RequestCrystallizeItem on a worn weapon: the same unequip side effects,
// then S1_DISARMED naming the weapon ahead of S1_CRYSTALLIZED.
func TestCrystallizeEquippedItemDropsEquipSideEffects(t *testing.T) {
	t.Parallel()
	srv, sword := bootRiggedSword(t)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	baseHP := enterWithRiggedSword(t, srv, sword)

	c.Send(encodeRequestCrystallizeItem(sword, 1))
	frames := collectUntilQuiet(t, c)
	off := readEquipSideEffects(t, frames)
	if !off.skillList || off.swordSkill {
		t.Fatalf("crystallize SkillList sent=%v lists item skill=%v, want sent without it", off.skillList, off.swordSkill)
	}
	if off.userInfoHP != baseHP {
		t.Fatalf("UserInfo MaxHP after crystallize = %d, want base %d", off.userInfoHP, baseHP)
	}
	var messages []int
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			messages = append(messages, systemMessageID(t, f))
		}
	}
	disarmed, crystallized := -1, -1
	for i, id := range messages {
		switch id {
		case serverpackets.SystemMessageS1Disarmed:
			disarmed = i
		case serverpackets.SystemMessageItemCrystallized:
			crystallized = i
		}
	}
	if disarmed < 0 || crystallized < 0 || disarmed > crystallized {
		t.Fatalf("system messages %v: want S1_DISARMED ahead of S1_CRYSTALLIZED", messages)
	}
	srv.FlushItems(t)
	assertItemGone(t, srv, objID, sword)
}

// TestDropWornFormalWearRelightsSkillList pins the formal wear half of the
// drop side effects: dropping the worn dress resends SkillList with every
// entry lit, ahead of the UserInfo refresh.
func TestDropWornFormalWearRelightsSkillList(t *testing.T) {
	t.Parallel()
	srv, objects := bootFormalWear(t)
	c := srv.Client
	startInWorld(t, c)

	c.Send(encodeUseItem(objects[formalWearID], false))
	assertSkillListDisabled(t, collectUntilQuiet(t, c), 1)

	c.Send(encodeRequestDropItem(objects[formalWearID], 1, spawnX, spawnY, spawnZ))
	assertSkillListDisabled(t, collectUntilQuiet(t, c), 0)
}

// readUntilUserInfo reads frames up to and including the next UserInfo and
// returns the system message frames seen before it, failing on any
// SystemMessage-free gap longer than the equip burst.
func readUntilUserInfo(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var messages [][]byte
	for range 20 {
		f := c.Read()
		switch f[0] {
		case serverpackets.OpcodeUserInfo:
			return messages
		case serverpackets.OpcodeSystemMessage:
			messages = append(messages, f)
		}
	}
	t.Fatal("no UserInfo within 20 frames")
	return nil
}

// assertEquipMessage requires frame to be messageID naming itemID, with the
// enchant level ahead of the name when enchant is positive.
func assertEquipMessage(t *testing.T, frame []byte, messageID int, enchant, itemID int32) {
	t.Helper()
	if enchant == 0 {
		assertSystemMessageItem(t, frame, messageID, itemID)
		return
	}
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", got, messageID)
	}
	if params := r.ReadInt32(); params != 2 {
		t.Fatalf("param count = %d, want 2", params)
	}
	if typ, lvl := r.ReadInt32(), r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber || lvl != enchant {
		t.Fatalf("param[0] = type %d value %d, want number %d", typ, lvl, enchant)
	}
	if typ, id := r.ReadInt32(), r.ReadInt32(); typ != serverpackets.SystemMessageParamItemName || id != itemID {
		t.Fatalf("param[1] = type %d value %d, want item name %d", typ, id, itemID)
	}
}

// TestUseItemEquipToggleAnnouncesItem walks UseItem's equip/unequip message
// table: putting a plain item on answers S1_EQUIPPED and an enchanted one
// S1_S2_EQUIPPED; taking it off through UseItem answers S1_DISARMED or
// EQUIPMENT_S1_S2_REMOVED. Each is the only system message of the toggle and
// precedes its UserInfo.
func TestUseItemEquipToggleAnnouncesItem(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		template int32
		enchant  int32
		on, off  int
	}{
		// Client message ids: S1_EQUIPPED 49, S1_S2_EQUIPPED 368,
		// S1_DISARMED 417, EQUIPMENT_S1_S2_REMOVED 1064.
		{name: "plain weapon", template: 30, on: 49, off: 417},
		{name: "enchanted armor", template: 40, enchant: 6, on: 368, off: 1064},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
			c := srv.Client
			objID := srv.SoleObjectID(t)
			obj := srv.GiveItem(t, objID, tc.template, 1)
			if tc.enchant > 0 {
				inst := mustFindItem(t, srv, objID, obj)
				inst.EnchantLevel = int(tc.enchant)
				if err := srv.Items.Update(context.Background(), inst); err != nil {
					t.Fatalf("seed enchant level: %v", err)
				}
			}
			startInWorld(t, c)

			for _, step := range []struct {
				what string
				msg  int
			}{{"equip", tc.on}, {"unequip", tc.off}} {
				c.Send(encodeUseItem(obj, false))
				messages := readUntilUserInfo(t, c)
				if len(messages) != 1 {
					t.Fatalf("%s sent %d system messages before UserInfo, want 1", step.what, len(messages))
				}
				assertEquipMessage(t, messages[0], step.msg, tc.enchant, tc.template)
				drainUntilQuiet(t, c)
				srv.InventoryUpdates.Tick()
				drainUntilQuiet(t, c)
			}
		})
	}
}

// TestUseItemWeaponEquipRechargesAutoShots pins the weapon toggle's shot
// handling: equipping a main-hand weapon with an auto-use soulshot recharges
// it (EnabledSoulshot plus the charge visual, after S1_EQUIPPED and ahead of
// UserInfo, one shot consumed), and toggling the weapon off and on again
// discharges it, so a direct use afterwards charges again instead of being a
// silent already-charged no-op.
func TestUseItemWeaponEquipRechargesAutoShots(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	shot := srv.GiveItem(t, objID, 1463, 10)
	startInWorld(t, c)

	c.Send(encodeRequestAutoSoulShot(1463, 1))
	assertExAutoSoulShot(t, c.Read(), 1463, true)
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(weapon, false))
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageS1Equipped, 30)
	frame := c.Read()
	if id := systemMessageID(t, frame); id != serverpackets.SystemMessageEnabledSoulshot {
		t.Fatalf("message after S1_EQUIPPED = %d, want EnabledSoulshot (%d)", id, serverpackets.SystemMessageEnabledSoulshot)
	}
	assertMagicSkillUseSelf(t, c.Read(), objID, 2150, 1, 0, 0)
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
	drainUntilQuiet(t, c)
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, shot); inst.Count != 9 {
		t.Fatalf("shot count after the equip recharge = %d, want 9", inst.Count)
	}

	// Turn automation off, then take the charged weapon off and put it
	// back on: the charge must not survive the toggle.
	c.Send(encodeRequestAutoSoulShot(1463, 0))
	assertExAutoSoulShot(t, c.Read(), 1463, false)
	drainUntilQuiet(t, c)
	for range 2 {
		c.Send(encodeUseItem(weapon, false))
		readUntilUserInfo(t, c)
		drainUntilQuiet(t, c)
	}
	c.Send(encodeUseItem(shot, false))
	reply := c.ReadWithTimeout(time.Second)
	if reply == nil {
		t.Fatal("direct soulshot use after re-equipping was silent: the weapon kept its charge through the unequip")
	}
	if id := systemMessageID(t, reply); id != serverpackets.SystemMessageEnabledSoulshot {
		t.Fatalf("direct soulshot use after re-equipping = message %d, want EnabledSoulshot (%d)", id, serverpackets.SystemMessageEnabledSoulshot)
	}
	drainUntilQuiet(t, c)
}

// framesUntilUserInfo reads frames up to the next UserInfo and returns the
// ones before it.
func framesUntilUserInfo(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 20 {
		f := c.Read()
		if f[0] == serverpackets.OpcodeUserInfo {
			return frames
		}
		frames = append(frames, f)
	}
	t.Fatal("no UserInfo within 20 frames")
	return nil
}

// assertSilentWeaponEquip requires an equip burst whose only system message
// is S1_EQUIPPED for templateID, with no ActionFailed, no ExAutoSoulShot and
// no charge visual ahead of UserInfo.
func assertSilentWeaponEquip(t *testing.T, frames [][]byte, templateID int32, what string) {
	t.Helper()
	var messages [][]byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSystemMessage:
			messages = append(messages, f)
		case serverpackets.OpcodeActionFailed:
			t.Fatalf("%s: ActionFailed ahead of UserInfo, want none for a server-driven recharge", what)
		case serverpackets.OpcodeMagicSkillUse:
			t.Fatalf("%s: charge visual (MagicSkillUse) ahead of UserInfo, want no recharge", what)
		case serverpackets.OpcodeExtended:
			if len(f) >= 3 && wire.NewReader(f[1:]).ReadUint16() == serverpackets.OpcodeExAutoSoulShot {
				t.Fatalf("%s: ExAutoSoulShot ahead of UserInfo, want the auto-use icon left alone", what)
			}
		}
	}
	if len(messages) != 1 {
		t.Fatalf("%s: %d system messages ahead of UserInfo, want only S1_EQUIPPED", what, len(messages))
	}
	assertSystemMessageItem(t, messages[0], serverpackets.SystemMessageS1Equipped, templateID)
}

// autoSoulShotEnabled reads whether objID has itemID set to auto-use.
func autoSoulShotEnabled(t *testing.T, srv *gameservertest.Server, objID, itemID int32) bool {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d not online", objID)
	}
	holder, ok := obj.(interface{ AutoSoulShotEnabled(int32) bool })
	if !ok {
		t.Fatalf("player %d = %T has no auto-shot state", objID, obj)
	}
	return holder.AutoSoulShotEnabled(itemID)
}

// TestUseItemWeaponEquipAutoShotGradeMismatchIsSilent pins a rejected
// auto-use recharge: equipping the D-grade sword with a C-grade soulshot on
// auto-use is refused by the grade gate without a word, since the stack is
// auto-enabled and no click is pending. The equip burst carries only
// S1_EQUIPPED ahead of UserInfo, the stack keeps every shot, and auto-use
// stays on.
func TestUseItemWeaponEquipAutoShotGradeMismatchIsSilent(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	shot := srv.GiveItem(t, objID, 1464, 10)
	startInWorld(t, c)

	c.Send(encodeRequestAutoSoulShot(1464, 1))
	assertExAutoSoulShot(t, c.Read(), 1464, true)
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(weapon, false))
	assertSilentWeaponEquip(t, framesUntilUserInfo(t, c), 30, "grade-mismatched auto soulshot")
	for _, f := range collectUntilQuiet(t, c) {
		if f[0] == serverpackets.OpcodeActionFailed || f[0] == serverpackets.OpcodeSystemMessage {
			t.Fatalf("frame %#x after the equip UserInfo, want no rejection reply at all", f[0])
		}
	}
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, shot); inst.Count != 10 {
		t.Fatalf("shot count after the rejected recharge = %d, want 10", inst.Count)
	}
	if !autoSoulShotEnabled(t, srv, objID, 1464) {
		t.Fatal("grade mismatch turned auto-use off, want it left on")
	}
}

// TestUseItemWeaponEquipDropsStaleAutoShot pins the stale auto-use entry: a
// soulshot stack destroyed whole while on auto-use leaves its id behind, and
// the next weapon equip removes it without a packet. Picking a new stack of
// the same shot up does not revive the entry, so a later equip does not recharge until
// auto-use is turned on again.
func TestUseItemWeaponEquipDropsStaleAutoShot(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	shot := srv.GiveItem(t, objID, 1463, 10)
	startInWorld(t, c)

	c.Send(encodeRequestAutoSoulShot(1463, 1))
	assertExAutoSoulShot(t, c.Read(), 1463, true)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestDestroyItem(shot, 10))
	drainUntilQuiet(t, c)
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	if carriedCount(t, srv, objID, 1463) != 0 {
		t.Fatal("soulshot stack still carried after destroying all of it")
	}
	if !autoSoulShotEnabled(t, srv, objID, 1463) {
		t.Fatal("destroying the stack cleared auto-use; the stale-entry path needs it left behind")
	}

	c.Send(encodeUseItem(weapon, false))
	assertSilentWeaponEquip(t, framesUntilUserInfo(t, c), 30, "stale auto-use entry")
	drainUntilQuiet(t, c)
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	if autoSoulShotEnabled(t, srv, objID, 1463) {
		t.Fatal("equip kept the auto-use entry of a stack no longer carried")
	}

	// Take the weapon off, pick up a fresh stack of the same shot, and put
	// the weapon on again: the entry is gone, so nothing recharges.
	c.Send(encodeUseItem(weapon, false))
	readUntilUserInfo(t, c)
	drainUntilQuiet(t, c)
	srv.SeedGroundItem(t, objID, 1463, 10, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	c.Send(encodeAction(soleGroundObjectID(t, srv), spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "pickup pending-action release")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, "GetItem")
	drainUntilQuiet(t, c)
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	if got := carriedCount(t, srv, objID, 1463); got != 10 {
		t.Fatalf("soulshots carried after pickup = %d, want 10", got)
	}

	c.Send(encodeUseItem(weapon, false))
	assertSilentWeaponEquip(t, framesUntilUserInfo(t, c), 30, "re-equip after pickup")
	drainUntilQuiet(t, c)
	if got := carriedCount(t, srv, objID, 1463); got != 10 {
		t.Fatalf("soulshots carried after re-equip = %d, want 10 (no recharge without auto-use)", got)
	}

	// Turning auto-use back on makes the next equip recharge again.
	c.Send(encodeRequestAutoSoulShot(1463, 1))
	assertExAutoSoulShot(t, c.Read(), 1463, true)
	drainUntilQuiet(t, c)
	c.Send(encodeUseItem(weapon, false))
	readUntilUserInfo(t, c)
	drainUntilQuiet(t, c)
	c.Send(encodeUseItem(weapon, false))
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageS1Equipped, 30)
	if id := systemMessageID(t, c.Read()); id != serverpackets.SystemMessageEnabledSoulshot {
		t.Fatalf("message after S1_EQUIPPED with auto-use back on = %d, want EnabledSoulshot (%d)", id, serverpackets.SystemMessageEnabledSoulshot)
	}
	drainUntilQuiet(t, c)
	if got := carriedCount(t, srv, objID, 1463); got != 9 {
		t.Fatalf("soulshots carried after the re-enabled recharge = %d, want 9", got)
	}
}
