package combat

import (
	"context"
	"database/sql"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: CursedWeapon.java and CursedWeaponManager.java. A monster
// kill rolls each weapon not yet out (checkDrop, Monster.java:464); the
// dropped weapon lies protected on the ground with a red sky, an earthquake
// and S2_WAS_DROPPED_IN_THE_S1_REGION for everyone. Obtaining it
// (Player.addItem -> activate) makes the player its holder: karma 9999999,
// PK kills 0, the weapon's skill at stage 1, the weapon worn, full vitals,
// and THE_OWNER_OF_S2_HAS_APPEARED_IN_THE_S1_REGION for everyone. A
// holder's player kill feeds the weapon (onKillUpdatePvPKarma ->
// increaseKills); a holder's death drops it or ends it (doDie -> dropIt);
// the hunger and life timers end it (endOfLife).

const (
	zaricheID    int32 = 8190
	zaricheSkill int32 = 3603
	akamanahID   int32 = 8689
	// cursedPotionID is a catalog item a holder's death must not drop.
	cursedPotionID int32 = 20
	// lrHandSlot is the both-hands body slot a cursed weapon is worn in.
	lrHandSlot int32 = 0x4000
)

// cursedSkillDefs adds the weapon's three passive stages to the kill skill.
func cursedSkillDefs() []modelskill.Definition {
	defs := killSkillDefs()
	for level := 1; level <= 3; level++ {
		defs = append(defs, modelskill.Definition{ID: modelskill.ID(zaricheSkill), Level: level, Activation: modelskill.ActivationPassive})
	}
	return defs
}

// cursedTable is Zariche as cursedWeapons.xml ships it, its skill topping
// out at stage 3, with stageKills as given.
func cursedTable(t *testing.T, stageKills int) *entity.CursedWeaponTable {
	t.Helper()
	zariche := entity.CursedWeapon{
		ItemID:          zaricheID,
		Skill:           modelskill.Ref{ID: modelskill.ID(zaricheSkill), Level: 3},
		Name:            "Demonic Sword Zariche",
		DropRate:        1,
		Duration:        72,
		DurationLost:    24,
		DisappearChance: 50,
		StageKills:      stageKills,
	}
	akamanah := zariche
	akamanah.ItemID, akamanah.Name, akamanah.Skill = akamanahID, "Blood Sword Akamanah", modelskill.Ref{ID: 3629, Level: 1}
	table, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{zariche, akamanah})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable: %v", err)
	}
	return table
}

// cursedTemplates is the shared catalog with Zariche worn in both hands,
// as the datapack declares it, and Akamanah as a copy of it.
func cursedTemplates() *item.Table {
	templates := gameservertest.ItemTemplates().All()
	for i, tmpl := range templates {
		if tmpl.ID == zaricheID {
			lr := *tmpl
			lr.Slot = item.SlotLRHand
			templates[i] = &lr
			akamanah := lr
			akamanah.ID, akamanah.Name = akamanahID, "Blood Sword Akamanah"
			templates = append(templates, &akamanah)
			break
		}
	}
	return item.NewTable(templates)
}

// cursedBoot boots the account's character as seeded by seed, with the
// cursed weapons running.
func cursedBoot(t *testing.T, stageKills int, extra ...gameservertest.Option) *gameservertest.Server {
	t.Helper()
	opts := []gameservertest.Option{
		gameservertest.WithWantChars(1),
		gameservertest.WithItemTemplates(cursedTemplates()),
		gameservertest.WithSkills(combatPersistence(t, cursedSkillDefs())),
		gameservertest.WithCursedWeapons(cursedTable(t, stageKills)),
	}
	return gameservertest.Boot(t, append(opts, extra...)...)
}

// pickUpCursed lays Zariche at id's feet and has its client pick it up,
// returning every frame the pickup sent it.
func pickUpCursed(t *testing.T, srv *gameservertest.Server, c *scriptedClient, id int32) [][]byte {
	t.Helper()
	return pickUpWeapon(t, srv, c, id, zaricheID)
}

// pickUpWeapon lays itemID at id's feet and has its client pick it up,
// returning every frame the pickup sent it.
func pickUpWeapon(t *testing.T, srv *gameservertest.Server, c *scriptedClient, id, itemID int32) [][]byte {
	t.Helper()
	x, y, z := srv.PlayerPosition(t, id)
	srv.SeedGroundItem(t, 0, itemID, 1, x, y, z)
	drainUntilQuiet(t, c)
	ground := groundWeapon(t, srv, itemID)
	c.Send(encodeAction(ground.ObjectID(), int32(x), int32(y), int32(z), false))
	srv.Settle(t)
	return readQuiet(c)
}

