package siege

import (
	"strings"
	"testing"
	"time"

	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// gludioGates are the test Gludio's castles.xml <gates>.
var gludioGates = []string{"gludio_gate", "gludio_wall"}

// gatedCastles is testCastles with gludioGates on Gludio.
func gatedCastles(t *testing.T) *castledata.Table {
	t.Helper()
	var all []*castledata.Castle
	for _, a := range []castledata.CastleAttrs{
		{ID: 1, Alias: "gludio_castle", Name: "Gludio Castle", Gates: gludioGates},
		{ID: 2, Alias: "dion_castle", Name: "Dion Castle"},
		{ID: 3, Alias: "giran_castle", Name: "Giran Castle"},
	} {
		c, err := castledata.NewCastle(a, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, c)
	}
	table, err := castledata.NewTable(all)
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// gateNotifier records the gate steps with whether the battlefield is
// active as they run.
type gateNotifier struct {
	*recordingNotifier
	field *zone.Siege
}

func (n gateNotifier) CloseDoors(gates []string) {
	n.add("close %s field=%t", strings.Join(gates, ","), n.field.Active())
}

func (n gateNotifier) RespawnDoors(gates []string, weak bool) {
	n.add("respawn %s weak=%t field=%t", strings.Join(gates, ","), weak, n.field.Active())
}

// TestSiegeGateSteps pins where the gate steps run in Siege.startSiege,
// endSiege and midVictory: the gates are guarded when the calendars start;
// at the start they close after the siege states and before
// SIEGE_STARTED (711), the battlefield on; at mid-victory they stand again
// at half HP after the alliance notice and before the new siege states;
// at the end they stand again at full HP after the registrations are
// dropped (292) and before the battlefield turns off.
func TestSiegeGateSteps(t *testing.T) {
	w := newWorldOf(t, DefaultConfig(), gatedCastles(t))
	gludio := w.siege(1)
	for _, id := range []int32{band1, rivals} {
		if _, refused := gludio.RegisterAttacker(w.clan(id)); refused {
			t.Fatalf("%#x refused", id)
		}
	}
	w.only(1)
	notes := gateNotifier{recordingNotifier: w.notes, field: w.field}
	w.clock = sim.NewInline(saturday.Add(-time.Second))
	w.e.Start(w.clock.NewQueue("sieges"), notes)
	expectLines(t, "calendars start", w.notes.take(), []string{"guard gludio_gate,gludio_wall"})

	gludio.Start()
	expectLines(t, "start", w.notes.take(), []string{
		"state [BandOne,Rivals]=1",
		"state [Lords]=2",
		"close gludio_gate,gludio_wall field=true",
		"all 711 castle=1",
		"sound systemmsg_e.17",
		"tell [BandOne,Rivals] 1189",
	})

	if _, ok := w.castles.SetOwner(gludio.Castle(), w.clan(band1)); !ok {
		t.Fatal("BandOne did not take Gludio")
	}
	gludio.MidVictory()
	expectLines(t, "mid-victory", w.notes.take(), []string{
		"tell [BandOne,Rivals] 1190",
		"respawn gludio_gate,gludio_wall weak=true field=true",
		"state [Lords,Rivals]=1",
		"state [BandOne]=2",
	})

	gludio.End()
	expectLines(t, "end", w.notes.take(), []string{
		"all 712 castle=1",
		"sound systemmsg_e.18",
		"all 291 BandOne castle=1",
		"taken 1 by BandOne from Lords",
		"reputation Lords -1000 1772 n=1000",
		"reputation BandOne +1000 1773 n=1000",
		"state [Lords,Rivals]=0",
		"state [BandOne]=0",
		"all 292 castle=1",
		"respawn gludio_gate,gludio_wall weak=false field=true",
	})
	if w.field.Active() {
		t.Fatal("the battlefield is still active after the end")
	}
}
