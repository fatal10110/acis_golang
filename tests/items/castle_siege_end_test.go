package items

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/hero"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The siege end's reputation messages.
const (
	clanDefeatedLostReputationMsg     = 1772 // CLAN_WAS_DEFEATED_IN_SIEGE_AND_LOST_S1_REPUTATION_POINTS
	clanVictoriousGainedReputationMsg = 1773 // CLAN_VICTORIOUS_IN_SIEGE_AND_GAINED_S1_REPUTATION_POINTS

	lordsStartReputation = 5000
)

// handover is Gludio Castle's siege run to its end: the Lords owned the
// castle, the Rivals took it while the siege was under way, and the leaders
// of both clans are in the world.
type handover struct {
	w      *castleWorld
	gludio *siege.Siege
	rival  *testsupport.ScriptedClient
	// lords and rivals are the frames each clan's leader got from the siege
	// ending on.
	lords, rivals [][]byte
}

// runSiegeEnd boots Gludio with the Lords owning it at lordsStartReputation
// and the Rivals, a clan of no reputation attacking its siege, whose leader
// is a noble when rivalNoble. With transfer the Rivals are given the castle
// under way; the siege is then ended and the frames each leader gets from
// the end are returned. The Lords' leader is a noble in either case.
func runSiegeEnd(t *testing.T, transfer, rivalNoble bool) *handover {
	t.Helper()
	w := bootCastleWorld(t, 1, func(db *sql.DB) {
		seedRivalClan(t, db)
		for _, q := range []struct {
			query string
			args  []any
		}{
			{"UPDATE clan_data SET reputation_score = ? WHERE clan_id = ?", []any{lordsStartReputation, castleClanID}},
			{"UPDATE characters SET nobless = 1 WHERE obj_Id = ?", []any{newbieID(t, db)}},
			{"UPDATE characters SET nobless = ? WHERE obj_Id = ?", []any{rivalNoble, rivalID}},
			{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, ?, 'ATTACKER')", []any{rivalClanID}},
		} {
			if _, err := db.ExecContext(context.Background(), q.query, q.args...); err != nil {
				t.Fatalf("seed siege end: %v", err)
			}
		}
	}, gameservertest.WithSieges(siege.DefaultConfig()))
	w.enter(t)
	rival := w.srv.DialClient(t, "rival", 1)
	enterAsClanMember(rival)
	for _, c := range []*testsupport.ScriptedClient{rival, w.leader, w.gm} {
		drainUntilQuiet(t, c)
	}

	gludio, ok := w.srv.Sieges.Get(1)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	gludio.Start()
	if !gludio.InProgress() {
		t.Fatal("siege not under way")
	}
	for _, c := range []*testsupport.ScriptedClient{rival, w.leader, w.gm} {
		drainUntilQuiet(t, c)
	}
	if transfer {
		w.gm.Send(encodeAction(rivalID, 0, 0, 0, false))
		drainUntilQuiet(t, w.gm)
		w.castleCommand(t, "set gludio_castle")
		for _, c := range []*testsupport.ScriptedClient{rival, w.leader, w.gm} {
			drainUntilQuiet(t, c)
		}
	}
	gludio.End()
	if gludio.InProgress() {
		t.Fatal("the siege did not end")
	}
	return &handover{w: w, gludio: gludio, rival: rival, lords: collectFrames(w.leader), rivals: collectFrames(rival)}
}

// newbieID is Newbie's object id, read from the seeded rows (the world is
// not booted while its rows are seeded).
func newbieID(t *testing.T, db *sql.DB) int32 {
	t.Helper()
	var id int32
	if err := db.QueryRowContext(context.Background(), "SELECT obj_Id FROM characters WHERE char_name = 'Newbie'").Scan(&id); err != nil {
		t.Fatalf("Newbie's object id: %v", err)
	}
	return id
}

// reputationTrail is the clan header scores and reputation messages among
// frames, in order, as "score N" and "msg ID/points" entries.
func reputationTrail(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var trail []string
	for _, f := range frames {
		r := wire.NewReader(f[1:])
		switch f[0] {
		case serverpackets.OpcodePledgeShowInfoUpdate:
			r.ReadInt32() // clan
			r.ReadInt32() // crest
			r.ReadInt32() // level
			r.ReadInt32() // castle
			r.ReadInt32() // hall
			r.ReadInt32() // rank
			trail = append(trail, fmt.Sprintf("score %d", r.ReadInt32()))
		case serverpackets.OpcodeSystemMessage:
			id := r.ReadInt32()
			if id != clanDefeatedLostReputationMsg && id != clanVictoriousGainedReputationMsg {
				continue
			}
			if n := r.ReadInt32(); n != 1 {
				t.Fatalf("message %d has %d params, want 1", id, n)
			}
			if kind := r.ReadInt32(); kind != 1 {
				t.Fatalf("message %d param type = %d, want a number", id, kind)
			}
			trail = append(trail, fmt.Sprintf("msg %d/%d", id, r.ReadInt32()))
		}
	}
	return trail
}