// cursedGround returns the Zariche lying in the world.
func cursedGround(t *testing.T, srv *gameservertest.Server) *grounditem.Item {
	t.Helper()
	return groundWeapon(t, srv, zaricheID)
}

// groundWeapon returns the itemID lying in the world.
func groundWeapon(t *testing.T, srv *gameservertest.Server, itemID int32) *grounditem.Item {
	t.Helper()
	for _, obj := range srv.State.Objects() {
		if g, ok := obj.(*grounditem.Item); ok && g.ItemID() == itemID {
			return g
		}
	}
	t.Fatalf("no item %d on the ground", itemID)
	return nil
}

// cursedMessage reads one SystemMessage frame: its id and its parameters,
// each a type followed by its values.
func cursedMessage(frame []byte) (int32, [][]int32) {
	r := wire.NewReader(frame[1:])
	id := r.ReadInt32()
	n := r.ReadInt32()
	params := make([][]int32, 0, n)
	for range n {
		typ := r.ReadInt32()
		p := []int32{typ, r.ReadInt32()}
		if typ == serverpackets.SystemMessageParamZoneName {
			p = append(p, r.ReadInt32(), r.ReadInt32())
		}
		params = append(params, p)
	}
	return id, params
}

// indexOfMessage finds the first SystemMessage frame at or after from
// carrying id with exactly params, or -1; a negative from finds nothing.
func indexOfMessage(frames [][]byte, from int, id int32, params ...[]int32) int {
	if from < 0 {
		return -1
	}
	for i := from; i < len(frames); i++ {
		if frames[i][0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		got, gotParams := cursedMessage(frames[i])
		if got != id || len(gotParams) != len(params) {
			continue
		}
		match := true
		for j := range params {
			if !slices.Equal(gotParams[j], params[j]) {
				match = false
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func zoneParam(x, y, z int) []int32 {
	return []int32{serverpackets.SystemMessageParamZoneName, int32(x), int32(y), int32(z)}
}

func itemParam(id int32) []int32 { return []int32{serverpackets.SystemMessageParamItemName, id} }

func numberParam(n int32) []int32 { return []int32{serverpackets.SystemMessageParamNumber, n} }

// skillListLevel returns the level skillID has in a SkillList frame, 0
// when it is not listed.
func skillListLevel(frame []byte, skillID int32) int32 {
	r := wire.NewReader(frame[1:])
	n := r.ReadInt32()
	for range n {
		r.ReadInt32() // passive
		level, id := r.ReadInt32(), r.ReadInt32()
		r.ReadUint8() // disabled
		if id == skillID {
			return level
		}
	}
	return 0
}

// lastSkillList returns the last SkillList among frames.
func lastSkillList(t *testing.T, frames [][]byte) []byte {
	t.Helper()
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i][0] == serverpackets.OpcodeSkillList {
			return frames[i]
		}
	}
	t.Fatal("no SkillList")
	return nil
}

// userInfoStage reads the cursed weapon stage, UserInfo's last field.
func userInfoStage(frame []byte) int32 {
	return wire.NewReader(frame[len(frame)-4:]).ReadInt32()
}

func cursedRow(t *testing.T, srv *gameservertest.Server) (playerID, karma, pk, stage, next, hungry int32, ok bool) {
	t.Helper()
	srv.FlushPersistence(t)
	err := srv.DB.QueryRowContext(context.Background(),
		"SELECT playerId, playerKarma, playerPkKills, currentStage, numberBeforeNextStage, hungryTime FROM cursed_weapons WHERE itemId = ?", zaricheID).
		Scan(&playerID, &karma, &pk, &stage, &next, &hungry)
	if err == sql.ErrNoRows {
		return 0, 0, 0, 0, 0, 0, false
	}
	if err != nil {
		t.Fatalf("read cursed_weapons: %v", err)
	}
	return playerID, karma, pk, stage, next, hungry, true
}

func encodeRequestCursedWeaponLocation() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestCursedWeaponLocation)
	return w.Bytes()
}

func encodeRequestDropItem(objectID, count int32, x, y, z int) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDropItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	return w.Bytes()
}

func encodeRequestUnEquipItem(bodySlot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUnEquipItem)
	w.WriteInt32(bodySlot)
	return w.Bytes()
}

