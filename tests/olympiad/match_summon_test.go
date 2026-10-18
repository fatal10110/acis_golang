package olympiad

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The summon scenarios' fixtures: a servitor, a wolf pet called by the
// test kit's wolf collar, a strider to ride, the test kit's sword, and a
// tunic barred in the Olympiad.
const (
	summonServitorID  = 12564
	summonWolfID      = 12500
	summonWolfCollar  = 9600
	summonStriderID   = 12526
	summonSwordID     = 30
	restrictedTunicID = 9700
)

// summonNPCs are the servitor and the wolf pet.
func summonNPCs() gameservertest.Option {
	stats := npc.PetLevelStats{MaxExp: 500, MaxHP: 400, MaxMP: 80, PAtk: 100, PDef: 90, MAtk: 20, MDef: 40, MaxMeal: 3000, MealInNormal: 5, MealInBattle: 10, SSCount: 1}
	return gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
		{
			ID: summonServitorID, TemplateID: summonServitorID, Type: "Servitor", Name: "Sin Eater", Level: 20,
			HPMax: 5000, MPMax: 100, PAtk: 50, AtkSpd: 300, BaseAttackRange: 40, RunSpeed: 120, WalkSpeed: 60,
			CollisionRadius: 8, CollisionHeight: 20,
		},
		{
			ID: summonWolfID, Name: "Wolf", Level: 10, STR: 40, CON: 43, DEX: 30, INT: 22, WIT: 20, MEN: 20,
			BaseAttackRange: 40, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
			Pet: &npc.PetData{AutoFeedLimit: 0.55, HungryLimit: 0.3, UnsummonLimit: 0.1, Levels: map[int]npc.PetLevelStats{10: stats}},
		},
	}))
}

// summonItems maps the wolf collar to the wolf.
func summonItems(t *testing.T) gameservertest.Option {
	t.Helper()
	items, err := item.NewSummonItemTable([]item.SummonItem{{ItemID: summonWolfCollar, NPCID: summonWolfID, SummonType: 1}})
	if err != nil {
		t.Fatalf("summon items: %v", err)
	}
	return gameservertest.WithSummonItems(items)
}

// summonItemTemplates are the test kit's items, the Noblesse Gate Pass and
// a tunic barred in the Olympiad.
func summonItemTemplates() gameservertest.Option {
	all := gameservertest.ItemTemplates().All()
	var tunic item.Template
	for _, tmpl := range all {
		if tmpl.ID == 40 {
			tunic = *tmpl
		}
	}
	tunic.ID, tunic.Name, tunic.OlyRestricted = restrictedTunicID, "Restricted Tunic", true
	return gameservertest.WithItemTemplates(item.NewTable(append(all, &tunic, &item.Template{
		ID: olympiad.NoblesseGatePass, Name: "Noblesse Gate Pass", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
		Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
	})))
}

// summonOf waits for p's summon to be in the world and returns it.
func (m *match) summonOf(t *testing.T, p *competitor) *summon.Actor {
	t.Helper()
	var s *summon.Actor
	m.srv.AdvanceUntil(t, p.name+"'s summon in the world", func() bool {
		obj, ok := m.srv.State.Summon(p.id)
		if ok {
			s, ok = obj.(*summon.Actor)
		}
		return ok
	})
	return s
}

// callServitor has p call the servitor and returns it.
func (m *match) callServitor(t *testing.T, p *competitor) *summon.Actor {
	t.Helper()
	pc := m.online(t, p)
	m.onQueue(t, pc, func() {
		pc.SummonServitor(modelskill.Definition{ID: 1111, Level: 1, SkillType: "SUMMON", NpcID: summonServitorID, SummonTotalLifeTime: 1_200_000})
	})
	s := m.summonOf(t, p)
	m.read(t)
	return s
}

