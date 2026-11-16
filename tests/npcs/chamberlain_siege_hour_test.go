package npcs

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestChamberlainSiegeHourEarlierMovesTheRegistrationEnd moves Gludio's
// siege from 18:00 to 16:00 through the chamberlain, its registrations
// open: they stay open until a day before the new hour and close then,
// telling the lord REGISTRATION_TERM_FOR_S1_ENDED and dropping the clan
// waiting to defend; a registration then is refused with
// DEADLINE_FOR_SIEGE_S1_PASSED.
func TestChamberlainSiegeHourEarlierMovesTheRegistrationEnd(t *testing.T) {
	t.Parallel()
	checkSiegeHourMove(t, "siege_time_set 1 4", 16)
}

// TestChamberlainSiegeHourLaterMovesTheRegistrationEnd is the move to
// 20:00: the registrations stay open past the old end, until a day before
// the new hour.
func TestChamberlainSiegeHourLaterMovesTheRegistrationEnd(t *testing.T) {
	t.Parallel()
	checkSiegeHourMove(t, "siege_time_set 1 8", 20)
}

// checkSiegeHourMove boots Gludio's siege two days away at 18:00, its
// registrations open and Rivals waiting to defend, sends command to the
// chamberlain and checks the registrations close a day before the siege's
// new hour.
func checkSiegeHourMove(t *testing.T, command string, hour int) {
	t.Helper()
	day := time.Now().AddDate(0, 0, 2)
	siegeDate := time.Date(day.Year(), day.Month(), day.Day(), 18, 0, 0, 0, time.Local)
	w := bootChamberlain(t, commandSetup{extra: []gameservertest.Option{
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seedResidenceChat(t, db, residenceSetup{})
			for _, q := range []struct {
				query string
				args  []any
			}{
				{"UPDATE castle SET siegeDate = ?, regTimeOver = 'false' WHERE id = ?", []any{siegeDate.UnixMilli(), taxCastleID}},
				{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, 'PENDING')", []any{taxCastleID, residenceRivalClan}},
			} {
				if _, err := db.ExecContext(context.Background(), q.query, q.args...); err != nil {
					t.Fatalf("seed siege: %v", err)
				}
			}
		}),
	}})
	if !w.srv.DrivesClock() {
		t.Skip("the registrations end days away: it needs the driven clock")
	}
	gludio, ok := w.srv.Sieges.Get(taxCastleID)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	join := func() []int32 {
		t.Helper()
		p := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinSiege)
		p.WriteInt32(taxCastleID)
		p.WriteInt32(1)
		p.WriteInt32(1)
		w.c.Send(p.Bytes())
		return messages(drainFrames(t, w.c))
	}

	// The calendar's step a second after boot arms the registrations' end,
	// a day before 18:00.
	w.srv.Advance(t, time.Second)
	assertAnswer(t, w.run(t, command), pageThenFinal, w.f, w.filledFile(t, "siegetime8.htm"))
	moved := time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, time.Local).UnixMilli()
	if _, _, _, date, over := w.castleRow(t); date != moved || over != "true" {
		t.Fatalf("stored siege date %d over %q, want %d true", date, over, moved)
	}
	deadline := time.UnixMilli(moved - (24 * time.Hour).Milliseconds())

	w.srv.Advance(t, deadline.Add(-time.Minute).Sub(w.srv.Sieges.Now()))
	if st, side := gludio.Status(), gludio.Side(residenceRivalClan); st != siege.StatusRegistrationOpened || side != siege.SidePending {
		t.Fatalf("a minute before the new end: status %s, Rivals %s; want REGISTRATION_OPENED, PENDING", st, side)
	}
	// The lord's clan may not attack its own castle: past the open
	// registrations, it is told it defends.
	if got := join(); !slices.Equal(got, []int32{siege.MsgOwnerAutomaticallyDefends}) {
		t.Fatalf("registration a minute before the new end told %v, want %d", got, siege.MsgOwnerAutomaticallyDefends)
	}

	w.srv.Advance(t, 2*time.Minute)
	if got := messages(drainFrames(t, w.c)); !slices.Equal(got, []int32{siege.MsgRegistrationTermEnded}) {
		t.Fatalf("the lord told %v at the new end, want REGISTRATION_TERM_FOR_S1_ENDED", got)
	}
	if st, side := gludio.Status(), gludio.Side(residenceRivalClan); st != siege.StatusRegistrationOver || side != siege.SideNone {
		t.Fatalf("a minute past the new end: status %s, Rivals %s; want REGISTRATION_OVER, none", st, side)
	}
	if got := join(); !slices.Equal(got, []int32{siege.MsgDeadlinePassed}) {
		t.Fatalf("registration past the new end told %v, want %d", got, siege.MsgDeadlinePassed)
	}
	w.srv.FlushPersistence(t)
	var pending int
	if err := w.srv.DB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM siege_clans WHERE castle_id = ? AND type = 'PENDING'", taxCastleID).Scan(&pending); err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if pending != 0 {
		t.Fatalf("stored pending defenders = %d, want none", pending)
	}
}