// TestCursedWeaponDropsFromMonsterKill: before any weapon is out a
// location request is answered with silence. A monster kill whose roll in
// a million comes out under the drop rate drops the first weapon in the
// reference's hash-map order, Akamanah (8689) ahead of Zariche (8190): it
// lands near the corpse,
// outside the ground cleanup; every player, the bystander too, sees the
// red sky, the earthquake at the weapon and the region of the killer, and
// the location request now lists the weapon where it lies, not held.
func TestCursedWeaponDropsFromMonsterKill(t *testing.T) {
	t.Parallel()
	srv := cursedBoot(t, 10, gameservertest.WithCharacter("Newbie", 5, 0))
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 42, 1)
	startInWorld(t, c)
	// The bystander stands far out of sight of the kill.
	bystanderChar := srv.SeedCharacterFor(t, "bystander", "Bystander", 5, 0)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = 50000, y = 50000 WHERE obj_Id = ?", bystanderChar.ID); err != nil {
		t.Fatalf("place bystander: %v", err)
	}
	bystander := srv.DialClient(t, "bystander", 1)
	startInWorld(t, bystander)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, bystander)

	c.Send(encodeRequestCursedWeaponLocation())
	if f := c.ReadWithTimeout(readQuietWindow); f != nil {
		t.Fatalf("location request with no weapon out answered %#x, want silence", f[0])
	}

	setVictimRoll(t, srv, objID, 0)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	castKillSkill(t, srv, c, objID, hostile.ObjectID(), false)
	srv.AdvanceUntil(t, "monster death", func() bool { return hostile.CurrentHP() <= 0 })
	srv.Settle(t)

	ground := groundWeapon(t, srv, akamanahID)
	gx, gy, gz := ground.Position()
	if dx, dy := gx-hostileX, gy-hostileY; dx < -70 || dx > 70 || dy < -70 || dy > 70 {
		t.Fatalf("Akamanah landed at (%d, %d), more than 70 off the corpse", gx, gy)
	}
	if !ground.DestroyProtected() || srv.GroundItems.Len() != 0 {
		t.Fatalf("Akamanah protected = %v, cleanup tracks %d items; want protected and untracked", ground.DestroyProtected(), srv.GroundItems.Len())
	}
	kx, ky, kz := srv.PlayerPosition(t, objID)
	for name, client := range map[string]*scriptedClient{"killer": c, "bystander": bystander} {
		frames := readQuiet(client)
		sky := indexOf(frames, 0, serverpackets.OpcodeExtended, -1)
		for sky >= 0 && wire.NewReader(frames[sky][1:]).ReadUint16() != serverpackets.OpcodeExRedSky {
			sky = indexOf(frames, sky+1, serverpackets.OpcodeExtended, -1)
		}
		if sky < 0 {
			t.Fatalf("%s saw no ExRedSky", name)
		}
		r := wire.NewReader(frames[sky][3:])
		if got := r.ReadInt32(); got != 10 {
			t.Fatalf("%s ExRedSky lasts %d, want 10", name, got)
		}
		quake := indexOf(frames, sky, serverpackets.OpcodeEarthquake, -1)
		if quake < 0 {
			t.Fatalf("%s saw no Earthquake after the red sky", name)
		}
		r = wire.NewReader(frames[quake][1:])
		if x, y, z, power, secs, npc := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); x != int32(gx) || y != int32(gy) || z != int32(gz) || power != 14 || secs != 3 || npc != 0 {
			t.Fatalf("%s Earthquake = (%d, %d, %d) %d/%d/%d, want the weapon's spot 14/3/0", name, x, y, z, power, secs, npc)
		}
		if indexOfMessage(frames, quake, serverpackets.SystemMessageS2WasDroppedInTheS1Region, zoneParam(kx, ky, kz), itemParam(akamanahID)) < 0 {
			t.Fatalf("%s heard no S2_WAS_DROPPED_IN_THE_S1_REGION for the killer's region after the earthquake", name)
		}
	}

	c.Send(encodeRequestCursedWeaponLocation())
	frame := mustRead(t, c, "ExCursedWeaponLocation")
	r := wire.NewReader(frame[1:])
	if op := r.ReadUint16(); frame[0] != serverpackets.OpcodeExtended || op != serverpackets.OpcodeExCursedWeaponLocation {
		t.Fatalf("location answer %#x/%#x, want ExCursedWeaponLocation", frame[0], op)
	}
	if n, id, active, x, y, z := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); n != 1 || id != akamanahID || active != 0 || x != int32(gx) || y != int32(gy) || z != int32(gz) {
		t.Fatalf("locations = %d: %d active %d at (%d, %d, %d), want Akamanah lying at (%d, %d, %d)", n, id, active, x, y, z, gx, gy, gz)
	}
}

