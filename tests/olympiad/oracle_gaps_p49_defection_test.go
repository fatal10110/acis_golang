package olympiad

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// told reports whether p was sent the system message id, whatever its
// parameters.
func (p *competitor) told(id int32) bool {
	for _, f := range p.frames {
		if f[0] == serverpackets.OpcodeSystemMessage && wire.NewReader(f[1:]).ReadInt32() == id {
			return true
		}
	}
	return false
}

// TestOlympiadMatchUnfitCompetitorDefects pins a competitor that can no
// longer compete as the stadium opens (AbstractOlympiadGame.checkDefection,
// AbstractOlympiadGame.java:205-246): one that is dead, plays a subclass,
// carries 80% of its inventory or is watching a match is told why, as the
// reference words it, except an observer, which is told nothing of its own.
// Its opponent is told the game was cancelled for an unfit opponent, the
// defector loses a third of its points, and the match counts for no one;
// neither is brought in.
func TestOlympiadMatchUnfitCompetitorDefects(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		opts    []gameservertest.Option
		prepare func(t *testing.T, srv *gameservertest.Server)
		unfit   func(t *testing.T, m *match, p *competitor)
		own     int32
	}{
		{
			name: "dead", own: serverpackets.SystemMessageCannotJoinOlympiadWhileDead,
			unfit: func(t *testing.T, m *match, p *competitor) { m.srv.MarkPlayerDead(t, p.id) },
		},
		{
			name: "subclass", own: serverpackets.SystemMessageSubJobCannotJoinOlympiad,
			opts: []gameservertest.Option{gameservertest.WithClassTemplates(func() *player.Template {
				mystic := gameservertest.ClassTemplate()
				mystic.ID = humanMystic
				return mystic
			}())},
			prepare: func(t *testing.T, srv *gameservertest.Server) {
				for _, q := range []string{
					`UPDATE characters SET classid = ? WHERE char_name = 'Bravo'`,
					`INSERT INTO character_subclasses (char_obj_id, class_id, exp, sp, level, class_index)
						SELECT obj_Id, ?, 0, 0, 40, 1 FROM characters WHERE char_name = 'Bravo'`,
				} {
					if _, err := srv.DB.Exec(q, humanMystic); err != nil {
						t.Fatalf("make Bravo play a subclass: %v", err)
					}
				}
			},
		},
		{
			name: "overweight", own: serverpackets.SystemMessageInventoryTooFullForOlympiad,
			prepare: func(t *testing.T, srv *gameservertest.Server) {
				// A character holds 80 items; 64 of them are the 80% the
				// reference's check refuses.
				var tmplID int32
				for _, tmpl := range gameservertest.ItemTemplates().All() {
					if !tmpl.Stackable && tmpl.Weight == 0 {
						tmplID = tmpl.ID
						break
					}
				}
				if tmplID == 0 {
					t.Fatal("no weightless, unstackable item template")
				}
				var id int32
				if err := srv.DB.QueryRow(`SELECT obj_Id FROM characters WHERE char_name = 'Bravo'`).Scan(&id); err != nil {
					t.Fatalf("find Bravo: %v", err)
				}
				for range 64 {
					srv.GiveItem(t, id, tmplID, 1)
				}
			},
		},
		{
			name: "observer",
			unfit: func(t *testing.T, m *match, p *competitor) {
				m.onCharacter(t, p, func(pc *player.Character) { pc.SetSavedLocation(pc.CurrentLocation()) })
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var prepare func(*gameservertest.Server)
			if tc.prepare != nil {
				prepare = func(srv *gameservertest.Server) { tc.prepare(t, srv) }
			}
			m := bootMatchPrepared(t, nil, prepare, tc.opts...)
			a, b := m.ones[0], m.ones[1]
			if tc.unfit != nil {
				tc.unfit(t, m, b)
			}
			for _, p := range m.ones {
				p.frames = nil
			}

			m.drawn(t)
			m.over(t)

			if want := lines(enterCountdown, "SM 1494", "SM 1658 Bravo 6", townCountdown, "Mode 0"); !slices.Equal(a.course(), want) {
				t.Errorf("Alpha course:\n got %q\nwant %q", a.course(), want)
			}
			if want := lines(enterCountdown, "SM 1658 Bravo 6", townCountdown); !slices.Equal(b.course(), want) {
				t.Errorf("Bravo course:\n got %q\nwant %q", b.course(), want)
			}
			for _, id := range []int32{
				serverpackets.SystemMessageCannotJoinOlympiadWhileDead,
				serverpackets.SystemMessageSubJobCannotJoinOlympiad,
				serverpackets.SystemMessageInventoryTooFullForOlympiad,
			} {
				if got, want := b.told(id), id == tc.own; got != want {
					t.Errorf("Bravo told %d = %v, want %v", id, got, want)
				}
				if a.told(id) {
					t.Errorf("Alpha was told %d, Bravo's reason", id)
				}
			}
			if n, _ := m.srv.Olympiad.Noble(a.id); n.Points != 18 || n.CompDone != 0 {
				t.Errorf("Alpha record = %+v, want 18 points and no match", n)
			}
			if n, _ := m.srv.Olympiad.Noble(b.id); n.Points != 12 || n.CompDone != 0 {
				t.Errorf("Bravo record = %+v, want 12 points and no match", n)
			}
			if got := matchFights(t, m); len(got) != 0 {
				t.Fatalf("olympiad_fights = %+v, want none", got)
			}
		})
	}
}