// entered waits for p's client to switch to Olympiad mode on side, then
// reads what its strip sent, and returns where in p's frames the switch is.
func (m *match) entered(t *testing.T, p *competitor, side int) int {
	t.Helper()
	m.runUntil(t, p.name+" in the stadium", 5*time.Minute, func() bool { return modeAt(p.frames, side) >= 0 })
	m.srv.Advance(t, 50*time.Millisecond)
	m.read(t)
	return modeAt(p.frames, side)
}

// hasOpcode reports whether frames hold one with opcode.
func hasOpcode(frames [][]byte, opcode byte) bool {
	return slices.ContainsFunc(frames, func(f []byte) bool { return f[0] == opcode })
}

// petInfoBeforeSkillList reports whether frames hold a PetInfo from index
// from on, ahead of the first SkillList after it.
func petInfoBeforeSkillList(frames [][]byte, from int) bool {
	for _, f := range frames[from:] {
		switch f[0] {
		case serverpackets.OpcodePetInfo:
			return true
		case serverpackets.OpcodeSkillList:
			return false
		}
	}
	return false
}

// TestOlympiadMatchUnsummonsPetAndDismounts pins the strip's summon and
// mount branches (AbstractOlympiadGame.removals): Alpha's pet is sent back
// as it enters the stadium, and Bravo, riding a strider, gets off it.
func TestOlympiadMatchUnsummonsPetAndDismounts(t *testing.T) {
	t.Parallel()
	var collar int32
	m := bootMatchPrepared(t, nil, func(srv *gameservertest.Server) {
		collar = srv.GiveItem(t, srv.SoleObjectID(t), summonWolfCollar, 1)
	}, summonNPCs(), summonItems(t))
	a, b := m.ones[0], m.ones[1]
	alpha, bravo := m.online(t, a), m.online(t, b)
	m.onQueue(t, alpha, func() { alpha.SummonCreature(modelskill.Definition{}, alpha.Inventory().ItemByObjectID(collar)) })
	if pet := m.summonOf(t, a); !pet.IsPet() {
		t.Fatal("Alpha's collar called no pet")
	}
	m.onQueue(t, bravo, func() {
		if !bravo.Mount(summonStriderID, 1) {
			t.Error("Bravo could not mount the strider")
		}
	})
	m.read(t)
	for _, p := range m.ones {
		p.frames = nil
	}

	entry := m.entered(t, a, 1)
	if _, ok := m.srv.State.Summon(a.id); ok {
		t.Error("Alpha's pet is still out in the stadium")
	}
	if !hasOpcode(a.frames[entry:], serverpackets.OpcodePetDelete) {
		t.Error("Alpha's client was not told its pet went")
	}
	m.entered(t, b, 2)
	if m.online(t, b).Mounted() {
		t.Error("Bravo still rides its strider in the stadium")
	}
	if !hasOpcode(b.frames, serverpackets.OpcodeRide) {
		t.Error("Bravo's client was not told it got off its strider")
	}
}