func wantTrail(t *testing.T, who string, frames [][]byte, want ...string) {
	t.Helper()
	got := reputationTrail(t, frames)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("%s: reputation frames %q, want %q (opcodes %x)", who, got, want, opcodesOf(frames))
	}
}

// TestSiegeEndMovesTheClansReputation ends Gludio's siege on both outcomes
// of its owner. The Lords, who lose the castle to the Rivals, lose 1000
// reputation points and the Rivals, who take it, gain 1000: each clan's
// members in the world see the clan header with the new score, then
// CLAN_WAS_DEFEATED_IN_SIEGE_AND_LOST_S1_REPUTATION_POINTS or
// CLAN_VICTORIOUS_IN_SIEGE_AND_GAINED_S1_REPUTATION_POINTS naming the
// points. The Lords holding the castle gain 500 and the attackers nothing.
// The scores are stored.
func TestSiegeEndMovesTheClansReputation(t *testing.T) {
	t.Parallel()
	t.Run("castle changes hands", func(t *testing.T) {
		t.Parallel()
		h := runSiegeEnd(t, true, false)
		wantTrail(t, "defeated owner", h.lords, "score 4000", "msg 1772/1000")
		wantTrail(t, "new owner", h.rivals, "score 1000", "msg 1773/1000")
		for clanID, want := range map[int32]int64{castleClanID: lordsStartReputation - 1000, rivalClanID: 1000} {
			if got := queryCastleInt(t, h.w.srv, "SELECT reputation_score FROM clan_data WHERE clan_id = ?", clanID); got != want {
				t.Fatalf("clan %#x stored reputation = %d, want %d", clanID, got, want)
			}
		}
	})
	t.Run("owner holds the castle", func(t *testing.T) {
		t.Parallel()
		h := runSiegeEnd(t, false, false)
		wantTrail(t, "holding owner", h.lords, "score 5500", "msg 1773/500")
		wantTrail(t, "attacker", h.rivals)
		for clanID, want := range map[int32]int64{castleClanID: lordsStartReputation + 500, rivalClanID: 0} {
			if got := queryCastleInt(t, h.w.srv, "SELECT reputation_score FROM clan_data WHERE clan_id = ?", clanID); got != want {
				t.Fatalf("clan %#x stored reputation = %d, want %d", clanID, got, want)
			}
		}
	})
}

// TestSiegeEndWritesTheNewOwnersNoblesHeroDiary ends Gludio's siege with the
// castle taken from the Lords: each noble of the clan taking it, in the
// world, gets a CASTLE_TAKEN diary entry naming Gludio (stored in
// heroes_diary); a member that is no noble gets none, nor does a noble of
// the clan losing it, and none is written when the owner keeps the castle.
func TestSiegeEndWritesTheNewOwnersNoblesHeroDiary(t *testing.T) {
	t.Parallel()
	diary := func(t *testing.T, h *handover, charID int32) int64 {
		t.Helper()
		return queryCastleInt(t, h.w.srv, "SELECT COUNT(*) FROM heroes_diary WHERE char_id = ? AND action = ? AND param = 1", charID, hero.DiaryCastleTaken)
	}
	for _, tt := range []struct {
		name                 string
		transfer, rivalNoble bool
		wantRival            int64
	}{
		{"noble of the new owner", true, true, 1},
		{"member of the new owner who is no noble", true, false, 0},
		{"the owner keeps the castle", false, true, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := runSiegeEnd(t, tt.transfer, tt.rivalNoble)
			if got := diary(t, h, rivalID); got != tt.wantRival {
				t.Fatalf("new owner's CASTLE_TAKEN diary rows = %d, want %d", got, tt.wantRival)
			}
			if got := diary(t, h, h.w.leaderID); got != 0 {
				t.Fatalf("former owner's CASTLE_TAKEN diary rows = %d, want none", got)
			}
		})
	}
}
