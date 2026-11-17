package perf

import (
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
}

func newCastKit(t *testing.T) *castKit {
	t.Helper()
	db := sqltest.SharedDB(t)
	defs := modelskill.NewTable(castDefinitions())
	persistence := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), defs, gamesql.NewCharacterSkillStore(db))
	return &castKit{defs: defs, opt: gameservertest.WithSkills(persistence)}
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
