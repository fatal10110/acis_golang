package pets

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Expected exp/SP below are oracle values from the reference reward path
// (Monster.calculateRewards, PlayerStatus.addExpAndSp) for a level-1 player
// killing a level-1 monster worth 5000 exp and 25 SP.
const (
	rewardMonsterExp = 5000
	rewardMonsterSp  = 25
)

// rewardMonsterTemplate is dropMonsterTemplate paying 5000 exp and 25 SP.
// Threat records each hit's full damage, so the scenarios deal explicit
// amounts; the monster's HP only has to fall between them, checked by
// spawnRewardMonster.
func rewardMonsterTemplate() *npc.Template {
	tmpl := dropMonsterTemplate()
	tmpl.HPMax = 2000
	tmpl.RewardExp, tmpl.RewardSp = rewardMonsterExp, rewardMonsterSp
	return tmpl
}

// spawnRewardMonsterAt spawns the reward monster, requiring its HP to
// survive 650 damage and fall to 1000.
func spawnRewardMonsterAt(t *testing.T, srv *gameservertest.Server) *npc.Hostile {
	t.Helper()
	monster := srv.SpawnHostileNPCTemplateAt(t, rewardMonsterTemplate(), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	if hp := monster.MaxHPValue(); hp <= 650 || hp > 1000 {
		t.Fatalf("reward monster max HP = %v, want within (650, 1000]", hp)
	}
	return monster
}

// rewardWolfTemplate is the fixture wolf with the level-81 growth row the
// pet reward gate reads, taking expType of every kill's exp.
func rewardWolfTemplate(expType int) *npc.Template {
	tmpl := wolfTemplate()
	levels := map[int]npc.PetLevelStats{81: wolfLevelStats(1 << 40)}
	for lvl, row := range tmpl.Pet.Levels {
		row.ExpType = expType
		levels[lvl] = row
	}
	tmpl.Pet.Levels = levels
	return tmpl
}

func bootRewardPet(t *testing.T, expType int, extra ...gameservertest.Option) (*petWorld, *summon.Actor) {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{rewardWolfTemplate(expType)})),
	}, extra...)
	h := bootOwnerWithCollarOpts(t, opts)
	pet, _ := h.spawnWolf(t)
	return h, pet
}

func (h *petWorld) spawnRewardMonster(t *testing.T) *npc.Hostile {
	t.Helper()
	monster := spawnRewardMonsterAt(t, h.srv)
	drainUntilQuiet(t, h.client)
	return monster
}

// onlinePlayer returns the world's registered player for objID as a
// combatant: the same network wrapper production attacks carry.
func onlinePlayer(t *testing.T, srv *gameservertest.Server, objID int32) attackable.Combatant {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d missing from world state", objID)
	}
	combatant, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("player %T is not a combatant", obj)
	}
	return combatant
}

// joinSecondPlayer brings a second level-1 player into the world.
func joinSecondPlayer(t *testing.T, srv *gameservertest.Server) (*testsupport.ScriptedClient, int32) {
	t.Helper()
	second := srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	c := srv.DialClient(t, "player2", 1)
	startInWorld(t, c)
	return c, second.ObjectID()
}

// earnedExpSp returns the payload of the one "earned exp and SP" message
// among frames.
func earnedExpSp(t *testing.T, frames [][]byte) (exp, sp int32) {
	t.Helper()
	frame := findSystemMessage(t, frames, serverpackets.SystemMessageYouEarnedS1ExpAndS2SP)
	if frame == nil {
		t.Fatal("no exp/SP gain message")
	}
	r := wire.NewReader(frame[5:])
	if params := r.ReadInt32(); params != 2 {
		t.Fatalf("gain message params = %d, want 2", params)
	}
	r.ReadInt32()
	exp = r.ReadInt32()
	r.ReadInt32()
	return exp, r.ReadInt32()
}