// TestCursedWeaponNotRolledOnFailedRoll: a roll at the drop rate drops
// nothing.
func TestCursedWeaponNotRolledOnFailedRoll(t *testing.T) {
	t.Parallel()
	srv := cursedBoot(t, 10, gameservertest.WithCharacter("Newbie", 5, 0))
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 42, 1)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	setVictimRoll(t, srv, objID, 1)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	castKillSkill(t, srv, c, objID, hostile.ObjectID(), false)
	srv.AdvanceUntil(t, "monster death", func() bool { return hostile.CurrentHP() <= 0 })
	srv.Settle(t)
	if active := srv.CursedWeapons.Active(); len(active) != 0 {
		t.Fatalf("weapons out = %+v, want none", active)
	}
}

// TestCursedWeaponPickupMakesHolder: picking up Zariche names it, then
// turns the player chaotic, gives it the stage 1 skill, puts the weapon on
// and announces the owner's region to everyone. No pickup attention line
// is broadcast. Its karma and PK kills are kept in cursed_weapons for when
// the weapon goes, with the first stage's kill count rolled in [5, 15].
func TestCursedWeaponPickupMakesHolder(t *testing.T) {
	t.Parallel()
	srv := cursedBoot(t, 10, gameservertest.WithSeed(seedDropVictim(25, 100, 3)))
	c, id := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	setVictimRoll(t, srv, id, 0)
	x, y, z := srv.PlayerPosition(t, id)

	frames := pickUpCursed(t, srv, c, id)

	picked := indexOfMessage(frames, 0, serverpackets.SystemMessageYouPickedUpS1, itemParam(zaricheID))
	karma := indexOfMessage(frames, picked, serverpackets.SystemMessageYourKarmaHasBeenChangedToS1, numberParam(9999999))
	equipped := indexOfMessage(frames, karma, serverpackets.SystemMessageS1Equipped, itemParam(zaricheID))
	appeared := indexOfMessage(frames, equipped, serverpackets.SystemMessageOwnerOfS2AppearedInS1Region, zoneParam(x, y, z), itemParam(zaricheID))
	if picked < 0 || karma < 0 || equipped < 0 || appeared < 0 {
		t.Fatalf("picked up %d, karma %d, equipped %d, appeared %d: want each, in that order", picked, karma, equipped, appeared)
	}
	skills := indexOf(frames, karma, serverpackets.OpcodeSkillList, -1)
	if skills < 0 || skills > equipped || skillListLevel(frames[skills], zaricheSkill) != 1 {
		t.Fatal("no SkillList with the stage 1 skill between the karma change and the equip")
	}
	if attention := indexOfSystemMessage(frames, 0, serverpackets.SystemMessageAttentionS1PickedUpS2); attention >= 0 {
		t.Fatal("a cursed weapon pickup broadcast the attention line")
	}
	if ui := indexOf(frames, equipped, serverpackets.OpcodeUserInfo, -1); ui < 0 || userInfoStage(frames[ui]) != 1 {
		t.Fatal("no UserInfo showing stage 1 after the equip")
	}

	holder := victimCharacter(t, srv, id)
	if holder.CursedWeaponID() != zaricheID || holder.Karma() != 9999999 || holder.ProgressionValues().PKKills != 0 {
		t.Fatalf("holder = weapon %d karma %d pk %d, want Zariche, 9999999, 0", holder.CursedWeaponID(), holder.Karma(), holder.ProgressionValues().PKKills)
	}
	if inst := holder.Inventory().ItemByTemplateID(zaricheID); inst == nil || !inst.Equipped() {
		t.Fatal("Zariche not worn")
	}
	if res := holder.ResourceValues(); res.CurrentHP != res.MaxHP || res.CurrentMP != res.MaxMP || res.CurrentCP != res.MaxCP {
		t.Fatalf("vitals = %+v, want full", res)
	}
	playerID, oldKarma, oldPK, stage, next, hungry, ok := cursedRow(t, srv)
	if !ok || playerID != id || oldKarma != 100 || oldPK != 3 || stage != 1 || next != 5 || hungry != 24*60 {
		t.Fatalf("cursed_weapons row = %v %d/%d/%d stage %d next %d hungry %d, want the holder's 100/3, stage 1, next 5, hunger 1440",
			ok, playerID, oldKarma, oldPK, stage, next, hungry)
	}
}

