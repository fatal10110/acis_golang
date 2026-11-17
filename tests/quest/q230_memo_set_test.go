package quest

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	quest001 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q230"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	galateaID       = 30634
	galateaLetterID = 3352
	dimensionalDiam = 7562
)

// darkWizardClassTemplates are the class templates of the dark wizard and
// its parents, so a character of that class can enter the world.
func darkWizardClassTemplates() []*player.Template {
	var classes []*player.Template
	for id, ok := 39, true; ok && id > 0; id, ok = player.ClassParent(id) {
		tmpl := gameservertest.ClassTemplate()
		tmpl.ID = id
		classes = append(classes, tmpl)
	}
	return classes
}

// TestQ230StartWithTheDiamondsAlreadyPaid: the character holds the
// secondClassChange39 memo, so Galatea's start gives the letter and the plain
// accept page, no Dimensional Diamonds and no memo write.
func TestQ230StartWithTheDiamondsAlreadyPaid(t *testing.T) {
	t.Parallel()
	pages := chatPages(questPages(q230Name, map[string][]string{
		"30634-01.htm": nil, "30634-03.htm": {"30634-08.htm"}, "30634-08.htm": nil, "30634-08a.htm": nil,
	}), galateaID)
	w := bootSeamQuest(t, 39, pages, func(srv *gameservertest.Server, objID int32) {
		if _, err := srv.DB.ExecContext(context.Background(),
			"UPDATE characters SET classid = ?, base_class = ? WHERE obj_Id = ?", 39, 39, objID); err != nil {
			t.Fatal(err)
		}
		if _, err := srv.DB.ExecContext(context.Background(),
			"INSERT INTO character_memo (charId,var,val) VALUES (?,'secondClassChange39','true')", objID); err != nil {
			t.Fatal(err)
		}
	}, q230ItemTemplates(), gameservertest.WithClassTemplates(darkWizardClassTemplates()...),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(pakoID), deathServitorTemplate()})),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{galateaID: script.KindFolk},
			[]script.Listing{{Path: "quest." + q001}, {Path: q230Path}},
			script.Catalog{"quest." + q001: quest001.New, q230Path: q230.New},
		))
	galatea := w.spawnFolk(t, "galatea", galateaID, 0, 60, 0)
	if got := w.questWindow(t, galatea.ObjectID()); !shows(got, "30634-03.htm") {
		t.Fatalf("Galatea's window = %q, want 30634-03.htm", got)
	}

	got := w.bypass(t, "Quest "+q230Name+" 30634-08.htm")
	if !shows(got, "30634-08.htm") || shows(got, "30634-08a.htm") {
		t.Fatalf("start page = %q, want the plain 30634-08.htm", got)
	}
	if n := w.srv.PlayerItemCount(t, w.player, galateaLetterID); n != 1 {
		t.Fatalf("Galatea's letter = %d, want 1", n)
	}
	if n := w.srv.PlayerItemCount(t, w.player, dimensionalDiam); n != 0 {
		t.Fatalf("Dimensional Diamonds = %d, want 0", n)
	}
	var memos int
	if err := w.srv.DB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM character_memo WHERE charId = ?", w.player).Scan(&memos); err != nil {
		t.Fatal(err)
	}
	if memos != 1 {
		t.Fatalf("memo rows = %d, want only the seeded secondClassChange39", memos)
	}
}
