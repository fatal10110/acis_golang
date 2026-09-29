package pets

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// riderSwordID is the fixture right-hand sword; riderShieldID and
	// striderFoodID are the shield and strider food this suite adds to the
	// fixture catalog, after the shipped Leather Shield and Food For Strider
	// (aCis_datapack/data/xml/items/0000-0099.xml, 5100-5199.xml).
	riderSwordID  = int32(30)
	riderShieldID = int32(18)
	striderFoodID = int32(5168)
	// striderFeedSkill is the strider food's feed skill (PetFoods.java:
	// case 5168 -> useFood(2101)); the fixture wyvern does not eat it.
	striderFeedSkill = 2101
	// wyvernBreathSkill is FrequentSkill.WYVERN_BREATH (SkillTable.java:
	// WYVERN_BREATH(4289, 1)).
	wyvernBreathSkill = 4289
	// riderToggleSkill is a toggle the rider holds when it mounts.
	riderToggleSkill = 288
)

// wyvernRiderItems is the fixture catalog plus a shield and strider food.
func wyvernRiderItems() *item.Table {
	return item.NewTable(append(gameservertest.ItemTemplates().All(),
		&item.Template{
			ID: riderShieldID, Name: "Leather Shield", Kind: item.KindArmor, Slot: item.SlotLHand, Duration: -1,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true,
			Armor: &item.ArmorDetail{Type: item.ArmorShield},
		},
		&item.Template{
			ID: striderFoodID, Name: "Food For Strider", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
			Dropable: true, Tradable: true, Destroyable: true,
			EtcItem:        &item.EtcItemDetail{Handler: "PetFoods"},
			AttachedSkills: []item.SkillRef{{ID: striderFeedSkill, Level: 1}},
		},
	))
}

// wyvernRiderSkills is petSkillTable plus the rider's toggle, Wyvern Breath
// and the strider food's feed skill.
func wyvernRiderSkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{ID: wolfFeedSkill, Level: 1, Feed: wolfFeedAmount},
		{ID: striderFeedSkill, Level: 1, Feed: wolfFeedAmount},
		{
			ID: riderToggleSkill, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf,
			SkillType: "BUFF", Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
		},
		{
			ID: wyvernBreathSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetArea,
			SkillType: "MDAM", HitTime: 3600, ReuseDelay: 6000, MPConsume: 400, CastRange: 700,
		},
	}), gamesql.NewCharacterSkillStore(db))
}

// bootWyvernOwner boots the owner, not yet mounted, with the wyvern collar,
// the fed wyvern, the rider skill table, the toggle known and seeds in its
// inventory.
func bootWyvernOwner(t *testing.T, seeds ...seedItem) *petWorld {
	t.Helper()
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate(), fedWyvernTemplate()})),
		gameservertest.WithRestartPoints(mountRestartTable()),
		gameservertest.WithSkills(wyvernRiderSkills(t)),
		gameservertest.WithItemTemplates(wyvernRiderItems()),
	)
	ownerID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, riderToggleSkill, 1); err != nil {
		t.Fatalf("seed toggle skill: %v", err)
	}
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	for _, s := range append([]seedItem{{TemplateID: wyvernCollarID, Count: 1}}, seeds...) {
		id := srv.GiveItem(t, ownerID, s.TemplateID, s.Count)
		h.seeded[s.TemplateID] = append(h.seeded[s.TemplateID], id)
	}
	startInWorld(t, h.client)
	return h
}

// character is the owner's live character.
func (h *petWorld) character(t *testing.T) *player.Character {
	t.Helper()
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("world player %T is not an online character", obj)
	}
	return c
}

// skillListIDs decodes a SkillList frame into its skill ids.
func skillListIDs(t *testing.T, frame []byte) map[int32]bool {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSkillList, "SkillList")
	r := wire.NewReader(frame[1:])
	ids := map[int32]bool{}
	for range r.ReadInt32() {
		r.ReadInt32() // passive
		r.ReadInt32() // level
		ids[r.ReadInt32()] = true
		r.ReadUint8() // disabled
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SkillList: %v", err)
	}
	return ids
}

// paperdollRows reports which of templateIDs the owner's persisted rows
// still hold on the paperdoll.
func (h *petWorld) paperdollRows(t *testing.T, templateIDs ...int32) map[int32]bool {
	t.Helper()
	h.srv.FlushItems(t)
	rows, err := h.srv.Items.ListByOwner(petCtx(), h.ownerID)
	if err != nil {
		t.Fatalf("list owner items: %v", err)
	}
	worn := map[int32]bool{}
	for _, row := range rows {
		for _, id := range templateIDs {
			if row.TemplateID == id && row.Location == item.LocationPaperdoll {
				worn[id] = true
			}
		}
	}
	return worn
}

