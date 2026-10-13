package items

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: MercenaryTickets.useItem gates a ticket's use, in order: no
// castle's siege zone under the player (silent), not that castle's lord
// (YOU_DO_NOT_HAVE_AUTHORITY_TO_POSITION_MERCENARIES), not one of its
// tickets (MERCENARIES_CANNOT_BE_POSITIONED_HERE), its siege in progress
// (THIS_MERCENARY_CANNOT_BE_POSITIONED_ANYMORE), not the seal validation
// period (MERC_CAN_BE_ASSIGNED), the Seal of Strife owner not among the
// ticket's ssq (MERC_CANT_BE_ASSIGNED_USING_STRIFE), its max amount already
// dropped (THIS_MERCENARY_CANNOT_BE_POSITIONED_ANYMORE), another ticket
// within 25 (POSITIONING_CANNOT_BE_DONE_BECAUSE_DISTANCE_BETWEEN_MERCENARIES_TOO_SHORT);
// then one ticket drops at the player's feet, the castle counts it
// (Castle.addDroppedTicket) and PLACE_S1_IN_CURRENT_LOCATION_AND_DIRECTION
// names it. ItemInstance.onAction lets only the castle's lord, out of a
// party, pick a ticket up (THIS_IS_NOT_A_MERCENARY_OF_A_CASTLE_THAT_YOU_OWN
// and ActionFailed); pickupMe uncounts it; Castle.removeOwner decays them;
// ItemsOnGroundTaskManager keeps them (destroy time 0) and restores them to
// their castle at boot.

const (
	gludioSwordTicket  int32 = 6038 // Gludio, max 2, NORMAL;DAWN
	gludioClericTicket int32 = 6041 // Gludio, max 1, NORMAL;DAWN
	gludioDuskTicket   int32 = 6175 // Gludio, max 2, DUSK
	dionSwordTicket    int32 = 6051 // Dion only
)

// ticketSetup shapes the ticket world: noField leaves Gludio without a siege
// zone, period and strife set the Seven Signs (the seal validation period
// and no Strife owner by default), and attacker registers a rival clan to
// attack Gludio.
type ticketSetup struct {
	noField    bool
	competing  bool
	strife     sevensigns.Cabal
	attacker   bool
	ownsCastle int
}

// ticketWorldOptions are the boot options a ticket world adds to the castle
// world: the shipped tickets, the sieges and Gludio's battlefield over the
// whole world.
func ticketWorldOptions(t *testing.T, s ticketSetup) []gameservertest.Option {
	t.Helper()
	_, shippedItems := shippedData()
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{circletOfGludioID, lordsCrownID, gludioSwordTicket, gludioClericTicket, gludioDuskTicket, dionSwordTicket} {
		tmpl, ok := shippedItems.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	opts := []gameservertest.Option{
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithSieges(siege.DefaultConfig()),
	}
	if !s.noField {
		form, err := zone.NewCuboid(-300_000, 300_000, -300_000, 300_000, -20_000, 20_000)
		if err != nil {
			t.Fatal(err)
		}
		field := zone.NewSiege(1, form)
		field.ResidenceID = 1
		zones := zone.NewIndex()
		zones.Add(field)
		opts = append(opts, gameservertest.WithZones(zones))
	}
	return opts
}

func bootTicketWorld(t *testing.T, s ticketSetup, extra ...gameservertest.Option) *castleWorld {
	t.Helper()
	datapack.Require(t)
	opts := append(append(ticketWorldOptions(t, s), extra...),
		gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) {
			ctx := context.Background()
			row, found, err := store.LoadStatus(ctx)
			if err != nil || !found {
				t.Fatalf("load seven signs status: found=%v err=%v", found, err)
			}
			row.Period = sevensigns.SealValidation
			if s.competing {
				row.Period = sevensigns.Competition
			}
			row.SealOwners[2] = s.strife
			if err := store.SaveStatus(ctx, row); err != nil {
				t.Fatalf("seed seven signs status: %v", err)
			}
		}))
	w := bootCastleWorld(t, s.ownsCastle, func(db *sql.DB) {
		if !s.attacker {
			return
		}
		for _, q := range []struct {
			q    string
			args []any
		}{
			{"INSERT INTO characters (account_name, obj_Id, char_name, level, clanid) VALUES ('rival', ?, 'Red', 40, ?)", []any{siegeRivalLeaderID, siegeRivalClanID}},
			{"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (?, 'Rivals', 5, ?)", []any{siegeRivalClanID, siegeRivalLeaderID}},
			{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, ?, 'ATTACKER')", []any{siegeRivalClanID}},
		} {
			if _, err := db.ExecContext(context.Background(), q.q, q.args...); err != nil {
				t.Fatalf("seed rival clan: %v", err)
			}
		}
	}, opts...)
	return w
}

// useTicket has the leader use its ticket objectID and returns what it
// was sent.
func (w *castleWorld) useTicket(t *testing.T, objectID int32) [][]byte {
	t.Helper()
	w.leader.Send(encodeUseItem(objectID, false))
	return collectFrames(w.leader)
}

