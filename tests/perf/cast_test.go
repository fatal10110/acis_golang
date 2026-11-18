package perf

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The cast variant's two skills. Both are plain HP removals of a few points,
// so a cast lands real damage and its effect path without making a fight
// unwinnable for the 36-HP test class; the reuse keeps a monster from
// recasting on every hit it takes.
const (
	castPlayerSkill  = modelskill.ID(9201)
	castMonsterSkill = modelskill.ID(9202)
)

var castMonsterRef = modelskill.Ref{ID: castMonsterSkill, Level: 1}

// castDefinitions are the cast variant's skills: the player's one-second
// reuse magic hit on its target, and the monster's two-second reuse one on
// the player that struck it.
func castDefinitions() []modelskill.Definition {
	return []modelskill.Definition{
		{
			ID: castPlayerSkill, Level: 1, Activation: modelskill.ActivationActive,
			Target: modelskill.TargetOne, Offensive: true, SkillType: "REAL_DAMAGE", Magic: true,
			CastRange: 600, HitTime: 400, StaticHitTime: true, StaticReuse: true, ReuseDelay: 1000,
			MPConsume: 1, Power: 4,
		},
		{
			ID: castMonsterSkill, Level: 1, Activation: modelskill.ActivationActive,
			Target: modelskill.TargetOne, Offensive: true, SkillType: "REAL_DAMAGE", Magic: true,
			CastRange: 600, HitTime: 400, StaticHitTime: true, StaticReuse: true, ReuseDelay: 2000,
			Power: 3,
		},
	}
}

// castKit is what the cast variant adds to the boot: the skill table the
// monsters' cast seam resolves through and the persistence the players'
// known skills live in.
type castKit struct {
	defs *modelskill.Table
	opt  gameservertest.Option
	// player is the skill the clients cast, known at level 1.
	player modelskill.ID
	// steps are the steps of a client's eight-step cycle that cast instead of
	// attack or walk.
	steps []int
	// afterAttack makes a client cast only once it has landed an attack: a
	// cast before it let a fleeing monster outrun its client.
	afterAttack bool
}

// newCastKit is the cast variant's kit: the two synthetic skills.
func newCastKit(t *testing.T) *castKit {
	t.Helper()
	return newKit(t, modelskill.NewTable(castDefinitions()), castPlayerSkill, []int{2, 4}, false)
}

// newKit is a kit over defs, whose skill player the clients cast on the
// given steps of their cycle; afterAttack is the kit's afterAttack field.
func newKit(t *testing.T, defs *modelskill.Table, player modelskill.ID, steps []int, afterAttack bool) *castKit {
	t.Helper()
	db := sqltest.SharedDB(t)
	persistence := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), defs, gamesql.NewCharacterSkillStore(db))
	return &castKit{defs: defs, opt: gameservertest.WithSkills(persistence), player: player, steps: steps, afterAttack: afterAttack}
}

// casts reports whether the client casts, rather than attacks or walks, on
// step of its eight-step cycle; attacked is whether it has landed an attack.
// A nil kit (a variant without casts) never does.
func (k *castKit) casts(step int, attacked bool) bool {
	return k != nil && slices.Contains(k.steps, step) && (attacked || !k.afterAttack)
}

func encodeMagicSkillUse(skillID modelskill.ID) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(int32(skillID))
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

// mana is the live character's MP surface the harness tops up through.
type mana interface {
	MPValue() float64
	MaxMPValue() float64
	AddMP(amount float64) float64
}
