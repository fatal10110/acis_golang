package ai

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// valakasAppearingRow is a live spawn_data row of Valakas saved at db
// value 2, while it appeared, standing at its sleeping place so its
// restore is seen moving it.
func valakasAppearingRow() *spawn.State {
	row := valakasLiveRow(2)
	row.Location = location.Location{X: -105200, Y: -253104, Z: -15264}
	return row
}

// TestValakasResumesItsAppearance covers #5133: restored from its live row
// at db value 2 it appears again: it stands at 212852,-114842,-1632, 2 s
// later its first camera shot plays and 27.401 s after that its sequence
// ends at 3 with its regeneration 4691 level 1 wanted on itself. Fighting,
// 16 quiet minutes reset it to 0, and the db value 0 is saved.
func TestValakasResumesItsAppearance(t *testing.T) {
	w := bootValakas(t, valakasAppearingRow(), valakasQuietDraws)
	boss := soleBaiumNPC(t, w, valakasID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	if !handle.DBLoaded() || handle.DBValue() != 2 {
		t.Fatalf("Valakas loaded=%v value=%d, want loaded at 2", handle.DBLoaded(), handle.DBValue())
	}
	if got, want := placeOf(boss), "212852,-114842,-1632 heading 0"; got != want {
		t.Fatalf("Valakas stands at %s, want %s", got, want)
	}
	roles := map[int32]string{boss.ObjectID(): "valakas", w.player.ObjectID(): "player"}

	wantAntharasFrames(t, "2 s less 1 ms", w.advance(t, 2*time.Second-time.Millisecond), roles)
	wantAntharasFrames(t, "2 s", w.advance(t, time.Millisecond), roles, valakasShot("1800,180,-1,1500,10000,0,0,1,0"))
	w.advance(t, 27401*time.Millisecond-time.Millisecond)
	if got := handle.DBValue(); got != 2 {
		t.Fatalf("db value before the end of the cameras = %d, want 2", got)
	}
	w.advance(t, time.Millisecond)
	if got := handle.DBValue(); got != 3 {
		t.Fatalf("db value after the cameras = %d, want 3", got)
	}
	if !slices.Contains(valakasCastLevels(boss, roles), "4691-1@valakas=4e+06") {
		t.Fatalf("Valakas's cast desires = %q, want 4691 level 1 on itself with 4000000", valakasCastLevels(boss, roles))
	}

	w.advance(t, 16*time.Minute)
	if got := handle.DBValue(); got != 0 {
		t.Fatalf("db value after 16 quiet minutes = %d, want 0", got)
	}
	if got := valakasDBValue(t, w); got != 0 {
		t.Fatalf("saved db value = %d, want 0", got)
	}
}

// TestHeartOfVolcanoReopensAfterValakasResumes covers #5133 through the
// Heart: with Valakas restored at db value 2 (whose entry answers
// heart_of_volcano003.htm, TestHeartOfVolcanoRefuses), its resumed
// appearance ends in its fight and 16 quiet minutes reset it to 0; the
// entry then lets the character in and Valakas stores 1.
func TestHeartOfVolcanoReopensAfterValakasResumes(t *testing.T) {
	w := bootVolcano(t, map[string]*spawn.State{valakasRow: valakasAppearingRow()})
	if got := volcanoNow(t, w).valakasDBValue; got != 2 {
		t.Fatalf("Valakas's db value restored = %d, want 2", got)
	}
	w.advance(t, 30*time.Second)
	if got := volcanoNow(t, w).valakasDBValue; got != 3 {
		t.Fatalf("Valakas's db value after its cameras = %d, want 3", got)
	}
	w.advance(t, 16*time.Minute)
	if got := volcanoNow(t, w).valakasDBValue; got != 0 {
		t.Fatalf("Valakas's db value after 16 quiet minutes = %d, want 0", got)
	}

	frames := enterVolcano(t, w)
	if got, want := w.characterTeleports(t, frames), []string{"S TeleportToLocation obj=player loc=204187,-111864,34 fast=0"}; !slices.Equal(got, want) {
		t.Fatalf("the entry's teleports = %q, want %q", got, want)
	}
	w.appear(t)
	if got := volcanoNow(t, w); got.count != 1 || got.valakasDBValue != 1 {
		t.Fatalf("the Heart = %+v, want count 1, Valakas 1", got)
	}
}
