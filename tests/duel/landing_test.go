package duel

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestPartyDuelArenaJumpWaitsForTheLanding pins the owner-approved retry
// of the party duel's arena jump refused while a member's own teleport
// lands (#5121): Mate's recall by a GM is in flight when the duel is
// accepted, so Mate's jump to the arena is held while the others go; once
// Mate's client reports the recall landed, Mate is moved onto its arena
// spot.
func TestPartyDuelArenaJumpWaitsForTheLanding(t *testing.T) {
	t.Parallel()
	data, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	a := bootArenaWith(t, []gameservertest.Option{gameservertest.WithAdmin(data)}, func(a *arena) {
		gmID := a.players[4].id
		if _, err := a.srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE obj_Id = ?", gmID); err != nil {
			t.Fatalf("set access level: %v", err)
		}
	}, "Lead", "Mate", "Foe", "Ally", "Gm")
	lead, mate, foe, ally, gm := a.players[0], a.players[1], a.players[2], a.players[3], a.players[4]
	a.party(t, 0, 1)
	a.party(t, 2, 3)

	recall := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	recall.WriteString("recall Mate")
	gm.c.Send(recall.Bytes())
	a.srv.AdvanceUntil(t, "Mate's recall in flight", func() bool {
		obj, ok := a.srv.State.Player(mate.id)
		return ok && obj.(interface{ Teleporting() bool }).Teleporting()
	})
	a.quiet(t)
	rx, ry, _ := a.srv.PlayerPosition(t, mate.id)

	lead.c.Send(encodeDuelStart(ally.name, true))
	a.quiet(t)
	foe.c.Send(encodeDuelAnswer(true, true))
	leadX, leadY := duel.Arena.X-180, duel.Arena.Y-150
	a.srv.AdvanceUntil(t, "Lead moved to the arena", func() bool {
		x, y, _ := a.srv.PlayerPosition(t, lead.id)
		return x == leadX && y == leadY
	})
	a.quiet(t)
	if x, y, _ := a.srv.PlayerPosition(t, mate.id); x != rx || y != ry || !a.standing(t, 1).InDuel() {
		t.Fatalf("Mate at %d,%d in duel=%v, want still landing at %d,%d, in the duel", x, y, a.standing(t, 1).InDuel(), rx, ry)
	}

	mate.c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	mateX, mateY := duel.Arena.X+40-180, duel.Arena.Y-150
	a.srv.AdvanceUntil(t, "Mate moved to the arena", func() bool {
		x, y, _ := a.srv.PlayerPosition(t, mate.id)
		return x == mateX && y == mateY
	})
}