// TestOlympiadMatchStripsServitorHeroGearAndRestores pins the strip's and
// the cleanup's branches for a hero with a servitor
// (AbstractOlympiadGame.removals, cleanEffects, playerStatusBack): on
// entry Alpha's servitor loses its effects, Alpha is shown its window, and
// the servitor stops; Alpha loses its hero skills, the cubic another
// player gave it, its barred tunic and its sword's soulshot charge. At the
// cleanup the servitor's and Alpha's effects and Alpha's charges go, and
// the hero skills come back in Alpha's next skill list.
func TestOlympiadMatchStripsServitorHeroGearAndRestores(t *testing.T) {
	t.Parallel()
	var sword, tunic int32
	opts := append(skillOptions(t), summonNPCs(), summonItemTemplates(),
		gameservertest.WithOlympiadSeed(func(db *sql.DB) {
			if _, err := db.Exec(`INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 0, 1, 1, 1 FROM characters WHERE char_name = 'Alpha'`); err != nil {
				t.Fatalf("seed Alpha's hero row: %v", err)
			}
		}))
	m := bootMatchPrepared(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 5_000 }, func(srv *gameservertest.Server) {
		id := srv.SoleObjectID(t)
		sword = srv.GiveItem(t, id, summonSwordID, 1)
		tunic = srv.GiveItem(t, id, restrictedTunicID, 1)
	}, opts...)
	a := m.ones[0]
	alpha := m.online(t, a)
	for _, id := range heroSkills {
		if got := m.srv.PlayerSkillLevel(t, a.id, int(id)); got != 1 {
			t.Fatalf("Alpha's hero skill %d before the match = %d, want 1", id, got)
		}
	}
	a.c.Send(encodeUseItem(sword))
	a.c.Send(encodeUseItem(tunic))
	m.read(t)
	servitor := m.callServitor(t, a)
	might, ok := shippedSkills.defs.Definition(modelskill.Ref{ID: stripMight, Level: 1})
	if !ok {
		t.Fatal("no Might 1 among the shipped skills")
	}
	inv := alpha.Inventory()
	m.onQueue(t, alpha, func() {
		skillhandler.LandEffects(alpha, servitor, might)
		alpha.AddOrRefreshCubic(cubic.Storm, true)
		alpha.SetChargedShot(item.ShotSoul, true)
		servitor.SetTarget(alpha)
	})
	if _, ok := servitor.EffectList().ActiveBySkillID(stripMight); !ok {
		t.Fatal("the servitor is not under Might before the match")
	}
	if !inv.ItemByObjectID(tunic).Equipped() || inv.ItemAt(itemcontainer.RHand) == nil {
		t.Fatal("Alpha does not wear its tunic and sword before the match")
	}
	for _, p := range m.ones {
		p.frames = nil
	}

	// Alpha's client holds its Appearing until the strip is done: landing
	// in the stadium would stop the servitor again.
	client := a.c
	a.c = nil
	m.runUntil(t, "Alpha stripped", 5*time.Minute, func() bool {
		a.frames = append(a.frames, m.srv.ReadQueued(t, client)...)
		entry := modeAt(a.frames, 1)
		return entry >= 0 && hasOpcode(a.frames[entry:], serverpackets.OpcodeSkillList)
	})
	if servitor.CurrentTarget() != nil {
		t.Error("the servitor kept its target as Alpha was stripped")
	}
	a.c = client
	a.c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	m.srv.Advance(t, 50*time.Millisecond)
	m.read(t)
	entry := modeAt(a.frames, 1)
	if _, ok := servitor.EffectList().ActiveBySkillID(stripMight); ok {
		t.Error("the servitor kept Might in the stadium")
	}
	if !petInfoBeforeSkillList(a.frames, entry) {
		t.Error("Alpha was not shown its servitor's window ahead of its stadium skill list")
	}
	for _, id := range heroSkills {
		if got := m.srv.PlayerSkillLevel(t, a.id, int(id)); got != 0 {
			t.Errorf("Alpha's hero skill %d in the stadium = %d, want none", id, got)
		}
	}
	lists := skillListsFrom(t, a.frames, entry)
	if len(lists) == 0 {
		t.Fatal("Alpha got no skill list as it entered the stadium")
	}
	for _, id := range heroSkills {
		if _, ok := lists[len(lists)-1][id]; ok {
			t.Errorf("Alpha's skill list in the stadium shows hero skill %d", id)
		}
	}
	if ids := alpha.CubicIDs(); len(ids) != 0 {
		t.Errorf("Alpha kept the cubics %v another player gave it", ids)
	}
	if inv.ItemByObjectID(tunic).Equipped() {
		t.Error("Alpha still wears its barred tunic in the stadium")
	}
	if sysMsgAt(a.frames, entry, serverpackets.SystemMessageItemCantBeEquippedForOlympiad) < 0 {
		t.Error("Alpha was not told its tunic cannot be worn in the Olympiad")
	}
	if w := inv.ItemAt(itemcontainer.RHand); w == nil || w.ChargedShot(item.ShotSoul) {
		t.Error("Alpha's sword is gone or still charged with a soulshot in the stadium")
	}

	m.fighting(t)
	m.onQueue(t, alpha, func() {
		skillhandler.LandEffects(alpha, servitor, might)
		alpha.IncreaseCharges(2, 7)
	})
	if m.srv.PlayerCharges(t, a.id) != 2 {
		t.Fatal("Alpha holds no charges during the fight")
	}
	m.over(t)
	if _, ok := servitor.EffectList().ActiveBySkillID(stripMight); ok {
		t.Error("the servitor kept the Might it got during the fight")
	}
	if got := m.srv.PlayerCharges(t, a.id); got != 0 {
		t.Errorf("Alpha's charges after the match = %d, want none", got)
	}
	back := modeAt(a.frames, 0)
	if back < 0 {
		t.Fatal("Alpha's client did not leave Olympiad mode")
	}
	lists = skillListsFrom(t, a.frames, back)
	if len(lists) == 0 {
		t.Fatal("Alpha got no skill list after the match")
	}
	for _, id := range heroSkills {
		if lists[0][id] != 1 {
			t.Errorf("Alpha's skill list after the match shows hero skill %d at %d, want 1", id, lists[0][id])
		}
	}
	if !petInfoBeforeSkillList(a.frames, back) {
		t.Error("Alpha was not shown its servitor's window ahead of its cleanup skill list")
	}
}