func assertDropOwner(t *testing.T, srv *gameservertest.Server, want int32) {
	t.Helper()
	drops := srv.GroundItems.Snapshots(func(int32) bool { return false })
	if len(drops) != 1 || drops[0].OwnerID != want {
		t.Fatalf("ground drops = %+v, want one stack protected to %d", drops, want)
	}
}

// TestPetKillSplitsExpWithOwner pays a pet kill to its owner, split with
// the pet by expType: a fixed ratio, or the pet's share of the owner's
// combined damage.
func TestPetKillSplitsExpWithOwner(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name              string
		expType           int
		ownerDamage       int
		ownerExp, ownerSp int32
		petExp            int64
		petSp             int
	}{
		// Pet-only kill, half kept by the pet: round(5000*0.5), round(25*0.5).
		{"fixed ratio", 50, 0, 2500, 12, 2500, 13},
		// Owner 250 + pet 750: the pet takes 750/1000 of 5000/25, truncated.
		{"damage share", -1, 250, 1250, 7, 3750, 18},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, pet := bootRewardPet(t, tc.expType)
			monster := h.spawnRewardMonster(t)
			petExp, petSp := pet.Exp(), pet.SP()

			if tc.ownerDamage > 0 && monster.TakeDamage(tc.ownerDamage, onlinePlayer(t, h.srv, h.ownerID)) {
				t.Fatal("owner's hit killed the monster")
			}
			if !monster.TakeDamage(1000-tc.ownerDamage, pet) {
				t.Fatal("pet's hit did not kill the monster")
			}

			if exp, sp := earnedExpSp(t, drainFrames(t, h.client)); exp != tc.ownerExp || sp != tc.ownerSp {
				t.Fatalf("owner earned %d exp %d SP, want %d/%d", exp, sp, tc.ownerExp, tc.ownerSp)
			}
			if got, want := pet.Exp()-petExp, pet.ScaledExpGain(tc.petExp); got != want {
				t.Fatalf("pet exp gain = %d, want %d (raw %d)", got, want, tc.petExp)
			}
			if got := pet.SP() - petSp; got != tc.petSp {
				t.Fatalf("pet SP gain = %d, want %d", got, tc.petSp)
			}
		})
	}
}

// TestPetDamageMakesOwnerTopDealer credits the pet's damage to its owner's
// total: owner 250 + pet 350 beats another player's 400, so the drop is
// protected to the owner although each alone dealt less and the other
// player landed the kill.
func TestPetDamageMakesOwnerTopDealer(t *testing.T) {
	t.Parallel()
	h, pet := bootRewardPet(t, -1)
	_, secondID := joinSecondPlayer(t, h.srv)
	monster := h.spawnRewardMonster(t)

	monster.TakeDamage(250, onlinePlayer(t, h.srv, h.ownerID))
	monster.TakeDamage(350, pet)
	if !monster.TakeDamage(400, onlinePlayer(t, h.srv, secondID)) {
		t.Fatal("second player's hit did not kill the monster")
	}

	assertDropOwner(t, h.srv, h.ownerID)
}