// ticketMessages returns the ids of the SystemMessage frames among frames.
func ticketMessages(t *testing.T, frames [][]byte) []int {
	t.Helper()
	var ids []int
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			ids = append(ids, systemMessageID(t, f))
		}
	}
	return ids
}

// droppedGround returns the object id and item id of the DropItem among
// frames.
func droppedGround(frames [][]byte) (objectID, itemID int32, ok bool) {
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeDropItem {
			continue
		}
		r := wire.NewReader(f[1:])
		r.ReadInt32() // dropper
		return r.ReadInt32(), r.ReadInt32(), true
	}
	return 0, 0, false
}

// gludioDropped is how many tickets Gludio Castle counts on its grounds.
func gludioDropped(t *testing.T, srv *gameservertest.Server) int {
	t.Helper()
	c, ok := srv.Castles.Get(1)
	if !ok {
		t.Fatal("no Gludio Castle")
	}
	return c.DroppedTickets()
}

// TestMercenaryTicketRefusals drives each gate of a ticket's use: each
// answers its message alone (none outside a castle) and drops nothing.
func TestMercenaryTicketRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		setup  ticketSetup
		ticket int32
		siege  bool
		want   []int
	}{
		{"outside any castle", ticketSetup{noField: true, ownsCastle: 1}, gludioSwordTicket, false, nil},
		{"not the lord", ticketSetup{}, gludioSwordTicket, false, []int{serverpackets.SystemMessageNoAuthorityToPositionMercenaries}},
		{"another castle's ticket", ticketSetup{ownsCastle: 1}, dionSwordTicket, false, []int{serverpackets.SystemMessageMercenariesCannotBePositionedHere}},
		{"siege in progress", ticketSetup{ownsCastle: 1, attacker: true}, gludioSwordTicket, true, []int{serverpackets.SystemMessageMercenaryCannotBePositionedAnymore}},
		{"not seal validation", ticketSetup{ownsCastle: 1, competing: true}, gludioSwordTicket, false, []int{serverpackets.SystemMessageMercCanBeAssigned}},
		{"strife owned by dusk", ticketSetup{ownsCastle: 1, strife: sevensigns.Dusk}, gludioSwordTicket, false, []int{serverpackets.SystemMessageMercCantBeAssignedUsingStrife}},
		{"dusk ticket, strife unowned", ticketSetup{ownsCastle: 1}, gludioDuskTicket, false, []int{serverpackets.SystemMessageMercCantBeAssignedUsingStrife}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootTicketWorld(t, tc.setup)
			ticket := w.srv.GiveItem(t, w.leaderID, tc.ticket, 2)
			w.enter(t)
			if tc.siege {
				gludio, ok := w.srv.Sieges.Get(1)
				if !ok {
					t.Fatal("no Gludio siege")
				}
				gludio.Start()
				if !gludio.InProgress() {
					t.Fatal("siege not under way")
				}
				drainUntilQuiet(t, w.leader)
				drainUntilQuiet(t, w.gm)
			}
			frames := w.useTicket(t, ticket)
			if got := ticketMessages(t, frames); !slices.Equal(got, tc.want) || len(frames) != len(tc.want) {
				t.Fatalf("answer: messages %v among opcodes %x, want messages %v alone", got, opcodesOf(frames), tc.want)
			}
			if w.srv.GroundItems.Len() != 0 {
				t.Fatalf("ground items after a refusal = %d, want 0", w.srv.GroundItems.Len())
			}
			w.srv.FlushItems(t)
			if inst := mustFindItem(t, w.srv, w.leaderID, ticket); inst.Count != 2 {
				t.Fatalf("ticket count after a refusal = %d, want 2", inst.Count)
			}
		})
	}
}