// TestWyvernMountDisarmsAndGrantsWyvernBreath pins Player.mount(int, int)
// for the wyvern collar: disarmWeapon(true) stops the attack, takes the
// weapon then the shield off naming each with S1_DISARMED and refreshes
// UserInfo/CharInfo; forceRunStance switches a walking rider to run
// (ChangeMoveType, UserInfo); stopAllToggles ends the rider's toggles;
// setMount grants Wyvern Breath and sends the SkillList; then the Ride goes
// to everyone around, broadcastUserInfo sends UserInfo to the rider and
// CharInfo to the observer, and startFeed shows the gauge.
func TestWyvernMountDisarmsAndGrantsWyvernBreath(t *testing.T) {
	t.Parallel()
	h := bootWyvernOwner(t, seedItem{TemplateID: riderSwordID, Count: 1}, seedItem{TemplateID: riderShieldID, Count: 1})
	observer := h.joinSecondPlayer(t, "Watcher")

	h.client.Send(encodeUseItem(h.seededItem(t, riderSwordID), false))
	h.client.Send(encodeUseItem(h.seededItem(t, riderShieldID), false))
	h.client.Send(encodeRequestChangeMoveType(false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeChangeMoveType, "walk ChangeMoveType")
	h.client.Send(encodeRequestMagicSkillUse(riderToggleSkill))
	readUntilOpcode(t, h.client, serverpackets.OpcodeAbnormalStatusUpdate, "toggle icon")
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, observer.client)
	if worn := h.paperdollRows(t, riderSwordID, riderShieldID); !worn[riderSwordID] || !worn[riderShieldID] {
		t.Fatalf("worn before the mount = %v, want sword and shield", worn)
	}
	rider := h.character(t)
	if rider.Running() {
		t.Fatal("owner still running after the walk toggle")
	}
	if _, ok := rider.EffectList().ActiveBySkillID(riderToggleSkill); !ok {
		t.Fatal("toggle not active before the mount")
	}

	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeSetupGauge, "feed gauge")

	var seq []byte
	var messages [][]byte
	var skillList []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSystemMessage:
			messages = append(messages, f)
		case serverpackets.OpcodeSkillList:
			skillList = f
		case serverpackets.OpcodeUserInfo, serverpackets.OpcodeChangeMoveType, serverpackets.OpcodeRide, serverpackets.OpcodeSetupGauge:
		default:
			continue
		}
		seq = append(seq, f[0])
	}
	want := []byte{
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeChangeMoveType, serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeSkillList, serverpackets.OpcodeRide, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSetupGauge,
	}
	if string(seq) != string(want) {
		t.Fatalf("mount frames = % x, want % x", seq, want)
	}
	assertSystemMessageItem(t, messages[0], serverpackets.SystemMessageS1Disarmed, riderSwordID)
	assertSystemMessageItem(t, messages[1], serverpackets.SystemMessageS1Disarmed, riderShieldID)
	assertSystemMessageID(t, messages[2], serverpackets.SystemMessageS1HasBeenAborted)
	if ids := skillListIDs(t, skillList); !ids[wyvernBreathSkill] || !ids[riderToggleSkill] {
		t.Fatalf("mount SkillList ids = %v, want Wyvern Breath and the toggle", ids)
	}

	if worn := h.paperdollRows(t, riderSwordID, riderShieldID); len(worn) != 0 {
		t.Fatalf("worn after the mount = %v, want neither", worn)
	}
	if !rider.Running() || !rider.Flying() {
		t.Fatalf("rider after the mount: running %v flying %v, want both", rider.Running(), rider.Flying())
	}
	if _, ok := rider.EffectList().ActiveBySkillID(riderToggleSkill); ok {
		t.Fatal("toggle still active after the mount")
	}
	if got := rider.SkillLevel(wyvernBreathSkill); got != 1 {
		t.Fatalf("Wyvern Breath level = %d, want 1", got)
	}

	seen := readUntilOpcode(t, observer.client, serverpackets.OpcodeRide, "observer Ride")
	charInfos := 0
	for _, f := range seen {
		if f[0] == serverpackets.OpcodeCharInfo {
			charInfos++
		}
	}
	if charInfos == 0 {
		t.Fatal("observer saw no CharInfo for the disarm before the Ride")
	}
	readUntilOpcode(t, observer.client, serverpackets.OpcodeCharInfo, "observer CharInfo after the Ride")
}

