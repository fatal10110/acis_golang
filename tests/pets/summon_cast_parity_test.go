package pets

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// requireSkillUseOnto finds the pet's MagicSkillUse in frames and checks it
// is aimed at targetID.
func requireSkillUseOnto(t *testing.T, frames [][]byte, petActor *summon.Actor, targetID int32) {
	t.Helper()
	use, ok := firstOpcode(frames, serverpackets.OpcodeMagicSkillUse)
	if !ok {
		t.Fatalf("no MagicSkillUse: opcodes %x", frameOpcodes(frames))
	}
	r := wire.NewReader(use[1:])
	if caster, target := r.ReadInt32(), r.ReadInt32(); caster != petActor.ObjectID() || target != targetID {
		t.Fatalf("MagicSkillUse caster/target = %d/%d, want pet %d onto %d", caster, target, petActor.ObjectID(), targetID)
	}
}

// TestSummonAuraSkillAimsAtItself commands an aura strike while the owner
// has the monster selected. An aura's final target is its caster
// (PlayableAI.tryToCast -> skill.getFinalTarget), so the pet casts it on
// itself, not on the clicked monster.
func TestSummonAuraSkillAimsAtItself(t *testing.T) {
	t.Parallel()
	aura := wolfStrike()
	aura.Target, aura.CastRange, aura.Radius = modelskill.TargetAura, -1, 200
	h, petActor, _ := bootWolfStrikerWith(t, aura)

	h.client.Send(encodeRequestActionUse(wolfStrikeAction, false))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "aura ActionFailed")
	requireSkillUseOnto(t, frames, petActor, petActor.ObjectID())
	drainUntilQuiet(t, h.client)
}

// TestSummonAreaSkillWithoutFinalTargetStartsNothing aims an area strike at
// the pet itself. An aimed area skill has no final target on its caster, so
// the request is dropped before any condition: no message, no turn, no cast.
func TestSummonAreaSkillWithoutFinalTargetStartsNothing(t *testing.T) {
	t.Parallel()
	area := wolfStrike()
	area.Target, area.Radius = modelskill.TargetArea, 200
	h, petActor, _ := bootWolfStrikerWith(t, area)

	runOnPetQueue(t, petActor, func() { petActor.TryUseSkill(wolfStrikeSkill, petActor, false) })
	if frames := drainFrames(t, h.client); len(frames) != 0 {
		t.Fatalf("area strike on the pet itself sent opcodes %x, want nothing", frameOpcodes(frames))
	}
	if petActor.CastingNow() {
		t.Fatal("pet is casting an area strike with no final target")
	}
}

// TestSummonCastRefusalReachesOwner refuses the pet's strike at the monster
// on each commit-time gate the owner is told about: not enough MP or HP
// (CreatureCast.meetsHpMpConditions), no line of sight
// (CreatureCast.canCast), and a failed skill <cond> clause
// (PlayableCast.canCast -> checkCondition). Summon.sendPacket forwards each
// message to the owner, the pet turns toward the monster, no cast starts,
// and nothing is charged.
func TestSummonCastRefusalReachesOwner(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		tune    func(*modelskill.Definition)
		blind   bool
		message func(*testing.T, []byte)
	}{
		{
			name: "not enough MP",
			tune: func(d *modelskill.Definition) { d.MPConsume = 100_000 },
			message: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageNotEnoughMP)
			},
		},
		{
			name: "not enough HP",
			tune: func(d *modelskill.Definition) { d.HPConsume = 100_000 },
			message: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageNotEnoughHP)
			},
		},
		{
			name:  "no line of sight",
			tune:  func(*modelskill.Definition) {},
			blind: true,
			message: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageCantSeeTarget)
			},
		},
		{
			// The clause holds only at or under half HP; the pet is at full.
			name: "failed skill condition",
			tune: func(d *modelskill.Definition) {
				d.Conditions = []modelskill.ConditionClause{{
					Root:      modelskill.Condition{Kind: "player", Attrs: map[string]string{"hp": "50"}},
					MessageID: serverpackets.SystemMessageS1CannotBeUsed, AddName: true,
				}}
			},
			message: func(t *testing.T, f []byte) {
				assertSystemMessageSkill(t, f, serverpackets.SystemMessageS1CannotBeUsed, wolfStrikeSkill, 1)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			strike := wolfStrike()
			strike.MPInitialConsume = 5
			tc.tune(&strike)
			geo := &sightGeo{}
			h, petActor, hostile := bootWolfStrikerWith(t, strike, gameservertest.WithGeo(geo))
			geo.blind.Store(tc.blind)
			mp, hp := petActor.MPValue(), petActor.HP()

			runOnPetQueue(t, petActor, func() { petActor.TryUseSkill(wolfStrikeSkill, hostile, false) })
			assertSummonCastRejected(t, h, petActor, hostile.ObjectID(), func(frame []byte) { tc.message(t, frame) })
			if petActor.CastingNow() {
				t.Fatal("refused strike left the pet casting")
			}
			if gotMP, gotHP := petActor.MPValue(), petActor.HP(); gotMP != mp || gotHP != hp {
				t.Fatalf("pet MP/HP = %v/%v after the refusal, want untouched %v/%v", gotMP, gotHP, mp, hp)
			}
		})
	}
}