// TestOlympiadMatchWonOnServitorDamage pins a summon's hits counting
// toward its owner's match (Summon/Servitor damage hooks): Alpha's
// servitor deals the only damage, the fight runs out of time with both
// standing, and Alpha wins it. The fight's end stops the servitor.
func TestOlympiadMatchWonOnServitorDamage(t *testing.T) {
	t.Parallel()
	hardy := gameservertest.ClassTemplate()
	hardy.HPTable, hardy.CPTable = []float64{80_000}, []float64{80_000}
	m := bootMatch(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 10_000 }, summonNPCs(), gameservertest.WithClassTemplate(hardy))
	a, b := m.ones[0], m.ones[1]
	servitor := m.callServitor(t, a)

	m.shown(t)
	a.c.Send(encodeAttackRequest(b.id))
	m.read(t)
	a.c.Send(encodeServitorAttack())
	m.read(t)
	hit := func() bool {
		return slices.ContainsFunc(b.frames, func(f []byte) bool {
			return f[0] == serverpackets.OpcodeAttack && wire.NewReader(f[1:]).ReadInt32() == servitor.ObjectID()
		})
	}
	m.runEvery(t, "the servitor's hit", 50*time.Millisecond, 10*time.Second, hit)
	if got := servitor.Intent(); got != summon.IntentAttackTarget {
		t.Fatalf("the servitor after its hit: intent %v, want attack-target", got)
	}
	// Once the fight stopped, ahead of the return to town, the servitor
	// no longer attacks.
	m.runUntil(t, "the fight stopped", time.Minute, func() bool {
		return sysMsgAt(a.frames, 0, serverpackets.SystemMessageMovedToTownInS1Seconds) >= 0
	})
	if got := servitor.Intent(); got == summon.IntentAttackTarget || servitor.CurrentTarget() != nil {
		t.Errorf("once the fight stopped the servitor has intent %v, a target %v; want it stopped with none", got, servitor.CurrentTarget() != nil)
	}
	m.over(t)
	if m.srv.PlayerDead(t, b.id) {
		t.Fatal("Bravo died; the fight should end on time")
	}
	for _, f := range a.frames {
		if f[0] == serverpackets.OpcodeAttack && wire.NewReader(f[1:]).ReadInt32() == a.id {
			t.Fatal("Alpha attacked Bravo; only its servitor's hits should count")
		}
	}
	if got := matchFights(t, m); len(got) != 1 || got[0].winner != 1 {
		t.Fatalf("olympiad_fights = %+v, want one won by side 1", got)
	}
}