// TestCursedWeaponSecondWeaponIsAssimilated: a holder obtaining a second
// cursed weapon cannot hold both. The held one goes up a stage, with the
// next skill level and the stage-up animation; the new one leaves the
// inventory and ends, S1_HAS_DISAPPEARED naming it.
func TestCursedWeaponSecondWeaponIsAssimilated(t *testing.T) {
	t.Parallel()
	srv := cursedBoot(t, 10, gameservertest.WithSeed(seedDropVictim(25, 100, 3)))
	c, id := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	pickUpCursed(t, srv, c, id)

	frames := pickUpWeapon(t, srv, c, id, akamanahID)

	social := indexOf(frames, 0, serverpackets.OpcodeSocialAction, id)
	gone := indexOfMessage(frames, 0, serverpackets.SystemMessageS1HasDisappeared, itemParam(akamanahID))
	if social < 0 || gone < social {
		t.Fatalf("stage-up animation at %d, S1_HAS_DISAPPEARED at %d: want both, in that order", social, gone)
	}
	if level := skillListLevel(lastSkillList(t, frames), zaricheSkill); level != 2 {
		t.Fatalf("Zariche's skill after the assimilation = %d, want 2", level)
	}
	holder := victimCharacter(t, srv, id)
	if holder.CursedWeaponID() != zaricheID || holder.CursedWeaponStage() != 2 || holder.Inventory().ItemByTemplateID(akamanahID) != nil {
		t.Fatalf("holder keeps %d at stage %d, Akamanah held %v; want Zariche at stage 2 alone",
			holder.CursedWeaponID(), holder.CursedWeaponStage(), holder.Inventory().ItemByTemplateID(akamanahID) != nil)
	}
	if active := srv.CursedWeapons.Active(); len(active) != 1 || active[0].ItemID != zaricheID {
		t.Fatalf("weapons out = %+v, want Zariche alone", active)
	}
}

// TestCursedWeaponHolderCannotLetGo: the holder cannot drop the weapon
// (CANNOT_DISCARD_THIS_ITEM), take it off, toggle it off, or put another
// weapon on; each refusal releases the client and changes nothing.
func TestCursedWeaponHolderCannotLetGo(t *testing.T) {
	t.Parallel()
	srv := cursedBoot(t, 10, gameservertest.WithSeed(seedDropVictim(25, 100, 3)))
	c, id := srv.Client, srv.SoleObjectID(t)
	sword := srv.GiveItem(t, id, dropSwordID, 1)
	startInWorld(t, c)
	pickUpCursed(t, srv, c, id)
	holder := victimCharacter(t, srv, id)
	weapon := holder.Inventory().ItemByTemplateID(zaricheID).ObjectID
	x, y, z := srv.PlayerPosition(t, id)

	c.Send(encodeRequestDropItem(weapon, 1, x, y, z))
	if f := mustRead(t, c, "drop refusal"); indexOfSystemMessage([][]byte{f}, 0, serverpackets.SystemMessageCannotDiscardThisItem) != 0 {
		t.Fatalf("drop answered %#x, want CANNOT_DISCARD_THIS_ITEM", f[0])
	}
	for name, packet := range map[string][]byte{
		"unequip both hands": encodeRequestUnEquipItem(lrHandSlot),
		"use the weapon":     encodeUseItem(weapon, false),
		"use another sword":  encodeUseItem(sword, false),
	} {
		c.Send(packet)
		if frames := readQuiet(c); len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("%s answered %d frames, want ActionFailed alone", name, len(frames))
		}
	}
	inv := holder.Inventory()
	if inst := inv.ItemByObjectID(weapon); inst == nil || !inst.Equipped() {
		t.Fatal("Zariche left the hands")
	}
	if inst := inv.ItemByObjectID(sword); inst == nil || inst.Equipped() {
		t.Fatal("the other sword went on")
	}
}