// TestSummonCostRefusalComesBeforeSight refuses a strike on two gates at
// once: the pet has neither the MP nor a line of sight. The reference checks
// the costs first (CreatureCast.canCast), so the owner reads NOT_ENOUGH_MP
// alone.
func TestSummonCostRefusalComesBeforeSight(t *testing.T) {
	t.Parallel()
	strike := wolfStrike()
	strike.MPConsume = 100_000
	geo := &sightGeo{}
	h, petActor, hostile := bootWolfStrikerWith(t, strike, gameservertest.WithGeo(geo))
	geo.blind.Store(true)

	runOnPetQueue(t, petActor, func() { petActor.TryUseSkill(wolfStrikeSkill, hostile, false) })
	assertSummonCastRejected(t, h, petActor, hostile.ObjectID(), func(frame []byte) {
		assertStaticSystemMessage(t, frame, serverpackets.SystemMessageNotEnoughMP)
	})
}

// TestSummonStrikeOnReuseTellsOwner presses the strike a second time while
// its reuse runs: CreatureCast.canAttemptCast answers S1_PREPARED_FOR_REUSE,
// which Summon.sendPacket forwards to the owner, and nothing else happens.
func TestSummonStrikeOnReuseTellsOwner(t *testing.T) {
	t.Parallel()
	h, petActor, hostile := bootWolfStriker(t)
	startWolfStrike(t, h)
	h.srv.AdvanceUntil(t, "strike landing on the monster", func() bool { return hostile.HP() < float64(hostile.MaxHP()) })
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeRequestActionUse(wolfStrikeAction, false))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "reuse ActionFailed")
	if len(frames) != 2 {
		t.Fatalf("strike on reuse sent opcodes %x, want S1_PREPARED_FOR_REUSE then ActionFailed", frameOpcodes(frames))
	}
	assertSystemMessageSkill(t, frames[0], serverpackets.SystemMessageS1PreparedForReuse, wolfStrikeSkill, 1)
	if petActor.CastingNow() {
		t.Fatal("strike on reuse started a cast")
	}
}

// TestSummonStrikeOnOwnFlaggedOwnerIsInvalid has a PvP-flagged owner select
// themself and command the pet's offensive single-target strike. The
// reference judges ONE against the acting player, and
// Playable.canCastOffensiveSkillOnPlayable refuses its own side whatever the
// flag or CTRL: the owner reads INVALID_TARGET, the pet turns toward them,
// and no cast starts.
func TestSummonStrikeOnOwnFlaggedOwnerIsInvalid(t *testing.T) {
	t.Parallel()
	for _, ctrl := range []bool{false, true} {
		name := "without ctrl"
		if ctrl {
			name = "with ctrl"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, petActor, _ := bootWolfStrikerWith(t, wolfStrike())
			owner, _ := h.srv.State.Player(h.ownerID)
			owner.(interface{ UpdatePvPFlag(task.PvPFlagState) }).UpdatePvPFlag(task.PvPFlagOn)
			h.targetPlayer(t, h.ownerID)

			h.client.Send(encodeRequestActionUse(wolfStrikeAction, ctrl))
			frames := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "strike ActionFailed")
			requireSummonStrikeRefused(t, frames, petActor, h.ownerID)
			if petActor.CastingNow() {
				t.Fatal("refused strike on the owner left the pet casting")
			}
		})
	}
}