// TestWyvernDismountRemovesWyvernBreath pins setMount(0, 0, 0) on a
// flying rider: the dismount's SkillList no longer lists Wyvern Breath.
func TestWyvernDismountRemovesWyvernBreath(t *testing.T) {
	t.Parallel()
	h := bootWyvernOwner(t)
	if !h.srv.DrivesClock() {
		t.Skip("starving the mount needs the driven clock")
	}
	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeSetupGauge, "feed gauge")
	drainFrames(t, h.client)

	// 508 - 10*50 = 8 is no more than one meal: tick 51 starves.
	h.advanceTicks(t, 50)
	frames := h.advanceTicks(t, 1)
	var skillList []byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSkillList {
			skillList = f
			break
		}
	}
	if skillList == nil {
		t.Fatalf("starving tick sent no SkillList: opcodes % x", frameOpcodes(frames))
	}
	if ids := skillListIDs(t, skillList); ids[wyvernBreathSkill] || !ids[riderToggleSkill] {
		t.Fatalf("dismount SkillList ids = %v, want the toggle without Wyvern Breath", ids)
	}
	if got := h.character(t).SkillLevel(wyvernBreathSkill); got != 0 {
		t.Fatalf("Wyvern Breath level after the dismount = %d, want 0", got)
	}
}

// TestRiderFeedsWyvernByHand pins PetFoods.useFood's player branch: a
// rider using food its mount eats uses up one unit, the feed skill plays
// for everyone around, and the gauge rises by the skill's feed value, with
// no hungry message.
func TestRiderFeedsWyvernByHand(t *testing.T) {
	t.Parallel()
	h := bootWyvernOwner(t, seedItem{TemplateID: wolfFoodID, Count: 2})
	if !h.srv.DrivesClock() {
		t.Skip("draining the gauge needs the driven clock")
	}
	observer := h.joinSecondPlayer(t, "Watcher")
	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeSetupGauge, "feed gauge")
	drainFrames(t, h.client)
	wantGauges(t, "first period", feedGauges(t, h.advanceTicks(t, 1)), gaugeAt(wyvernMaxMeal-wyvernRideMealInNormal))
	drainUntilQuiet(t, observer.client)

	h.client.Send(encodeUseItem(h.seededItem(t, wolfFoodID), false))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeSetupGauge, "fed gauge")
	var seq []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSetupGauge, serverpackets.OpcodeSystemMessage:
			seq = append(seq, f[0])
		}
	}
	if want := []byte{serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSetupGauge}; string(seq) != string(want) {
		t.Fatalf("hand-feed frames = % x, want % x", seq, want)
	}
	r := wire.NewReader(frames[len(frames)-2][1:])
	if caster, target, skill, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != h.ownerID || target != h.ownerID || skill != wolfFeedSkill || level != 1 {
		t.Fatalf("feed MagicSkillUse = %d->%d skill %d-%d, want %d->%d skill %d-1", caster, target, skill, level, h.ownerID, h.ownerID, wolfFeedSkill)
	}
	wantGauges(t, "hand feed", feedGauges(t, frames), fullGauge())
	readUntilOpcode(t, observer.client, serverpackets.OpcodeMagicSkillUse, "observer feed MagicSkillUse")
	if got := h.ownerItemCount(t, wolfFoodID); got != 1 {
		t.Fatalf("wolf food left = %d, want 1", got)
	}
}

// TestPetFoodRejectedWithoutMatchingMount pins PetFoods.useFood's player
// branch refusal: a player on foot, or a rider whose mount does not eat the
// food, is told S1_CANNOT_BE_USED with the food's name and keeps the item.
func TestPetFoodRejectedWithoutMatchingMount(t *testing.T) {
	t.Parallel()
	h := bootWyvernOwner(t, seedItem{TemplateID: wolfFoodID, Count: 1}, seedItem{TemplateID: striderFoodID, Count: 1})

	h.client.Send(encodeUseItem(h.seededItem(t, wolfFoodID), false))
	assertSystemMessageItem(t, mustRead(t, h.client, "on-foot refusal"), serverpackets.SystemMessageS1CannotBeUsed, wolfFoodID)
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeSetupGauge, "feed gauge")
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeUseItem(h.seededItem(t, striderFoodID), false))
	assertSystemMessageItem(t, mustRead(t, h.client, "wrong-food refusal"), serverpackets.SystemMessageS1CannotBeUsed, striderFoodID)
	drainUntilQuiet(t, h.client)

	if got := h.ownerItemCount(t, wolfFoodID); got != 1 {
		t.Fatalf("wolf food left = %d, want 1", got)
	}
	if got := h.ownerItemCount(t, striderFoodID); got != 1 {
		t.Fatalf("strider food left = %d, want 1", got)
	}
}
