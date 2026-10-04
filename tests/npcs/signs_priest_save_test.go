package npcs

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// otherSigner is a second sign-up's owner, a row only the full save writes.
const otherSigner = 1999999

// markSignsRows overwrites the persisted contribution of Talker's row and
// of the other sign-up's behind the state's back, so a save shows up as the
// mark being overwritten.
func (w *folkWorld) markSignsRows(t *testing.T, mark int) {
	t.Helper()
	w.srv.FlushPersistence(t)
	if _, err := w.srv.DB.ExecContext(context.Background(), `UPDATE seven_signs SET contribution_score = ?`, mark); err != nil {
		t.Fatal(err)
	}
}

// contributionOf reads the persisted contribution of objectID's row once the
// queued saves have landed.
func (w *folkWorld) contributionOf(t *testing.T, objectID int32) int {
	t.Helper()
	w.srv.FlushPersistence(t)
	var score int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT contribution_score FROM seven_signs WHERE char_obj_id = ?`, objectID).Scan(&score); err != nil {
		t.Fatal(err)
	}
	return score
}

// unflushedCount reads the persisted count of templateID without running the
// lazy item write: only what was written eagerly shows.
func (w *folkWorld) unflushedCount(t *testing.T, templateID int32) int {
	t.Helper()
	w.srv.FlushPersistence(t)
	rows, err := w.srv.Items.ListByOwner(context.Background(), w.player)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	n := 0
	for _, row := range rows {
		if row.TemplateID == templateID {
			n += row.Count
		}
	}
	return n
}

// TestSignsPriestEmptyTurnInSavesNothing pins that a turn-in which changes
// nothing writes nothing: "21 <stone> 0", and an amount the cap leaves no
// room for, take no stone and queue no save, however often repeated. A
// turn-in that does earn points writes the talker's row alone, never another
// sign-up's.
func TestSignsPriestEmptyTurnInSavesNothing(t *testing.T) {
	t.Parallel()
	w := bootPriestWorld(t, map[int32]int32{blueStone: 5})
	setSevenSigns(t, w, sevensigns.Competition, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.Dawn)
	if _, err := w.srv.DB.ExecContext(context.Background(), `INSERT INTO seven_signs (char_obj_id, cabal, seal) VALUES (?, 'DUSK', 'GNOSIS')`, otherSigner); err != nil {
		t.Fatal(err)
	}
	if err := w.srv.SevenSigns.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	dawn := w.selectedPriest(t, "DawnPriest", dawnPriestID, 30)
	const mark = 777

	w.markSignsRows(t, mark)
	// The reference names the stone and the points even for none.
	nothing := chatWindow(destroyed(blueStone, 0), contribIncreased(0))
	for range 3 {
		assertDialog(t, "21 zero", w.ask(t, dawn, "SevenSigns 21 6360 0"), dawn, signsPage(dawn, "signs_6_dawn"), nothing...)
	}
	if got := w.contributionOf(t, w.player); got != mark {
		t.Fatalf("after 21 zero: contribution saved = %d, want the untouched %d", got, mark)
	}

	// 999 998 points leave no room for a blue stone.
	w.setSignsRow(t, "contribution_score = 999998")
	w.markSignsRows(t, mark)
	assertDialog(t, "21 capped", w.ask(t, dawn, "SevenSigns 21 6360 5"), dawn, signsPage(dawn, "signs_6_dawn"), nothing...)
	if got := w.contributionOf(t, w.player); got != mark {
		t.Fatalf("after 21 capped: contribution saved = %d, want the untouched %d", got, mark)
	}
	w.assertHeld(t, map[int32]int{blueStone: 5})

	w.setSignsRow(t, "contribution_score = 0")
	w.markSignsRows(t, mark)
	assertDialog(t, "21 one blue", w.ask(t, dawn, "SevenSigns 21 6360 1"), dawn, signsPage(dawn, "signs_6_dawn"),
		chatWindow(destroyed(blueStone, 1), contribIncreased(3))...)
	if got := w.contributionOf(t, w.player); got != 3 {
		t.Fatalf("after 21 one blue: contribution saved = %d, want 3", got)
	}
	if got := w.contributionOf(t, otherSigner); got != mark {
		t.Fatalf("after 21 one blue: other sign-up saved = %d, want the untouched %d", got, mark)
	}
}

// TestSignsPriestTurnInWritesStonesFirst pins that a turn-in's stones are
// written no later than the contribution they paid for: once the queued save
// has landed, the persisted stacks already show the stones gone, a stack
// partly taken as well as stacks taken whole, without the lazy item write
// having run.
func TestSignsPriestTurnInWritesStonesFirst(t *testing.T) {
	t.Parallel()
	w := bootPriestWorld(t, map[int32]int32{blueStone: 10, redStone: 3})
	setSevenSigns(t, w, sevensigns.Competition, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.Dawn)
	dawn := w.selectedPriest(t, "DawnPriest", dawnPriestID, 30)

	assertDialog(t, "21 four blue", w.ask(t, dawn, "SevenSigns 21 6360 4"), dawn, signsPage(dawn, "signs_6_dawn"),
		chatWindow(destroyed(blueStone, 4), contribIncreased(12))...)
	if got := w.contributionOf(t, w.player); got != 12 {
		t.Fatalf("after 21: contribution saved = %d, want 12", got)
	}
	if got := w.unflushedCount(t, blueStone); got != 6 {
		t.Fatalf("after 21: blue stones persisted = %d, want 6 written with the contribution", got)
	}

	assertDialog(t, "6 4", w.ask(t, dawn, "SevenSigns 6 4"), dawn, signsPage(dawn, "signs_6_dawn"),
		chatWindow(destroyed(redStone, 3), destroyed(blueStone, 6), contribIncreased(30+18))...)
	if got := w.contributionOf(t, w.player); got != 12+48 {
		t.Fatalf("after 6 4: contribution saved = %d, want %d", got, 12+48)
	}
	for _, id := range []int32{redStone, blueStone} {
		if got := w.unflushedCount(t, id); got != 0 {
			t.Fatalf("after 6 4: stone %d persisted = %d, want 0 written with the contribution", id, got)
		}
	}
}
