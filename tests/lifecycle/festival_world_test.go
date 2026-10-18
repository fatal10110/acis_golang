package lifecycle

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bloodOfferingTemplates is the shared item catalog plus the blood
// offering.
func bloodOfferingTemplates() gameservertest.Option {
	return gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: festival.BloodOfferingID, Name: "Blood Offering", Kind: item.KindEtcItem, Duration: -1,
		Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
	})))
}

// The festival's reset as a new Seven Signs cycle begins takes every blood
// offering from the players online: the player only sees the stack leave
// its inventory with the next inventory update, and the row is deleted.
// Its other items stay.
func TestCycleBegunTakesBloodOfferings(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		bloodOfferingTemplates(),
	)
	owner := srv.SoleObjectID(t)
	offerings := srv.GiveItem(t, owner, festival.BloodOfferingID, 12)
	potions := srv.GiveItem(t, owner, 20, 3)
	c := srv.Client
	startInWorld(t, c)

	srv.Festival.CycleBegun(2)
	srv.InventoryUpdates.Tick()
	frames := srv.ReadQueued(t, c)
	if len(frames) != 1 {
		t.Fatalf("reset answer = %d frames, want the InventoryUpdate only", len(frames))
	}
	entries := readInventoryUpdateEntries(t, frames[0])
	if len(entries) != 1 || entries[0].objID != offerings || entries[0].state != 3 {
		t.Fatalf("InventoryUpdate = %+v, want the offerings (%d) removed", entries, offerings)
	}
	srv.FlushItems(t)
	var left int
	if err := srv.DB.QueryRow(`SELECT COUNT(*) FROM items WHERE object_id = ?`, offerings).Scan(&left); err != nil || left != 0 {
		t.Fatalf("offering rows after the reset = %d (%v), want 0", left, err)
	}
	if err := srv.DB.QueryRow(`SELECT COUNT(*) FROM items WHERE object_id = ?`, potions).Scan(&left); err != nil || left != 1 {
		t.Fatalf("potion rows after the reset = %d (%v), want 1", left, err)
	}
}

// The competition's end rewards, for each festival in turn, every member of
// the party that set its best score in any cycle: the member's clan earns
// 100 reputation, shown to its members online, who are then told who
// earned it. A member online answers for its own clan, matched in any
// case; one offline is found in the clans' rosters. A member of a clan
// below level 5 earns nothing, though its clan is still told, and a member
// earns again for each festival it holds the record of. Of equal records,
// the earlier cycle's party counts.
//
// Newbie (online, Lords, 50 points) and Ghost (offline, Ghosts) hold
// festival 0's record of cycle 1; Newbie also holds festival 1's of cycle
// 0, under its name in lower case; Lowly (offline, level-3 Lowlies) holds
// festival 2's of cycle 0, tied by Ghost in cycle 1.
func TestCompetitionEndRewardsBestPartyClans(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
			for i, name := range []string{"Ghost", "Lowly"} {
				ch, err := player.NewCharacter(int32(5001+i), gameservertest.ClassTemplate(), "other", name, 20, 0, 0, player.SexMale)
				if err != nil {
					t.Fatal(err)
				}
				if err := chars.Create(context.Background(), ch); err != nil {
					t.Fatal(err)
				}
			}
		}),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, c := range []struct {
				id                int32
				name, leader      string
				level, reputation int
			}{{0x7f000081, "Lords", "Newbie", 5, 50}, {0x7f000082, "Ghosts", "Ghost", 5, 0}, {0x7f000083, "Lowlies", "Lowly", 3, 0}} {
				if _, err := db.Exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id)
					SELECT ?, ?, ?, ?, obj_Id FROM characters WHERE char_name = ?`, c.id, c.name, c.level, c.reputation, c.leader); err != nil {
					t.Fatalf("seed clan %s: %v", c.name, err)
				}
				if _, err := db.Exec(`UPDATE characters SET clanid = ? WHERE char_name = ?`, c.id, c.leader); err != nil {
					t.Fatalf("seed clan %s: %v", c.name, err)
				}
			}
		}),
		gameservertest.WithFestivalSeed(func(store *gamesql.FestivalStore) {
			if err := store.SaveScores(context.Background(), []festival.Score{
				{FestivalID: 0, Cabal: sevensigns.Dusk, Cycle: 1, Date: 1, Score: 10, Members: "Newbie,Ghost"},
				{FestivalID: 0, Cabal: sevensigns.Dawn, Cycle: 0, Date: 1, Score: 9, Members: "Lowly"},
				{FestivalID: 1, Cabal: sevensigns.Dawn, Cycle: 0, Date: 1, Score: 20, Members: "newbie"},
				{FestivalID: 2, Cabal: sevensigns.Dusk, Cycle: 0, Date: 1, Score: 30, Members: "Lowly"},
				{FestivalID: 2, Cabal: sevensigns.Dawn, Cycle: 1, Date: 1, Score: 30, Members: "Ghost"},
			}); err != nil {
				t.Fatalf("seed festival scores: %v", err)
			}
		}),
	)
	c := srv.Client
	// A clan member's login burst carries its clan's packets too: only drain
	// it.
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "game start SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "game start CharSelected")
	c.Send(encodeEnterWorld())
	srv.ReadQueued(t, c)

	srv.Festival.CompetitionEnded()
	frames := srv.ReadQueued(t, c)
	want := []byte{serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage}
	got := make([]byte, len(frames))
	for i, f := range frames {
		got[i] = f[0]
	}
	if string(got) != string(want) {
		t.Fatalf("reward answer = % x, want % x", got, want)
	}
	for i, name := range []string{"Newbie", "newbie"} {
		r := wire.NewReader(frames[2*i+1][1:])
		id, n := r.ReadInt32(), r.ReadInt32()
		textType, text := r.ReadInt32(), r.ReadString()
		numberType, number := r.ReadInt32(), r.ReadInt32()
		if id != serverpackets.SystemMessageFestivalHighestRankedParty || n != 2 || textType != serverpackets.SystemMessageParamText || text != name ||
			numberType != serverpackets.SystemMessageParamNumber || number != 100 {
			t.Fatalf("reward message %d = id %d, %d params, %d %q, %d %d; want %d naming %q and 100", i, id, n, textType, text, numberType, number,
				serverpackets.SystemMessageFestivalHighestRankedParty, name)
		}
	}

	srv.FlushPersistence(t)
	for name, wantScore := range map[string]int{"Lords": 250, "Ghosts": 100, "Lowlies": 0} {
		var score int
		if err := srv.DB.QueryRow(`SELECT reputation_score FROM clan_data WHERE clan_name = ?`, name).Scan(&score); err != nil {
			t.Fatal(err)
		}
		if score != wantScore {
			t.Errorf("%s reputation = %d, want %d", name, score, wantScore)
		}
	}
}

// withLordsClan seeds the level-5 clan Lords, 50 points, led by Newbie.
func withLordsClan(t *testing.T) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		if _, err := db.Exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id)
			SELECT 0x7f000081, 'Lords', 5, 50, obj_Id FROM characters WHERE char_name = 'Newbie'`); err != nil {
			t.Fatalf("seed clan: %v", err)
		}
		if _, err := db.Exec(`UPDATE characters SET clanid = 0x7f000081 WHERE char_name = 'Newbie'`); err != nil {
			t.Fatalf("seed clan: %v", err)
		}
	})
}

