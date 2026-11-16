package pets

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const summonSoullessSkill = 1278

// contractPaymentDamage is the datapack's MP drain per tick of each
// Contract Payment level.
var contractPaymentDamage = [12]float64{3, 3.3, 3.6, 3.9, 4.2, 4.5, 4.8, 4.8, 5.1, 5.1, 5.4, 5.4}

// contractPaymentDefs are the datapack's twelve Contract Payment levels.
func contractPaymentDefs() []modelskill.Definition {
	defs := make([]modelskill.Definition, 0, len(contractPaymentDamage))
	for i, damage := range contractPaymentDamage {
		defs = append(defs, modelskill.Definition{
			ID: modelskill.ContractPaymentSkillID, Level: i + 1, Activation: modelskill.ActivationActive,
			Target: modelskill.TargetOne, SkillType: "MDOT", Magic: true, Debuff: true, StayAfterDeath: true,
			EffectRange: 1100, Power: 100,
			Effects: []modelskill.EffectTemplate{{
				Name: "ManaDamOverTime", Count: 20, Time: 3, Value: damage,
				StackType: "dot_mp", StackOrder: float64(i + 1), Icon: true,
			}},
		})
	}
	return defs
}

// bootSoullessOwner boots an owner who knows Summon Soulless at level, in a
// skill table that also holds every Contract Payment level.
func bootSoullessOwner(t *testing.T, level int) *servitorOwner {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(append([]modelskill.Definition{{
		ID: summonSoullessSkill, Level: level, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON", NpcID: killableServitorNPCID, SummonTotalLifeTime: 1_200_000,
		SummonTimeLostIdle: 500, SummonTimeLostActive: 1000,
		StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
	}}, contractPaymentDefs()...)), gamesql.NewCharacterSkillStore(db))
	soulless := &npc.Template{
		ID: killableServitorNPCID, TemplateID: killableServitorNPCID, Type: "Servitor", Name: "Soulless", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20, CorpseTime: servitorCorpseTime,
	}
	srv := bootPets(t, gameservertest.WithNPCs(npc.NewTable([]*npc.Template{soulless})), gameservertest.WithSkills(skills))
	ownerID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, summonSoullessSkill, level); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	startInWorld(t, srv.Client)
	return &servitorOwner{srv: srv, client: srv.Client, id: ownerID, seeded: map[int32][]int32{}}
}

// contractPaymentIcon returns the Contract Payment level the latest
// AbnormalStatusUpdate in frames shows, and whether one shows it.
func contractPaymentIcon(t *testing.T, frames [][]byte) (level int, ok bool) {
	t.Helper()
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeAbnormalStatusUpdate {
			continue
		}
		r := wire.NewReader(frame[1:])
		level, ok = 0, false
		for range r.ReadUint16() {
			id, lvl, _ := r.ReadInt32(), r.ReadUint16(), r.ReadInt32()
			if id == int32(modelskill.ContractPaymentSkillID) {
				level, ok = int(lvl), true
			}
		}
		if err := r.Err(); err != nil {
			t.Fatalf("read AbnormalStatusUpdate: %v", err)
		}
	}
	return level, ok
}

// TestSummonSoullessPaysContract calls a Soulless out at several skill
// levels. Its caster takes Contract Payment at the summon level less two,
// kept within 1..12, and sees its icon; each tick drains that level's MP,
// and the debuff never runs out on its own, outlasting its 20 ticks. When
// the Soulless is dismissed the debuff goes with it.
func TestSummonSoullessPaysContract(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ summonLevel, payment int }{
		{1, 1}, {4, 2}, {14, 12},
	} {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			o := bootSoullessOwner(t, tc.summonLevel)
			obj, ok := o.srv.State.Player(o.id)
			if !ok {
				t.Fatal("owner missing from world state")
			}
			owner, ok := network.OnlineCharacter(obj)
			if !ok {
				t.Fatalf("world player %T is not an online character", obj)
			}

			o.client.Send(encodeRequestMagicSkillUse(summonSoullessSkill))
			o.srv.AdvanceUntil(t, "soulless in world state", func() bool {
				s, ok := o.srv.State.Summon(o.id)
				_, isSummon := s.(*summon.Actor)
				return ok && isSummon
			})
			frames := drainFrames(t, o.client)
			if level, ok := owner.EffectList().ActiveBySkillID(int(modelskill.ContractPaymentSkillID)); !ok || level != tc.payment {
				t.Fatalf("summon level %d: Contract Payment active=%v level %d, want level %d", tc.summonLevel, ok, level, tc.payment)
			}
			if level, ok := contractPaymentIcon(t, frames); !ok || level != tc.payment {
				t.Fatalf("summon level %d: Contract Payment icon shown=%v level %d, want level %d (frames %x)",
					tc.summonLevel, ok, level, tc.payment, frameOpcodes(frames))
			}

			o.srv.Advance(t, 3*time.Second)
			before := owner.MPValue()
			o.srv.TickEffects()
			if got, want := owner.MPValue(), before-contractPaymentDamage[tc.payment-1]; math.Abs(got-want) > 1e-9 {
				t.Fatalf("summon level %d: MP after a Contract Payment tick = %v, want %v", tc.summonLevel, got, want)
			}

			o.srv.AdvanceWithEffects(t, 75*time.Second)
			if _, ok := owner.EffectList().ActiveBySkillID(int(modelskill.ContractPaymentSkillID)); !ok {
				t.Fatalf("summon level %d: Contract Payment ran out after 25 ticks, want it held while the Soulless stays", tc.summonLevel)
			}

			o.dismissServitor(t)
			if _, ok := owner.EffectList().ActiveBySkillID(int(modelskill.ContractPaymentSkillID)); ok {
				t.Fatalf("summon level %d: Contract Payment still active after the Soulless left", tc.summonLevel)
			}
		})
	}
}
