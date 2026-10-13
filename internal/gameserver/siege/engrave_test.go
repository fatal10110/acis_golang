package siege

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/rs/zerolog"
)

// twoArtifactWorld is a siege engine over Gludio (1), owned by lords and
// attacked by rivals and others, with two artifacts placed as objects 501
// and 502, its siege started.
func twoArtifactWorld(t *testing.T) *world {
	t.Helper()
	var artifacts []castledata.Artifact
	for _, pos := range []string{"10;0;0;0", "-10;0;0;32768"} {
		a, err := castledata.NewArtifact(35322, pos)
		if err != nil {
			t.Fatal(err)
		}
		artifacts = append(artifacts, a)
	}
	gludio, err := castledata.NewCastle(castledata.CastleAttrs{ID: 1, Alias: "gludio_castle", Name: "Gludio Castle"}, artifacts, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := castledata.NewTable([]*castledata.Castle{gludio})
	if err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, clans: testClans(), store: &recordingStore{}, notes: &recordingNotifier{}}
	w.castles = castle.NewManager(data, w.clans, nil, nil, zerolog.Nop())
	w.castles.Restore([]castle.Row{{ID: 1, SiegeDate: saturday.UnixMilli()}}, []castle.Owner{{ClanID: lords, CastleID: 1}})
	next := int32(500)
	if placed := w.castles.SpawnArtifacts(func(castledata.Artifact) (int32, bool) { next++; return next, true }); placed != 2 {
		t.Fatalf("placed %d artifacts, want 2", placed)
	}
	form, err := zone.NewCuboid(-1000, 1000, -1000, 1000, -1000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	w.field = zone.NewSiege(11, form)
	w.field.ResidenceID = 1
	w.e = New(DefaultConfig(), w.castles, w.clans, []*zone.Siege{w.field}, w.store, nil, zerolog.Nop())
	if err := w.e.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int32{rivals, others} {
		if _, refused := w.siege(1).RegisterAttacker(w.clan(id)); refused {
			t.Fatalf("%#x refused", id)
		}
	}
	w.start(saturday.Add(-time.Second))
	w.siege(1).Start()
	w.notes.take()
	return w
}

// TestEngrave pins Castle.engrave: each engraving tells the attackers, then
// the owner and the defenders, that the clan engraved the ruler; the
// castle is the clan's to take only once every artifact carries its ruler,
// and that wipes them all. An object that is not one of the castle's
// artifacts records and tells nothing.
func TestEngrave(t *testing.T) {
	w := twoArtifactWorld(t)
	gludio := w.siege(1)
	steps := []struct {
		clan     int32
		artifact int32
		taken    bool
		told     bool
	}{
		{rivals, 999, false, false},
		{rivals, 501, false, true},
		{others, 502, false, true},
		{rivals, 502, true, true},
		// Taking the castle wiped 501 too.
		{rivals, 502, false, true},
	}
	for i, s := range steps {
		if got := gludio.Engrave(w.clan(s.clan), s.artifact); got != s.taken {
			t.Fatalf("step %d: Engrave(%#x, %d) = %v, want %v", i, s.clan, s.artifact, got, s.taken)
		}
		var want []string
		if s.told {
			name := w.clan(s.clan).Name()
			want = []string{"tell [Rivals,Others] 285 " + name, "tell [Lords] 285 " + name}
		}
		expectLines(t, "engrave", w.notes.take(), want)
	}
}

// TestSiegeEndWipesEngravings pins the end of Siege.endSiege: the rulers
// engraved on the castle's artifacts are wiped, so a clan holding one
// artifact when the siege ends must engrave both again.
func TestSiegeEndWipesEngravings(t *testing.T) {
	w := twoArtifactWorld(t)
	gludio := w.siege(1)
	if gludio.Engrave(w.clan(rivals), 501) {
		t.Fatal("one artifact took the castle")
	}
	gludio.End()
	w.e.mu.Lock()
	left := len(gludio.engraved)
	w.e.mu.Unlock()
	if left != 0 {
		t.Fatalf("%d engravings outlived the siege's end", left)
	}
}

// TestEngraveAfterEnd pins that an engraving landing once the siege is over
// records and tells nothing and takes no castle, though the check its cast
// ran passed while the siege was fought.
func TestEngraveAfterEnd(t *testing.T) {
	w := twoArtifactWorld(t)
	gludio := w.siege(1)
	gludio.End()
	w.notes.take()
	for _, id := range []int32{501, 502} {
		if gludio.Engrave(w.clan(rivals), id) {
			t.Fatalf("engraving %d after the end took the castle", id)
		}
	}
	expectLines(t, "engrave after end", w.notes.take(), nil)
	w.e.mu.Lock()
	left := len(gludio.engraved)
	w.e.mu.Unlock()
	if left != 0 {
		t.Fatalf("%d engravings recorded after the end", left)
	}
}

// TestArtifactResidence pins the residence a placed holy artifact is given
// as it spawns: it belongs to the castle that placed it, though its
// template is none of the castle's residence NPCs, while another NPC of
// the same template belongs to no castle.
func TestArtifactResidence(t *testing.T) {
	w := twoArtifactWorld(t)
	for _, id := range []int32{501, 502} {
		if c, ok := w.castles.ByArtifact(id); !ok || c.ID != 1 {
			t.Fatalf("ByArtifact(%d) = %v, %v; want Gludio", id, c, ok)
		}
		if c, ok := w.castles.ForNPC(id, 35322, 0, nil, nil); !ok || c.ID != 1 {
			t.Fatalf("ForNPC(placed artifact %d) = %v, %v; want Gludio", id, c, ok)
		}
	}
	if c, ok := w.castles.ByArtifact(503); ok {
		t.Fatalf("ByArtifact(another copy) = %v; want none", c)
	}
	if c, ok := w.castles.ForNPC(503, 35322, 0, nil, nil); ok {
		t.Fatalf("ForNPC(another copy of the artifact) = %v; want none", c)
	}
	if c, ok := w.castles.ByNPC(35322); ok {
		t.Fatalf("ByNPC(artifact template) = %v; want none", c)
	}
}