// As the competition ends, the clans of the festivals' best parties are
// rewarded once the end is announced and before the winning cabal is:
// a member online sees the end, its clan's new score and who earned it,
// then the winner.
func TestCompetitionEndRewardPrecedesTheWinner(t *testing.T) {
	t.Parallel()
	var change func()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1),
		withLordsClan(t),
		seedSevenSignsStatus(t, func(row *sevensigns.StatusRow) {
			row.Period = sevensigns.Competition
			row.DawnStoneScore = 10
		}),
		gameservertest.WithSevenSignsTimer(func(_ time.Duration, fn func()) *time.Timer { change = fn; return nil }),
		gameservertest.WithFestivalSeed(func(store *gamesql.FestivalStore) {
			if err := store.SaveScores(context.Background(), []festival.Score{
				{FestivalID: 0, Cabal: sevensigns.Dusk, Cycle: 1, Date: 1, Score: 10, Members: "Newbie"},
			}); err != nil {
				t.Fatalf("seed festival scores: %v", err)
			}
		}),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "game start SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "game start CharSelected")
	c.Send(encodeEnterWorld())
	srv.ReadQueued(t, c)

	change()
	frames := srv.ReadQueued(t, c)
	message := func(f []byte) int32 {
		if f[0] != serverpackets.OpcodeSystemMessage {
			return -1
		}
		return wire.NewReader(f[1:]).ReadInt32()
	}
	if len(frames) < 5 || frames[0][0] != serverpackets.OpcodePlaySound || message(frames[1]) != serverpackets.SystemMessageQuestEventPeriodEnded ||
		frames[2][0] != serverpackets.OpcodePledgeShowInfoUpdate || message(frames[3]) != serverpackets.SystemMessageFestivalHighestRankedParty {
		got := make([]byte, len(frames))
		for i, f := range frames {
			got[i] = f[0]
		}
		t.Fatalf("period change answer = % x, want PlaySound, the end, the clan's score, the reward message, then the rest", got)
	}
	won := -1
	for i, f := range frames {
		if message(f) == serverpackets.SystemMessageDawnWon {
			won = i
		}
	}
	if won < 4 {
		t.Fatalf("dawn won message at %d, want it after the reward", won)
	}
}