// TestDepartedAttackerLeavesTheFight drops an attacker that left before the
// kill from the reward: the one who stayed earns the whole 5000/25 and the
// drop, however much the departed attacker dealt.
func TestDepartedAttackerLeavesTheFight(t *testing.T) {
	t.Parallel()
	t.Run("top dealer logged out", func(t *testing.T) {
		t.Parallel()
		h, _ := bootRewardPet(t, -1)
		second, secondID := joinSecondPlayer(t, h.srv)
		monster := h.spawnRewardMonster(t)

		monster.TakeDamage(600, onlinePlayer(t, h.srv, secondID))
		second.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
		if !second.AwaitClose(2 * time.Second) {
			t.Fatal("logout did not close the connection")
		}
		h.srv.AdvanceUntil(t, "second player left world", func() bool {
			_, ok := h.srv.State.Player(secondID)
			return !ok
		})
		if !monster.TakeDamage(400, onlinePlayer(t, h.srv, h.ownerID)) {
			t.Fatal("owner's hit did not kill the monster")
		}

		assertDropOwner(t, h.srv, h.ownerID)
		if exp, sp := earnedExpSp(t, drainFrames(t, h.client)); exp != rewardMonsterExp || sp != rewardMonsterSp {
			t.Fatalf("owner earned %d exp %d SP, want %d/%d", exp, sp, rewardMonsterExp, rewardMonsterSp)
		}
	})
	t.Run("top dealer's pet unsummoned", func(t *testing.T) {
		t.Parallel()
		h, pet := bootRewardPet(t, -1)
		second, secondID := joinSecondPlayer(t, h.srv)
		monster := h.spawnRewardMonster(t)
		drainUntilQuiet(t, second)

		monster.TakeDamage(600, pet)
		h.returnPet(t)
		if !monster.TakeDamage(400, onlinePlayer(t, h.srv, secondID)) {
			t.Fatal("second player's hit did not kill the monster")
		}

		assertDropOwner(t, h.srv, secondID)
		if exp, sp := earnedExpSp(t, drainFrames(t, second)); exp != rewardMonsterExp || sp != rewardMonsterSp {
			t.Fatalf("second player earned %d exp %d SP, want %d/%d", exp, sp, rewardMonsterExp, rewardMonsterSp)
		}
	})
}

// TestRewardRangeMeasuresTheAttackersBody counts an attacker only within
// the 1500 party range of the victim, body to body in 3D, measured from the
// attacker itself: a pet in range credits its owner even when the owner
// stands out of range.
func TestRewardRangeMeasuresTheAttackersBody(t *testing.T) {
	t.Parallel()
	const partyRange = 1500
	cases := []struct {
		name string
		// offset places the monster along +x from the owner; the pet
		// stands 40 units along +x from the owner.
		offset  func(reach float64) int
		byPet   bool
		counted bool
	}{
		{"owner at the edge", func(reach float64) int { return int(reach) }, false, true},
		{"owner one unit past the edge", func(reach float64) int { return int(reach) + 1 }, false, false},
		{"pet in range, owner out", func(reach float64) int { return int(reach) + 30 }, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, pet := bootRewardPet(t, 50)
			owner := onlinePlayer(t, h.srv, h.ownerID)
			ox, oy, oz := owner.Position()
			if px, py, pz := pet.Position(); px != ox+40 || py != oy || pz != oz {
				t.Fatalf("pet at (%d,%d,%d), want 40 along +x from the owner at (%d,%d,%d)", px, py, pz, ox, oy, oz)
			}
			tmpl := rewardMonsterTemplate()
			reach := float64(partyRange) + tmpl.CollisionRadius + owner.CollisionRadius()
			monster := h.srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: ox + tc.offset(reach), Y: oy, Z: oz})
			drainUntilQuiet(t, h.client)
			if !monster.Knows(owner) || !monster.Knows(pet) {
				t.Fatal("monster does not know the owner and pet")
			}

			var attacker attackable.Combatant = owner
			if tc.byPet {
				attacker = pet
			}
			if !monster.TakeDamage(1000, attacker) {
				t.Fatal("hit did not kill the monster")
			}

			gain := findSystemMessage(t, drainFrames(t, h.client), serverpackets.SystemMessageYouEarnedS1ExpAndS2SP)
			if !tc.counted {
				if gain != nil {
					t.Fatal("out-of-range attacker earned exp")
				}
				return
			}
			if gain == nil {
				t.Fatal("in-range attacker earned no exp")
			}
			// The expType-50 pet keeps half either way: round(5000*0.5), 25-13.
			if exp, sp := earnedExpSp(t, [][]byte{gain}); exp != 2500 || sp != 12 {
				t.Fatalf("owner earned %d exp %d SP, want 2500/12", exp, sp)
			}
		})
	}
}

