package combat

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// wholeMaxPlayer is the slice of a live player the whole-point maximum tests
// drive.
type wholeMaxPlayer interface {
	HP() float64
	MPValue() float64
	CP() float64
	MaxHPValue() float64
	MaxMPValue() float64
	MaxCPValue() float64
	HPFull() bool
	AddHP(float64) float64
	AddMP(float64) float64
	AddCP(float64) float64
	SetHP(float64)
	SetCP(float64)
	ReduceCurrentMP(int)
}

// TestPlayerRestoresClampToWholePointMaxima pins that a player's HP/MP/CP
// maxima are whole points: the reference's getMaxHp/getMaxMp/getMaxCp return
// the calculator result cast to int, and every restore clamps against that.
// The level 5 fixture fighter's CP calculates to 14.72 and its MP to 30.x, so
// a fractional clamp would let a pool the client shows as full absorb a
// sub-point restore.
func TestPlayerRestoresClampToWholePointMaxima(t *testing.T) {
	t.Parallel()
	_, hp := bootHPWriterPlayer(t)
	player := hp.(wholeMaxPlayer)

	for name, maxValue := range map[string]float64{
		"HP": player.MaxHPValue(), "MP": player.MaxMPValue(), "CP": player.MaxCPValue(),
	} {
		if maxValue != math.Trunc(maxValue) {
			t.Fatalf("max %s = %v, want a whole-point value", name, maxValue)
		}
	}

	if got := player.MaxCPValue(); got != 14 {
		t.Fatalf("max CP = %v, want 14 (14.72 truncated)", got)
	}
	player.SetCP(10)
	if got := player.AddCP(5); got != 4 {
		t.Fatalf("AddCP(5) on 10 of 14 CP = %v, want 4", got)
	}
	if got := player.CP(); got != 14 {
		t.Fatalf("CP after AddCP = %v, want 14", got)
	}
	player.SetCP(100)
	if got := player.CP(); got != 14 {
		t.Fatalf("SetCP(100) left CP %v, want the whole-point max 14", got)
	}

	// A login refill lands on the whole-point maximum, so a full pool has
	// nothing left to take.
	if got, want := player.MPValue(), player.MaxMPValue(); got != want {
		t.Fatalf("login MP = %v, want the whole-point max %v", got, want)
	}
	if got := player.AddMP(1); got != 0 {
		t.Fatalf("AddMP(1) on a full MP pool = %v, want 0", got)
	}
	// Draining MP to zero and restoring the displayed maximum fills the pool
	// exactly; the next restore finds it full.
	player.ReduceCurrentMP(int(player.MaxMPValue()) + 1)
	if got, want := player.AddMP(player.MaxMPValue()), player.MaxMPValue(); got != want {
		t.Fatalf("AddMP(max) on an empty pool = %v, want %v", got, want)
	}
	if got := player.AddMP(1); got != 0 {
		t.Fatalf("AddMP(1) after refilling to the displayed max = %v, want 0", got)
	}

	player.SetHP(player.MaxHPValue() + 50)
	if got, want := player.HP(), player.MaxHPValue(); got != want {
		t.Fatalf("SetHP(max+50) left HP %v, want the whole-point max %v", got, want)
	}
	if !player.HPFull() {
		t.Fatal("HPFull() = false at the whole-point max")
	}
	if got := player.AddHP(1); got != 0 {
		t.Fatalf("AddHP(1) on full HP = %v, want 0", got)
	}
}

// TestHostileRestoresClampToWholePointMaxima pins the same whole-point
// maxima on a monster whose template carries fractional HP/MP: it spawns at
// the truncated maxima, a set above them lands on them, and a restore at
// full applies nothing.
func TestHostileRestoresClampToWholePointMaxima(t *testing.T) {
	t.Parallel()
	srv, _ := bootHPWriterPlayer(t)
	hostile := srv.SpawnHostileNPCTemplateAt(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1,
		HPMax: 1000.6, MPMax: 50.7,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})

	maxHP, maxMP := hostile.MaxHPValue(), hostile.MaxMPValue()
	if maxHP != math.Trunc(maxHP) || maxMP != math.Trunc(maxMP) {
		t.Fatalf("max HP/MP = %v/%v, want whole-point values", maxHP, maxMP)
	}
	if got := hostile.HP(); got != maxHP {
		t.Fatalf("spawn HP = %v, want %v", got, maxHP)
	}
	if got := hostile.AddHP(1); got != 0 {
		t.Fatalf("AddHP(1) at full HP = %v, want 0", got)
	}
	if got := hostile.AddMP(1); got != 0 {
		t.Fatalf("AddMP(1) at full MP = %v, want 0", got)
	}
	hostile.SetHP(maxHP + 50)
	if got := hostile.HP(); got != maxHP {
		t.Fatalf("SetHP(max+50) left HP %v, want %v", got, maxHP)
	}
}
