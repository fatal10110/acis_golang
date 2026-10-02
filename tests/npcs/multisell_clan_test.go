package npcs

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: MultiSellChoose.runImpl (MultiSellChoose.java:183-205) refuses
// a clan reputation price (id 65336) with YOU_ARE_NOT_A_CLAN_MEMBER,
// ONLY_THE_CLAN_LEADER_IS_ENABLED or THE_CLAN_REPUTATION_SCORE_IS_TOO_LOW;
// takes it in ingredient order with Clan.takeReputationScore, whose
// refresh (Clan.setReputationScore, Clan.java:1580-1645) reaches the
// members before S1_DEDUCTED_FROM_CLAN_REP (MultiSellChoose.java:214-220);
// and pays a reputation product with Clan.addReputationScore, unnamed
// (MultiSellChoose.java:297-298). aCis revision in the outer repo.

const msClanID = 268435456

// msClan seeds the clan Traders at level and reputation; the trader leads
// it unless leader names another character, stored here as a member too.
func msClan(t *testing.T, level, reputation int, leader string) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		ctx := context.Background()
		stmts := []string{"UPDATE characters SET clanid = 268435456 WHERE char_name = 'Trader'"}
		if leader != "" {
			stmts = append(stmts, "INSERT INTO characters (account_name, obj_Id, char_name, level, clanid, power_grade) VALUES ('elder', 910000, '"+leader+"', 40, 268435456, 1)")
		} else {
			leader = "Trader"
		}
		for _, stmt := range stmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id)
			SELECT ?, 'Traders', ?, ?, obj_Id FROM characters WHERE char_name = ?`, msClanID, level, reputation, leader); err != nil {
			t.Fatal(err)
		}
	})
}

// storedReputation reads the clan's stored reputation score.
func (w *msWorld) storedReputation(t *testing.T) int {
	t.Helper()
	w.srv.FlushPersistence(t)
	var rep int
	if err := w.srv.DB.QueryRowContext(context.Background(), "SELECT reputation_score FROM clan_data WHERE clan_id = ?", msClanID).Scan(&rep); err != nil {
		t.Fatal(err)
	}
	return rep
}

// TestMultisellClanReputationNeedsTheLeader pins a member who does not lead
// its clan buying with clan reputation: ONLY_THE_CLAN_LEADER_IS_ENABLED,
// nothing taken, the list kept.
func TestMultisellClanReputationNeedsTheLeader(t *testing.T) {
	t.Parallel()
	w := bootMultisell(t, msTalker(), [][2]int32{{msOreID, 2}}, nil, msClan(t, 5, 1000, "Elder"))
	w.mustOpen(t, "9005")

	assertMessages(t, w.choose(t, "9005", 1, 1), msg(serverpackets.SystemMessageOnlyClanLeaderEnabled))
	if got := w.held(t, msOreID); got != 2 {
		t.Fatalf("ore = %d after the refusal, want 2", got)
	}
	if got := w.storedReputation(t); got != 1000 {
		t.Fatalf("stored reputation = %d after the refusal, want 1000", got)
	}
	assertMessages(t, w.choose(t, "9005", 2, 1), msg(serverpackets.SystemMessageS1Disappeared, msOreID), traded)
}

// TestMultisellPaysWithClanReputation pins the leader trading with clan
// reputation: two units of a price of 10 reputation and an ore show the
// clan its header, then name the 20 points deducted, then the ores taken
// and the potions earned; a product of 100 reputation goes to the clan
// unnamed. The score is stored each time.
func TestMultisellPaysWithClanReputation(t *testing.T) {
	t.Parallel()
	w := bootMultisell(t, msTalker(), [][2]int32{{msOreID, 3}}, nil, msClan(t, 5, 1000, ""))
	w.mustOpen(t, "9005")

	frames := w.choose(t, "9005", 1, 2)
	assertMessages(t, frames,
		msg(serverpackets.SystemMessageS1DeductedFromClanRep, 20),
		msg(serverpackets.SystemMessageS2S1Disappeared, msOreID, 2),
		msg(serverpackets.SystemMessageEarnedS2S1S, msPotionID, 2),
		traded)
	const info, sys = serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage
	if got := msOnly(frames, info, sys); !slices.Equal(got, []byte{info, sys, sys, sys, sys}) {
		t.Fatalf("exchange frames = %x, want the clan header before the messages", opcodes(frames))
	}
	if got := w.storedReputation(t); got != 980 {
		t.Fatalf("stored reputation = %d, want 980", got)
	}

	frames = w.choose(t, "9005", 2, 1)
	assertMessages(t, frames, msg(serverpackets.SystemMessageS1Disappeared, msOreID), traded)
	if got := msOnly(frames, info, sys); !slices.Equal(got, []byte{sys, info, sys}) {
		t.Fatalf("reputation product frames = %x, want the clan header after the ore", opcodes(frames))
	}
	if got := w.storedReputation(t); got != 1080 {
		t.Fatalf("stored reputation = %d, want 1080", got)
	}
}

// TestMultisellClanReputationToZero pins a price that takes the clan's
// last reputation: the clan skills are switched off first, then the
// deduction is named.
func TestMultisellClanReputationToZero(t *testing.T) {
	t.Parallel()
	w := bootMultisell(t, msTalker(), [][2]int32{{msOreID, 1}}, nil, msClan(t, 5, 10, ""))
	w.mustOpen(t, "9004")

	assertMessages(t, w.choose(t, "9004", 4, 1),
		msg(serverpackets.SystemMessageReputationLowClanSkillsDeactivated),
		msg(serverpackets.SystemMessageS1DeductedFromClanRep, 10),
		msg(serverpackets.SystemMessageEarnedItemS1, msPotionID),
		traded)
	if got := w.storedReputation(t); got != 0 {
		t.Fatalf("stored reputation = %d, want 0", got)
	}
}

// msOnly keeps the opcodes of frames found in keep, in order.
func msOnly(frames [][]byte, keep ...byte) []byte {
	var out []byte
	for _, f := range frames {
		if slices.Contains(keep, f[0]) {
			out = append(out, f[0])
		}
	}
	return out
}
