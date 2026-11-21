package pets

import (
	"slices"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// mpRatePassiveID is a wolf passive that halves its physical skill MP costs,
// quarters its magic ones and scales its dance/song ones by 0.75.
const mpRatePassiveID = 9601

func mpRatePassive() modelskill.Definition {
	return modelskill.Definition{
		ID: mpRatePassiveID, Level: 1, Activation: modelskill.ActivationPassive,
		Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncMul, Stat: "PhysicalMpConsumeRate", Value: 0.5},
			{Op: modelskill.FuncMul, Stat: "MagicalMpConsumeRate", Value: 0.25},
			{Op: modelskill.FuncMul, Stat: "DanceMpConsumeRate", Value: 0.75},
		},
	}
}

// bootWolfStrikerWithPassive is bootWolfStrikerWith with mpRatePassive on
// the wolf's template.
func bootWolfStrikerWithPassive(t *testing.T, strike modelskill.Definition) (*petWorld, *summon.Actor, *npc.Hostile) {
	t.Helper()
	wolf := wolfTemplate()
	wolf.Skills = map[int]int{wolfStrikeSkill: 1}
	wolf.Passives = []modelskill.Ref{{ID: mpRatePassiveID, Level: 1}}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		strike,
		mpRatePassive(),
	}), gamesql.NewCharacterSkillStore(db))
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, treeTemplate()})),
		gameservertest.WithSkills(skills),
	})
	petActor, _ := h.spawnWolf(t)
	hostile := h.srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	drainFrames(t, h.client)
	return h, petActor, hostile
}

// readUntilSystemMessage collects the owner's frames until one is system
// message id, returning every frame read including the match.
func readUntilSystemMessage(t *testing.T, h *petWorld, id int, what string) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 100 {
		frames = append(frames, readUntilOpcode(t, h.client, serverpackets.OpcodeSystemMessage, what)...)
		if systemMessageID(t, frames[len(frames)-1]) == id {
			return frames
		}
	}
	t.Fatalf("%s not found within 100 system messages", what)
	return nil
}

// TestSummonCastStartTellsOwnerUseS1 pins CreatureCast.doCast
// (CreatureCast.java:148-151): a summon's cast start broadcasts
// MagicSkillUse, then sends USE_S1 naming the skill, which Summon.sendPacket
// forwards to the owner. A summon's cast carries no item, so every cast
// sends it, once.
func TestSummonCastStartTellsOwnerUseS1(t *testing.T) {
	t.Parallel()
	h, petActor, hostile := bootWolfStriker(t)

	h.client.Send(encodeRequestActionUse(wolfStrikeAction, false))
	frames := readUntilSystemMessage(t, h, serverpackets.SystemMessageUseS1, "summon cast USE_S1")
	use := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeMagicSkillUse })
	if use < 0 {
		t.Fatalf("USE_S1 arrived before the pet's MagicSkillUse: opcodes %x", frameOpcodes(frames))
	}
	requireSkillUseOnto(t, frames[use:], petActor, hostile.ObjectID())
	assertSystemMessageSkill(t, frames[len(frames)-1], serverpackets.SystemMessageUseS1, wolfStrikeSkill, 1)

	h.srv.AdvanceUntil(t, "strike landing", func() bool { return !petActor.CastingNow() })
	frames = append(frames, drainFrames(t, h.client)...)
	if n := countSystemMessage(t, frames, serverpackets.SystemMessageUseS1); n != 1 {
		t.Fatalf("owner read USE_S1 %d times over the cast, want once", n)
	}
}

func countSystemMessage(t *testing.T, frames [][]byte, id int) int {
	t.Helper()
	n := 0
	for _, got := range systemMessageIDs(t, frames) {
		if got == id {
			n++
		}
	}
	return n
}

