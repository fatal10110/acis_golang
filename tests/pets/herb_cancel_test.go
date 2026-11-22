package pets

import (
	"context"
	"slices"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// herbOfPowerID is a herb skill: its effects run at half period on a
	// player with a summon.
	herbOfPowerID  = 2280
	plainBuffID    = 4113
	cancelSelfID   = 4114
	herbCancelTime = 7200
)

// TestCancelSkillSeesHerbHalvedPeriod has an owner with a wolf out buff
// itself with a herb and with an ordinary buff of the same 7200 s period,
// then cast a cancel (power 0, same level as the owner) at itself, once as a CANCEL skill and once as a Cancel effect, with
// the roll pinned at 45. The ordinary buff's rate is 7200/120 = 60, so it is
// stripped; the herb runs 3600 s, rates 30, and stays.
func TestCancelSkillSeesHerbHalvedPeriod(t *testing.T) {
	t.Parallel()
	cancelBase := modelskill.Definition{
		ID: cancelSelfID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 0, StaticHitTime: true, StaticReuse: true,
	}
	skillType := cancelBase
	skillType.SkillType, skillType.MagicLevel = "CANCEL", 1
	effectType := cancelBase
	effectType.SkillType = "BUFF"
	effectType.Effects = []modelskill.EffectTemplate{{Name: "Cancel", EffectPower: 0}}
	for _, tc := range []struct {
		name   string
		cancel modelskill.Definition
	}{
		{name: "cancel skill handler", cancel: skillType},
		{name: "cancel effect", cancel: effectType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cancelHerbHalved(t, tc.cancel)
		})
	}
}

func cancelHerbHalved(t *testing.T, cancel modelskill.Definition) {
	t.Helper()
	selfBuff := func(id int, stack string) modelskill.Definition {
		return modelskill.Definition{
			ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 0, StaticHitTime: true, StaticReuse: true,
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: herbCancelTime, Icon: true, StackType: stack, StackOrder: 1}},
		}
	}
	defs := []modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		selfBuff(herbOfPowerID, "herb_probe"),
		selfBuff(plainBuffID, "plain_probe"),
		cancel,
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(defs), gamesql.NewCharacterSkillStore(db))
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate()})),
		gameservertest.WithSkills(skills),
	)
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	for _, id := range []int{herbOfPowerID, plainBuffID, cancelSelfID} {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, id, 1); err != nil {
			t.Fatalf("seed known skill %d: %v", id, err)
		}
	}
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)
	h.spawnWolf(t)
	owner := h.character(t)
	if owner.Level() != 1 {
		t.Fatalf("owner level = %d, want 1", owner.Level())
	}

	for _, id := range []int32{herbOfPowerID, plainBuffID} {
		h.cast(t, id)
	}
	activeBuffs := func() (ids []int32) {
		done := make(chan struct{})
		if !owner.Queue().Post(func() {
			for _, e := range owner.EffectList().All() {
				// The cancel effect form stays on the list itself.
				if e.Skill.ID != cancelSelfID {
					ids = append(ids, int32(e.Skill.ID))
				}
			}
			close(done)
		}) {
			t.Fatal("post effect read: queue closed")
		}
		<-done
		slices.Sort(ids)
		return ids
	}
	if got := activeBuffs(); !slices.Equal(got, []int32{herbOfPowerID, plainBuffID}) {
		t.Fatalf("owner effects before the cancel = %v, want the herb and the plain buff", got)
	}

	done := make(chan struct{})
	if !owner.Queue().Post(func() {
		owner.SetRollSource(func(int) int { return 45 })
		close(done)
	}) {
		t.Fatal("post roll pin: queue closed")
	}
	<-done
	h.cast(t, cancelSelfID)
	if got := activeBuffs(); !slices.Equal(got, []int32{herbOfPowerID}) {
		t.Fatalf("owner effects after the cancel = %v, want only the herb (3600 s, rate 30)", got)
	}
}

// cast has the owner use skillID and lets the cast end.
func (h *petWorld) cast(t *testing.T, skillID int32) {
	t.Helper()
	h.client.Send(encodeRequestMagicSkillUse(skillID))
	readUntilOpcode(t, h.client, serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	h.srv.Advance(t, 2*time.Second)
	drainUntilQuiet(t, h.client)
}