// TestCursedWeaponKillFeedsWeapon: a holder killing a player gains a PK
// kill (and no karma or PvP point), and with the stage reached its weapon
// goes up a stage: the next skill level and the stage-up animation. The
// victim's death costs it nothing: a PK victim of a holder drops no item.
func TestCursedWeaponKillFeedsWeapon(t *testing.T) {
	t.Parallel()
	srv := cursedBoot(t, 2,
		gameservertest.WithSeed(seedDropVictim(25, 100, 3)),
		gameservertest.WithDeathDrop(playerDeathDropEverything()),
	)
	c, victimID := srv.Client, srv.SoleObjectID(t)
	srv.GiveItem(t, victimID, cursedPotionID, 5)
	startInWorld(t, c)

	killerChar := srv.SeedCharacterFor(t, "killer", "Killer", 30, 0)
	seedKnownSkill(t, srv, killerChar.ID, 42, 1)
	killer := srv.DialClient(t, "killer", 1)
	startInWorld(t, killer)
	setVictimRoll(t, srv, killerChar.ID, 0)
	pickUpCursed(t, srv, killer, killerChar.ID)
	drainUntilQuiet(t, c)

	killPrimaryClient(t, srv, killer, killerChar.ID, victimID)

	frames := readQuiet(killer)
	ui := indexOf(frames, 0, serverpackets.OpcodeUserInfo, -1)
	social := indexOf(frames, 0, serverpackets.OpcodeSocialAction, killerChar.ID)
	if ui < 0 || social < 0 || ui > social {
		t.Fatalf("UserInfo at %d, SocialAction at %d: want the kill's UserInfo, then the stage-up animation", ui, social)
	}
	if got := wire.NewReader(frames[social][5:]).ReadInt32(); got != 17 {
		t.Fatalf("stage-up animation = %d, want 17", got)
	}
	if level := skillListLevel(lastSkillList(t, frames), zaricheSkill); level != 2 {
		t.Fatalf("skill level after the stage-up = %d, want 2", level)
	}
	holder := victimCharacter(t, srv, killerChar.ID)
	if p := holder.ProgressionValues(); p.PKKills != 1 || holder.Karma() != 9999999 || p.PvPKills != 0 {
		t.Fatalf("holder pk %d karma %d pvp %d, want 1, 9999999, 0", p.PKKills, holder.Karma(), p.PvPKills)
	}
	if holder.CursedWeaponStage() != 2 || srv.CursedWeapons.Stage(zaricheID) != 2 {
		t.Fatalf("stage = %d (weapon %d), want 2", holder.CursedWeaponStage(), srv.CursedWeapons.Stage(zaricheID))
	}
	if got := droppedNames(readQuiet(c)); len(got) != 0 {
		t.Fatalf("the holder's victim dropped %v, want nothing", got)
	}
}

// playerDeathDropEverything drops every droppable item of any death.
func playerDeathDropEverything() player.DeathDropRules {
	return player.DeathDropRules{Karma: everyDeathDrop, Monster: everyDeathDrop}
}

// TestCursedWeaponHolderDeath: a holder's death ends the weapon when its
// roll in a hundred is at most the disappear chance, and drops it next to
// the corpse otherwise. Either way the holder gets its karma and PK kills
// back and loses the skill, the row goes, and nothing else drops.
func TestCursedWeaponHolderDeath(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		roll    int
		dropped bool
	}{
		{name: "drops", roll: 51, dropped: true},
		{name: "disappears", roll: 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := cursedBoot(t, 10,
				gameservertest.WithSeed(seedDropVictim(25, 100, 3)),
				gameservertest.WithDeathDrop(playerDeathDropEverything()),
			)
			c, id := srv.Client, srv.SoleObjectID(t)
			srv.GiveItem(t, id, cursedPotionID, 5)
			startInWorld(t, c)
			pickUpCursed(t, srv, c, id)
			setVictimRoll(t, srv, id, tc.roll)
			x, y, z := srv.PlayerPosition(t, id)

			killByNPC(t, srv, id)

			frames := readQuiet(c)
			holder := victimCharacter(t, srv, id)
			if holder.CursedWeaponEquipped() || holder.Karma() != 100 || holder.ProgressionValues().PKKills != 3 {
				t.Fatalf("after death: weapon %d karma %d pk %d, want none, 100, 3", holder.CursedWeaponID(), holder.Karma(), holder.ProgressionValues().PKKills)
			}
			if level := skillListLevel(lastSkillList(t, frames), zaricheSkill); level != 0 {
				t.Fatalf("skill still listed at level %d", level)
			}
			if holder.Inventory().ItemByTemplateID(zaricheID) != nil {
				t.Fatal("Zariche still held")
			}
			if holder.Inventory().ItemByTemplateID(cursedPotionID) == nil {
				t.Fatal("the holder's death dropped its potions")
			}
			if _, _, _, _, _, _, ok := cursedRow(t, srv); ok {
				t.Fatal("cursed_weapons row kept")
			}
			if tc.dropped {
				if got := droppedNames(frames); !slices.Equal(got, []int32{zaricheID}) {
					t.Fatalf("YOU_DROPPED_S1 named %v, want Zariche alone", got)
				}
				if indexOfMessage(frames, 0, serverpackets.SystemMessageS2WasDroppedInTheS1Region, zoneParam(x, y, z), itemParam(zaricheID)) < 0 {
					t.Fatal("no S2_WAS_DROPPED_IN_THE_S1_REGION for the holder's region")
				}
				ground := cursedGround(t, srv)
				if gx, gy, _ := ground.Position(); gx-x < -25 || gx-x > 25 || gy-y < -25 || gy-y > 25 || !ground.DestroyProtected() {
					t.Fatalf("Zariche lies at (%d, %d) protected %v, want within 25 of (%d, %d), protected", gx, gy, ground.DestroyProtected(), x, y)
				}
				active := srv.CursedWeapons.Active()
				if len(active) != 1 || active[0].Activated || srv.CursedWeapons.Stage(zaricheID) != 1 {
					t.Fatalf("weapons out = %+v, want Zariche lying at stage 1", active)
				}
				return
			}
			if indexOfMessage(frames, 0, serverpackets.SystemMessageS1HasDisappeared, itemParam(zaricheID)) < 0 {
				t.Fatal("no S1_HAS_DISAPPEARED")
			}
			if len(srv.CursedWeapons.Active()) != 0 {
				t.Fatal("Zariche still out")
			}
		})
	}
}