// TestMercenaryTicketPlacement places Gludio tickets as its lord: one of a
// stack drops at the lord's feet and is named; the castle counts it, so
// the cleric ticket's single slot is then taken and another sword ticket
// on the same spot is too close. Another player may not pick a ticket up;
// the lord picks one up, which uncounts it. Losing the castle takes the
// last ticket out of the world. The admin castle page reads the count.
func TestMercenaryTicketPlacement(t *testing.T) {
	t.Parallel()
	w := bootTicketWorld(t, ticketSetup{ownsCastle: 1})
	sword := w.srv.GiveItem(t, w.leaderID, gludioSwordTicket, 3)
	cleric := w.srv.GiveItem(t, w.leaderID, gludioClericTicket, 2)
	w.enter(t)

	frames := w.useTicket(t, cleric)
	clericGround, itemID, ok := droppedGround(frames)
	if !ok || itemID != gludioClericTicket {
		t.Fatalf("cleric ticket use: DropItem %v item %d among opcodes %x", ok, itemID, opcodesOf(frames))
	}
	var placed bool
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			assertSystemMessageItem(t, f, serverpackets.SystemMessagePlaceS1InCurrentLocationAndDirection, gludioClericTicket)
			placed = true
		}
	}
	if !placed {
		t.Fatalf("cleric ticket use named nothing among opcodes %x", opcodesOf(frames))
	}
	if got := gludioDropped(t, w.srv); got != 1 {
		t.Fatalf("Gludio dropped tickets = %d, want 1", got)
	}
	w.srv.FlushItems(t)
	if inst := mustFindItem(t, w.srv, w.leaderID, cleric); inst.Count != 1 {
		t.Fatalf("cleric tickets held after placing = %d, want 1", inst.Count)
	}

	if got := ticketMessages(t, w.useTicket(t, cleric)); !slices.Equal(got, []int{serverpackets.SystemMessageMercenaryCannotBePositionedAnymore}) {
		t.Fatalf("second cleric ticket told %v, want max amount", got)
	}
	if got := ticketMessages(t, w.useTicket(t, sword)); !slices.Equal(got, []int{serverpackets.SystemMessageMercenariesTooCloseToPosition}) {
		t.Fatalf("sword ticket on the cleric's spot told %v, want too close", got)
	}
	if got := gludioDropped(t, w.srv); got != 1 {
		t.Fatalf("Gludio dropped tickets after refusals = %d, want 1", got)
	}

	drainUntilQuiet(t, w.gm) // the ticket falling
	gm, _ := w.castleCommand(t, "gludio_castle")
	if _, page, _ := gmAnswer(t, gm); !strings.Contains(page, "Dropped tickets: 1<") {
		t.Fatalf("castle page with one ticket: %q", page)
	}

	// The game master, no lord of Gludio, may not pick the ticket up.
	w.gm.Send(encodeAction(clericGround, 0, 0, 0, false))
	gmFrames := collectFrames(w.gm)
	if len(gmFrames) != 2 || systemMessageID(t, gmFrames[0]) != serverpackets.SystemMessageNotMercenaryOfOwnCastleCannotCancelPositioning || gmFrames[1][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("game master's pickup: opcodes %x, want the not-your-mercenary message then ActionFailed", opcodesOf(gmFrames))
	}

	// The lord picks it up: it no longer counts.
	w.leader.Send(encodeAction(clericGround, 0, 0, 0, false))
	drainUntilQuiet(t, w.leader)
	drainUntilQuiet(t, w.gm)
	if got := gludioDropped(t, w.srv); got != 0 {
		t.Fatalf("Gludio dropped tickets after the pickup = %d, want 0", got)
	}
	w.srv.FlushItems(t)
	if inst := mustFindItem(t, w.srv, w.leaderID, cleric); inst.Count != 2 {
		t.Fatalf("cleric tickets held after the pickup = %d, want 2", inst.Count)
	}

	frames = w.useTicket(t, sword)
	swordGround, _, ok := droppedGround(frames)
	if !ok {
		t.Fatalf("sword ticket use: opcodes %x, want a drop", opcodesOf(frames))
	}
	drainUntilQuiet(t, w.gm)

	_, leader := w.castleCommand(t, "remove gludio_castle")
	var deleted bool
	for _, f := range leader {
		if f[0] == serverpackets.OpcodeDeleteObject && wire.NewReader(f[1:]).ReadInt32() == swordGround {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("leader after the castle's removal: opcodes %x, want the ticket deleted", opcodesOf(leader))
	}
	if _, ok := w.srv.State.Object(swordGround); ok {
		t.Fatal("ticket still in the world after the castle's removal")
	}
	if got := gludioDropped(t, w.srv); got != 0 {
		t.Fatalf("Gludio dropped tickets after the castle's removal = %d, want 0", got)
	}
}

// TestMercenaryTicketSurvivesRestart places a ticket, shuts the server
// down and boots again: the ticket was saved never to expire, and comes
// back on Gludio's grounds, counted there.
func TestMercenaryTicketSurvivesRestart(t *testing.T) {
	t.Parallel()
	w := bootTicketWorld(t, ticketSetup{ownsCastle: 1})
	sword := w.srv.GiveItem(t, w.leaderID, gludioSwordTicket, 1)
	w.enter(t)
	ground, _, ok := droppedGround(w.useTicket(t, sword))
	if !ok {
		t.Fatal("no ticket placed")
	}
	w.srv.Shutdown(t)

	var left sql.NullInt64
	if err := w.srv.DB.QueryRowContext(context.Background(), "SELECT time FROM items_on_ground WHERE object_id = ?", ground).Scan(&left); err != nil || left.Int64 != 0 {
		t.Fatalf("saved ticket time = %v (%v), want 0", left, err)
	}

	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	srv := gameservertest.Boot(t, append(ticketWorldOptions(t, ticketSetup{}),
		gameservertest.WithWantChars(1),
		gameservertest.WithCastles(castles),
	)...)
	if _, ok := srv.State.Object(ground); !ok {
		t.Fatal("ticket not restored")
	}
	if got := gludioDropped(t, srv); got != 1 {
		t.Fatalf("Gludio dropped tickets after the restart = %d, want 1", got)
	}
}