// TestForcedSummonStrikeOnUnflaggedPlayerLands forces the pet's strike on an
// unflagged player with CTRL. Nothing re-judges the target conditions once
// the cast has started (CreatureCast.onMagicLaunch -> getTargetList), so the
// launch names the player and the hit lands.
func TestForcedSummonStrikeOnUnflaggedPlayerLands(t *testing.T) {
	t.Parallel()
	h, petActor, bystanderID := bootWolfStrikerWithBystander(t, wolfStrike())
	h.targetPlayer(t, bystanderID)
	obj, ok := h.srv.State.Player(bystanderID)
	if !ok {
		t.Fatal("bystander missing from world state")
	}
	bystander := obj.(interface{ HP() float64 })
	before := bystander.HP()

	h.client.Send(encodeRequestActionUse(wolfStrikeAction, true))
	requireSummonStrikeStarted(t, readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "strike ActionFailed"), petActor, bystanderID)

	h.srv.Advance(t, wolfStrikeLaunch)
	launched := readUntilOpcode(t, h.client, serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	if got, want := launchedTargets(t, launched[len(launched)-1]), []int32{bystanderID}; !slices.Equal(got, want) {
		t.Fatalf("forced strike launched onto %v, want %v", got, want)
	}
	h.srv.AdvanceUntil(t, "forced strike landing on the player", func() bool { return bystander.HP() < before })
	drainUntilQuiet(t, h.client)
}

// Pet item-skill fixtures: the suite's mana potion, an ItemSkills etc item a
// pet may use (tradable). The test gives its skill a target-side clause, the
// shape shipped by 8192 Breaking Arrow (2234), 8274 Chapel Key (2236) and
// 8556 Dewdrop of Destruction (2276).
const (
	petPotionID         = int32(728)
	petPotionSkill      = 2279
	petPotionSkillLevel = 2
)

// TestPetItemSkillConditionJudgesThePetTarget has the pet use an ItemSkills
// item whose skill requires its target to be the fixture monster.
// ItemSkills.useItem checks the clause against the pet's own target
// (playable.getTarget()): with the monster selected the pet uses the item,
// with nothing selected the owner reads the clause's message and the pet
// casts nothing.
func TestPetItemSkillConditionJudgesThePetTarget(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		target bool
	}{
		{"monster targeted", true},
		{"no target", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			potionSkill := modelskill.Definition{
				ID: petPotionSkill, Level: petPotionSkillLevel, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				SkillType: "DUMMY", Potion: true, StaticHitTime: true, StaticReuse: true,
				Conditions: []modelskill.ConditionClause{{
					Root:      modelskill.Condition{Kind: "target", Attrs: map[string]string{"npcid": "100"}},
					MessageID: serverpackets.SystemMessageS1CannotBeUsed, AddName: true,
				}},
			}
			db := sqltest.SharedDB(t)
			skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
				{
					ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
				},
				potionSkill,
			}), gamesql.NewCharacterSkillStore(db))
			h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate()})),
				gameservertest.WithSkills(skills),
			}, seedItem{TemplateID: petPotionID, Count: 2})
			petActor, _ := h.spawnWolf(t)
			hostile := h.srv.SpawnHostileNPC(t)
			drainUntilQuiet(t, h.client)
			potionID := h.seededItem(t, petPotionID)
			h.giveToPet(t, potionID, 2)
			if tc.target {
				runOnPetQueue(t, petActor, func() { petActor.SetTarget(hostile) })
			}

			h.client.Send(encodeRequestPetUseItem(potionID))
			frames := drainFrames(t, h.client)
			var sawUse, sawRefusal bool
			for _, f := range frames {
				if f[0] != serverpackets.OpcodeSystemMessage {
					continue
				}
				switch systemMessageID(t, f) {
				case serverpackets.SystemMessagePetUsesS1:
					sawUse = true
				case serverpackets.SystemMessageS1CannotBeUsed:
					assertSystemMessageSkill(t, f, serverpackets.SystemMessageS1CannotBeUsed, petPotionSkill, 1)
					sawRefusal = true
				}
			}
			if sawUse != tc.target || sawRefusal == tc.target {
				t.Fatalf("PET_USES_S1 = %v, S1_CANNOT_BE_USED = %v; want use %v: opcodes %x", sawUse, sawRefusal, tc.target, frameOpcodes(frames))
			}
			if got, ok := firstOpcode(frames, serverpackets.OpcodeMagicSkillUse); ok != tc.target {
				t.Fatalf("MagicSkillUse sent = %v (%x), want %v", ok, got, tc.target)
			}
		})
	}
}