// seedHolder stores id as Zariche's holder since before the boot: the
// weapon in its inventory, its karma at 9999999, and the row at stage 2
// with hungry minutes of hunger and the life ending at end.
func seedHolder(t *testing.T, srv *gameservertest.Server, id int32, hungry int32, end time.Time) {
	t.Helper()
	ctx := context.Background()
	srv.GiveItem(t, id, zaricheID, 1)
	if _, err := srv.DB.ExecContext(ctx, "UPDATE characters SET karma = 9999999, pkkills = 0 WHERE obj_Id = ?", id); err != nil {
		t.Fatalf("seed holder karma: %v", err)
	}
	if _, err := srv.DB.ExecContext(ctx,
		"INSERT INTO cursed_weapons (itemId, playerId, playerKarma, playerPkKills, nbKills, currentStage, numberBeforeNextStage, hungryTime, endTime) VALUES (?, ?, 100, 3, 0, 2, 5, ?, ?)",
		zaricheID, id, hungry, end.UnixMilli()); err != nil {
		t.Fatalf("seed cursed_weapons: %v", err)
	}
	if err := srv.CursedWeapons.Restore(ctx); err != nil {
		t.Fatalf("restore cursed weapons: %v", err)
	}
}

// cursedClock is a settable clock for the cursed weapons' lifecycle.
type cursedClock struct{ now atomic.Int64 }

func newCursedClock(at time.Time) *cursedClock {
	c := &cursedClock{}
	c.now.Store(at.UnixNano())
	return c
}

func (c *cursedClock) Now() time.Time { return time.Unix(0, c.now.Load()) }

