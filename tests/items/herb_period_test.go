package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// herbOfPowerItem is the Herb of Power, carrying skill 2280 (a 120 s
// P. Atk. buff).
const herbOfPowerItem = 8606

// herbOfPowerSkills is the skill table the Herb of Power resolves against,
// shaped like the datapack's skill 2280.
func herbOfPowerSkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: 2280, Level: 1, Name: "Herb of Power", Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", Potion: true,
		Effects: []modelskill.EffectTemplate{{
			Name: "Buff", Count: 1, Time: 120, Icon: true, StackType: "pa_up", StackOrder: 2.9,
			Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "pAtk", Value: 1.1}},
		}},
	}}), gamesql.NewCharacterSkillStore(db))
}

// herbIconSeconds is the icon time, in seconds, the last AbnormalStatusUpdate
// among frames gives skill 2280; ok is false when none lists it.
func herbIconSeconds(t *testing.T, frames [][]byte) (seconds int32, ok bool) {
	t.Helper()
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeAbnormalStatusUpdate {
			continue
		}
		r := wire.NewReader(f[1:])
		for range r.ReadUint16() {
			id, _, dur := r.ReadInt32(), r.ReadUint16(), r.ReadInt32()
			if id == 2280 {
				seconds, ok = dur, true
			}
		}
	}
	return seconds, ok
}

// TestHerbBuffHalvedWithSummon picks up a Herb of Power: on a player alone
// its buff shows 120 s; with a servitor out it runs half as long, 60 s, on
// the player and on the servitor the herb mirrors onto.
func TestHerbBuffHalvedWithSummon(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		servitor bool
		want     int32
	}{
		{name: "alone", want: 120},
		{name: "with servitor", servitor: true, want: 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			catalog := item.NewTable(append(gameservertest.ItemTemplates().All(), &item.Template{
				ID: herbOfPowerItem, Name: "Herb of Power", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
				EtcItem:        &item.EtcItemDetail{Type: item.EtcItemHerb, Handler: "ItemSkills"},
				AttachedSkills: []item.SkillRef{{ID: 2280, Level: 1}},
			}))
			srv := gameservertest.Boot(t,
				gameservertest.WithItemTemplates(catalog),
				gameservertest.WithSkills(herbOfPowerSkills(t)),
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1))
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)

			var servitor *summon.Actor
			if tc.servitor {
				var err error
				servitor, err = summon.NewServitor(summon.ServitorConfig{
					ObjectID: srv.NewObjectID(),
					Level:    44,
					Stats:    summon.CombatStats{MaxHP: 500, MaxMP: 200},
				})
				if err != nil {
					t.Fatal(err)
				}
				servitor.SetQueue(srv.PlayerQueue(t, objID))
				srv.State.AddSummon(objID, servitor)
			}
			srv.SeedGroundItem(t, objID, herbOfPowerItem, 1, spawnX, spawnY, spawnZ)
			drainUntilQuiet(t, c)
			groundID := soleGroundObjectID(t, srv)

			c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
			got, ok := herbIconSeconds(t, collectUntilQuiet(t, c))
			if !ok {
				t.Fatal("herb pickup sent no AbnormalStatusUpdate listing skill 2280")
			}
			// The icon time is the whole seconds left until expiry, so the
			// milliseconds the pickup itself takes may round it down by one.
			if got != tc.want && got != tc.want-1 {
				t.Fatalf("Herb of Power icon time = %d s, want %d (or %d)", got, tc.want, tc.want-1)
			}
			if servitor != nil {
				held := servitor.EffectList().All()
				if len(held) != 1 || held[0].Skill.ID != 2280 || held[0].Period() != 60 {
					t.Fatalf("servitor holds %d effects, want one 2280 buff of 60 s", len(held))
				}
			}
		})
	}
}