// TestSummonCastMPCostFollowsConsumeRate pins CreatureStatus.getMpConsume and
// getMpInitialConsume (CreatureStatus.java:698-740) for a summon: both costs
// scale by DanceMpConsumeRate for a dance/song, else MagicalMpConsumeRate
// for a magic skill, else PhysicalMpConsumeRate, and the cast gate
// (meetsHpMpConditions) checks their scaled sum. A dance/song's final cost
// first adds NextDanceCost per dance/song already active on the pet; its
// initial cost does not. The pet holds exactly that sum, well under the raw
// costs, so the cast starts, pays the scaled initial cost at the start, the
// scaled final cost at the hit, and ends on zero MP.
func TestSummonCastMPCostFollowsConsumeRate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		magic, dance  bool
		initial, last int
	}{
		// int(10*0.5) = 5, int(20*0.5) = 10.
		{name: "physical", initial: 5, last: 10},
		// int(10*0.25) = 2, int(20*0.25) = 5.
		{name: "magic", magic: true, initial: 2, last: 5},
		// The dance rate wins over the magic one. One dance active:
		// int(10*0.75) = 7, int((20+1*8)*0.75) = 21.
		{name: "dance", magic: true, dance: true, initial: 7, last: 21},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			strike := wolfStrike()
			strike.Magic, strike.Dance = tc.magic, tc.dance
			strike.MPInitialConsume, strike.MPConsume = 10, 20
			if tc.dance {
				extra := strike.Extra()
				extra.NextDanceCost = 8
				strike = strike.WithExtra(extra)
			}
			h, petActor, hostile := bootWolfStrikerWithPassive(t, strike)
			if tc.dance {
				addToPet(t, petActor, danceOn(t, petActor))
				drainUntilQuiet(t, h.client)
			}
			total := float64(tc.initial + tc.last)
			runOnPetQueue(t, petActor, func() { petActor.ReduceMP(petActor.MPValue() - total) })
			if got := petActor.MPValue(); got != total {
				t.Fatalf("pet MP = %v before the cast, want %v", got, total)
			}

			startWolfStrike(t, h)
			if got, want := petActor.MPValue(), total-float64(tc.initial); got != want {
				t.Fatalf("pet MP = %v after the cast start, want %v (scaled initial cost %d paid)", got, want, tc.initial)
			}
			h.srv.AdvanceUntil(t, "strike landing", func() bool { return hostile.HP() < float64(hostile.MaxHP()) })
			if got := petActor.MPValue(); got != 0 {
				t.Fatalf("pet MP = %v after the hit, want 0 (scaled final cost %d paid)", got, tc.last)
			}
			drainUntilQuiet(t, h.client)
		})
	}
}

// TestSummonCastFailingAtHitTellsOwner pins CreatureCast.onMagicHitTimer
// (CreatureCast.java:248-271): when the pet no longer holds its final MP or
// HP cost at the hit, it sends NOT_ENOUGH_MP or NOT_ENOUGH_HP, forwarded to
// the owner by Summon.sendPacket, before stop() broadcasts
// MagicSkillCanceled; the skill does not land.
func TestSummonCastFailingAtHitTellsOwner(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		tune    func(*modelskill.Definition)
		drain   func(*summon.Actor)
		message int
	}{
		{
			name:    "MP drained",
			tune:    func(d *modelskill.Definition) { d.MPConsume = 20 },
			drain:   func(p *summon.Actor) { p.ReduceMP(p.MPValue()) },
			message: serverpackets.SystemMessageNotEnoughMP,
		},
		{
			name:    "HP drained",
			tune:    func(d *modelskill.Definition) { d.HPConsume = 10 },
			drain:   func(p *summon.Actor) { p.ConsumeHP(p.HP() - 5) },
			message: serverpackets.SystemMessageNotEnoughHP,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			strike := wolfStrike()
			tc.tune(&strike)
			h, petActor, hostile := bootWolfStrikerWith(t, strike)

			startWolfStrike(t, h)
			runOnPetQueue(t, petActor, func() { tc.drain(petActor) })
			h.srv.AdvanceUntil(t, "strike stopped at the hit", func() bool { return !petActor.CastingNow() })
			frames := readUntilOpcode(t, h.client, serverpackets.OpcodeMagicSkillCanceled, "pet MagicSkillCanceled")
			canceled := len(frames) - 1
			message := slices.IndexFunc(frames, func(f []byte) bool {
				return f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == tc.message
			})
			if message < 0 {
				t.Fatalf("owner frames before MagicSkillCanceled = %x (system messages %v), want message %d first",
					frameOpcodes(frames), systemMessageIDs(t, frames), tc.message)
			}
			assertStaticSystemMessage(t, frames[message], tc.message)
			if message > canceled {
				t.Fatalf("message at %d after MagicSkillCanceled at %d", message, canceled)
			}
			if hostile.HP() != float64(hostile.MaxHP()) {
				t.Fatalf("monster HP = %v, want untouched %d: the failed hit landed", hostile.HP(), hostile.MaxHP())
			}
			drainUntilQuiet(t, h.client)
		})
	}
}

// danceOn is an active dance effect on pet, counted by the per-cast
// dance/song MP surcharge.
func danceOn(t *testing.T, pet *summon.Actor) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 275, Level: 1, Dance: true}, modelskill.EffectTemplate{
		Name: "Buff", Time: 120, StackType: "dance_of_fury", StackOrder: 1,
	})
	if err != nil {
		t.Fatalf("effect.New(dance): %v", err)
	}
	e.Effector, e.Effected = pet, pet
	return e
}