// TestCursedWeaponHolderReturnsAndStarves: a holder stored with 50 hours
// left logs in holding its stage 2 skill and stage, and its login burst
// announces it, after the shortcuts and ahead of the reuse timers:
// S2_OWNER_HAS_LOGGED_INTO_THE_S1_REGION for everyone, then its time left
// in hours. The location request finds it. Each hour of hunger reminds it
// of its time left; when the hunger runs out the weapon ends: karma and PK
// kills come back, the skill and the weapon go, S1_HAS_DISAPPEARED is
// heard, and the stored state follows.
func TestCursedWeaponHolderReturnsAndStarves(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	clock := newCursedClock(base)
	srv := cursedBoot(t, 10,
		gameservertest.WithSeed(seedDropVictim(25, 100, 3)),
		gameservertest.WithCursedWeaponClock(clock.Now),
	)
	c, id := srv.Client, srv.SoleObjectID(t)
	seedHolder(t, srv, id, 61, base.Add(50*time.Hour))

	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, mustRead(t, c, "SSQInfo"), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, mustRead(t, c, "CharSelected"), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(encodeEnterWorld())
	var burst [][]byte
	for {
		f := mustRead(t, c, "login burst")
		burst = append(burst, f)
		if f[0] == serverpackets.OpcodeSkillCoolTime {
			break
		}
	}
	drainUntilQuiet(t, c)
	x, y, z := srv.PlayerPosition(t, id)
	skills := indexOf(burst, 0, serverpackets.OpcodeSkillList, -1)
	ui := indexOf(burst, 0, serverpackets.OpcodeUserInfo, -1)
	shortcuts := indexOf(burst, 0, serverpackets.OpcodeShortCutInit, -1)
	if skills < 0 || skillListLevel(burst[skills], zaricheSkill) != 2 || ui < 0 || userInfoStage(burst[ui]) != 2 {
		t.Fatal("login SkillList/UserInfo do not carry stage 2")
	}
	loggedIn := indexOfMessage(burst, shortcuts, serverpackets.SystemMessageS2OwnerLoggedIntoS1Region, zoneParam(x, y, z), itemParam(zaricheID))
	left := indexOfMessage(burst, loggedIn, serverpackets.SystemMessageS2HoursOfUsageTimeLeftForS1, itemParam(zaricheID), numberParam(50))
	if loggedIn < 0 || left != loggedIn+1 || left != len(burst)-2 {
		t.Fatalf("logged-in notice at %d, time left at %d of %d: want both after ShortCutInit, right ahead of SkillCoolTime", loggedIn, left, len(burst))
	}

	c.Send(encodeRequestCursedWeaponLocation())
	frame := mustRead(t, c, "ExCursedWeaponLocation")
	r := wire.NewReader(frame[3:])
	if n, itemID, active, lx, ly, lz := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); n != 1 || itemID != zaricheID || active != 1 || lx != int32(x) || ly != int32(y) || lz != int32(z) {
		t.Fatalf("locations = %d: %d active %d at (%d, %d, %d), want Zariche held at the holder", n, itemID, active, lx, ly, lz)
	}

	srv.TickCursedWeapons(base.Add(60 * time.Minute))
	srv.Settle(t)
	frames := readQuiet(c)
	if indexOfMessage(frames, 0, serverpackets.SystemMessageS2HoursOfUsageTimeLeftForS1, itemParam(zaricheID), numberParam(49)) < 0 {
		t.Fatal("the hour of hunger sent no reminder of 49 hours left")
	}

	srv.TickCursedWeapons(base.Add(61 * time.Minute))
	srv.Settle(t)
	frames = readQuiet(c)
	if indexOfMessage(frames, 0, serverpackets.SystemMessageS1HasDisappeared, itemParam(zaricheID)) < 0 {
		t.Fatal("no S1_HAS_DISAPPEARED when the hunger ran out")
	}
	holder := victimCharacter(t, srv, id)
	if holder.CursedWeaponEquipped() || holder.Karma() != 100 || holder.ProgressionValues().PKKills != 3 || holder.SkillLevel(int(zaricheSkill)) != 0 {
		t.Fatalf("after the end: weapon %d karma %d pk %d skill %d, want none, 100, 3, none",
			holder.CursedWeaponID(), holder.Karma(), holder.ProgressionValues().PKKills, holder.SkillLevel(int(zaricheSkill)))
	}
	if holder.Inventory().ItemByTemplateID(zaricheID) != nil {
		t.Fatal("Zariche still held")
	}
	if _, _, _, _, _, _, ok := cursedRow(t, srv); ok {
		t.Fatal("cursed_weapons row kept")
	}
	var karma, pk int32
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT karma, pkkills FROM characters WHERE obj_Id = ?", id).Scan(&karma, &pk); err != nil {
		t.Fatalf("read character: %v", err)
	}
	if karma != 100 || pk != 3 {
		t.Fatalf("stored karma %d pk %d, want 100, 3", karma, pk)
	}
}

// TestCursedWeaponExpiredWhileOffline: a holder's weapon whose life ran
// out while the server was down ends at boot. The holder is not online,
// so its stored karma and PK kills come back and the weapon leaves its
// stored items.
func TestCursedWeaponExpiredWhileOffline(t *testing.T) {
	t.Parallel()
	srv := cursedBoot(t, 10, gameservertest.WithSeed(seedDropVictim(25, 100, 3)))
	id := srv.SoleObjectID(t)
	seedHolder(t, srv, id, 600, time.Now().Add(-time.Minute))
	srv.FlushPersistence(t)

	if _, _, _, _, _, _, ok := cursedRow(t, srv); ok {
		t.Fatal("cursed_weapons row kept")
	}
	var karma, pk, weapons int32
	ctx := context.Background()
	if err := srv.DB.QueryRowContext(ctx, "SELECT karma, pkkills FROM characters WHERE obj_Id = ?", id).Scan(&karma, &pk); err != nil {
		t.Fatalf("read character: %v", err)
	}
	if err := srv.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM items WHERE owner_id = ? AND item_id = ?", id, zaricheID).Scan(&weapons); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if karma != 100 || pk != 3 || weapons != 0 {
		t.Fatalf("stored karma %d pk %d weapons %d, want 100, 3, 0", karma, pk, weapons)
	}
	if len(srv.CursedWeapons.Active()) != 0 {
		t.Fatal("Zariche still out")
	}
}
