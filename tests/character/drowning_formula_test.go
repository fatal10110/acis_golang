package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestDrowningStartsWhenTheBreathRunsOut pins the breath length of a swimmer:
// 60000 ms times the race multiplier (Stats.BREATH of the character,
// unmodified here), so a human drowns on the tick at the 60 s mark and not
// before; the DrownDamage message carries one number. The damage amount is
// pinned by the network package's drown damage unit test.
func TestDrowningStartsWhenTheBreathRunsOut(t *testing.T) {
	t.Parallel()
	srv, character, _, clock := bootInWater(t)
	if character.Race != player.RaceHuman {
		t.Fatalf("test character is race %v, want human", character.Race)
	}
	const breath = 60_000 * time.Millisecond

	clock.advance(breath - time.Millisecond)
	srv.Water.Tick()
	if i := drownIndex(srv.ReadQueued(t, srv.Client)); i >= 0 {
		t.Fatal("drowning damage before the 60 s breath ran out")
	}

	clock.advance(time.Millisecond)
	srv.Water.Tick()
	frames := srv.ReadQueued(t, srv.Client)
	i := drownIndex(frames)
	if i < 0 {
		t.Fatal("no drowning damage once the 60 s breath ran out")
	}
	r := wire.NewReader(frames[i][1:])
	r.ReadInt32() // message id
	if params, kind := r.ReadInt32(), r.ReadInt32(); params != 1 || kind != serverpackets.SystemMessageParamNumber {
		t.Fatalf("DrownDamage parameters = %d of kind %d, want one number", params, kind)
	}
}
