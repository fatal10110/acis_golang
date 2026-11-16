package item

import (
	"testing"
	"time"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// reuseRecordingServitor records the reuse a mirrored herb puts on it.
type reuseRecordingServitor struct {
	*summon.Actor
	disabled map[int32]time.Duration
}

func (s *reuseRecordingServitor) DisableSkill(key int32, delay time.Duration) {
	s.disabled[key] = delay
}

// TestUseHerbMirrorReuseOnServitor pins the servitor's half of a mirrored
// herb cast: it takes the skill's own reuse delay, never the item's, and
// none at all for a delay of 10ms or less.
func TestUseHerbMirrorReuseOnServitor(t *testing.T) {
	for _, tc := range []struct {
		name       string
		skillReuse int
		itemReuse  int32
		want       time.Duration
	}{
		{"no reuse", 0, 0, 0},
		{"10ms stays usable", 10, 0, 0},
		{"11ms disables", 11, 0, 11 * time.Millisecond},
		{"skill delay wins over a longer item delay", 2000, 5000, 2000 * time.Millisecond},
		{"item delay alone never disables", 0, 5000, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := modelskill.Definition{ID: 100, Level: 1, Potion: true, ReuseDelay: tc.skillReuse}
			req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, def, newUseCaster(t), false)
			tmpl, _ := req.Inventory.Templates().Get(1)
			tmpl.EtcItem.ReuseDelay = tc.itemReuse
			servitor := &reuseRecordingServitor{Actor: newServitor(t, 2, 1, 1), disabled: map[int32]time.Duration{}}
			req.Summon = servitor

			res := Use(req)

			if res.Outcome != Applied || res.MirroredSummon == nil {
				t.Fatalf("Outcome = %v, mirrored = %v, want Applied onto the servitor", res.Outcome, res.MirroredSummon)
			}
			got, disabled := servitor.disabled[actorcast.ReuseKey(&def)]
			if want := tc.want > 0; disabled != want || got != tc.want {
				t.Fatalf("servitor reuse = %v (set %v), want %v", got, disabled, tc.want)
			}
			if len(servitor.disabled) > 1 {
				t.Fatalf("servitor reuse keys = %v, want only the skill's", servitor.disabled)
			}
		})
	}
}