// TestPetOverhitGrantsOwnerBonus pays a pet's overhit to its owner: damage
// past the monster's full HP adds the capped 25% before the pet takes its
// half.
func TestPetOverhitGrantsOwnerBonus(t *testing.T) {
	t.Parallel()
	h, pet := bootRewardPet(t, 50)
	monster := h.spawnRewardMonster(t)

	monster.EnableOverhit()
	if !monster.TakeDamage(int(monster.CurrentHP())*2, pet) {
		t.Fatal("pet's hit did not kill the monster")
	}

	frames := drainFrames(t, h.client)
	if findSystemMessage(t, frames, serverpackets.SystemMessageOverHit) == nil {
		t.Fatal("no OVER_HIT message for the pet's owner")
	}
	// (5000 + round(0.25*5000)) - round(6250*0.5); 25 - round(25*0.5).
	if exp, sp := earnedExpSp(t, frames); exp != 3125 || sp != 12 {
		t.Fatalf("owner earned %d exp %d SP, want 3125/12", exp, sp)
	}
}

// TestPetKillDeepBlueUsesPetLevel applies the deep-blue drop penalty from
// the attackers' top level: a level-10 pet on level-1 monsters keeps 28%
// of the drop chance, although its level-1 owner would keep it whole.
func TestPetKillDeepBlueUsesPetLevel(t *testing.T) {
	t.Parallel()
	h, pet := bootRewardPet(t, -1, gameservertest.WithDeepBlueDropRules(true))

	// Every kill drops unpenalized; penalized, all of them drop with
	// probability 0.28^30 (about 3e-17).
	const kills = 30
	for range kills {
		if !h.spawnRewardMonster(t).TakeDamage(1_000_000, pet) {
			t.Fatal("pet's hit did not kill the monster")
		}
	}

	if drops := h.srv.GroundItems.Snapshots(func(int32) bool { return false }); len(drops) == kills {
		t.Fatalf("all %d kills dropped, want the pet-level deep-blue penalty", kills)
	}
}

// TestServitorDamageCountsForOwnerShare credits a servitor's damage to its
// owner's share and top-dealer total, less the summoning skill's exp
// penalty: owner 250 + servitor 350 beats another player's 400 and earns
// 600/1000 of 5000 exp scaled by (1 - 0.3f) in single precision.
func TestServitorDamageCountsForOwnerShare(t *testing.T) {
	t.Parallel()
	const (
		summonCatSkill = 1111
		catNPCID       = 12600
	)
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: summonCatSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON", NpcID: catNPCID, SummonTotalLifeTime: 1_200_000, ExpPenalty: 0.3,
		StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
	}}), gamesql.NewCharacterSkillStore(db))
	cat := &npc.Template{
		ID: catNPCID, TemplateID: catNPCID, Type: "Servitor", Name: "Kat the Cat", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20,
	}
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{cat})),
		gameservertest.WithSkills(skills))
	c, ownerID := srv.Client, srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, summonCatSkill, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	startInWorld(t, c)
	c.Send(encodeRequestMagicSkillUse(summonCatSkill))
	var servitor *summon.Actor
	srv.AdvanceUntil(t, "servitor in world state", func() bool {
		obj, ok := srv.State.Summon(ownerID)
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	second, secondID := joinSecondPlayer(t, srv)
	monster := spawnRewardMonsterAt(t, srv)
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, second)

	monster.TakeDamage(400, onlinePlayer(t, srv, secondID))
	monster.TakeDamage(250, onlinePlayer(t, srv, ownerID))
	if !monster.TakeDamage(350, servitor) {
		t.Fatal("servitor's hit did not kill the monster")
	}

	assertDropOwner(t, srv, ownerID)
	if exp, sp := earnedExpSp(t, drainFrames(t, c)); exp != 2100 || sp != 15 {
		t.Fatalf("owner earned %d exp %d SP, want 2100/15", exp, sp)
	}
	if exp, sp := earnedExpSp(t, drainFrames(t, second)); exp != 2000 || sp != 10 {
		t.Fatalf("second player earned %d exp %d SP, want 2000/10", exp, sp)
	}
}
