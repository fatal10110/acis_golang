package npcs

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestSubclassKeepsItsLevelFloorOnExpLoss pins SubClass.setLevel's floor
// (SubClass.java:95-98): an active subclass never drops below level 40, and
// the experience a loss would leave under that level snaps back to the
// level's start. Told through the live character and the saved subclass row.
func TestSubclassKeepsItsLevelFloorOnExpLoss(t *testing.T) {
	levelRows := make(map[int]player.Level, 85)
	for lvl := 1; lvl <= 85; lvl++ {
		levelRows[lvl] = player.Level{RequiredExpToLevelUp: int64(lvl-1) * 1000}
	}
	levels, err := player.NewLevelTable(levelRows)
	if err != nil {
		t.Fatal(err)
	}
	floorExp := levels.RequiredExpForLevel(player.SubclassStartLevel)
	const startLevel = player.SubclassStartLevel + 1
	startExp := levels.RequiredExpForLevel(startLevel) + 5

	w := bootSubclassWorldWith(t, func(srv *gameservertest.Server, objID int32) {
		exec(t, srv, `INSERT INTO character_subclasses (char_obj_id, class_id, exp, sp, level, class_index) VALUES (?, ?, ?, 0, ?, 1)`, objID, spellhowler, startExp, startLevel)
		exec(t, srv, `UPDATE characters SET classid = ? WHERE obj_Id = ?`, spellhowler, objID)
	}, gameservertest.WithLevels(levels))

	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatal("player not in the world")
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%T is not an online character", obj)
	}
	var level int
	var exp int64
	onQueue := func(fn func()) {
		t.Helper()
		done := make(chan struct{})
		if !w.srv.PlayerQueue(t, w.player).Post(func() { defer close(done); fn() }) {
			t.Fatal("post: queue closed")
		}
		<-done
	}
	onQueue(func() { level, exp = c.Level(), c.Exp })
	if !c.SubclassActive() || level != startLevel || exp != startExp {
		t.Fatalf("subclass starts at level %d exp %d active=%v, want %d/%d", level, exp, c.SubclassActive(), startLevel, startExp)
	}

	// A loss deep enough to drop to level 1 stops at the floor.
	onQueue(func() {
		c.RemoveExp(levels, nil, startExp)
		level, exp = c.Level(), c.Exp
	})
	if level != player.SubclassStartLevel || exp != floorExp {
		t.Fatalf("subclass after the loss = level %d exp %d, want the floor %d/%d", level, exp, player.SubclassStartLevel, floorExp)
	}

	w.srv.FlushPersistence(t)
	w.srv.TickAutosave(t)
	w.srv.FlushPersistence(t)
	var savedLevel int
	var savedExp int64
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT level, exp FROM character_subclasses WHERE char_obj_id = ? AND class_index = 1`, w.player).Scan(&savedLevel, &savedExp); err != nil {
		t.Fatal(err)
	}
	if savedLevel != player.SubclassStartLevel || savedExp != floorExp {
		t.Fatalf("saved subclass = level %d exp %d, want %d/%d", savedLevel, savedExp, player.SubclassStartLevel, floorExp)
	}
}
