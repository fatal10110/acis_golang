package lifecycle

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

func encodeRequestSSQStatus(page byte) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestSSQStatus)
	w.WriteUint8(page)
	return w.Bytes()
}

// seedSevenSignsStatus rewrites the status row through edit before boot.
func seedSevenSignsStatus(t *testing.T, edit func(*sevensigns.StatusRow)) gameservertest.Option {
	return gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) {
		ctx := context.Background()
		row, found, err := store.LoadStatus(ctx)
		if err != nil || !found {
			t.Fatalf("load status row: found=%v err=%v", found, err)
		}
		edit(&row)
		if err := store.SaveStatus(ctx, row); err != nil {
			t.Fatalf("seed status row: %v", err)
		}
	})
}

func assertQuietClient(t *testing.T, c *testsupport.ScriptedClient, what string) {
	t.Helper()
	if frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("%s: unexpected opcode %#x", what, frame[0])
	}
}

// The record's pages answer RequestSSQStatus from the live competition: the
// player's own sign-up and stones, both cabals' scores, the seal votes and
// the predicted owners. The festival page waits on the festival and
// releases the client; an unknown page carries only the page and period.
//
// Seeded: cycle 3, competition, stone points dawn 30 / dusk 10, dawn
// festival 5, Gnosis owned by Dusk and Strife by Dawn. The player then signs
// up for Dawn choosing Avarice and turns in one red stone (10 points), so the
// pot is dawn 40 / dusk 10: stone shares 400 / 100, totals 405 / 100,
// percents 405/505 -> 80 and 100/505 -> 20.
func TestRequestSSQStatusAnswersEachRecordPage(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		seedSevenSignsStatus(t, func(row *sevensigns.StatusRow) {
			row.Cycle = 3
			row.Period = sevensigns.Competition
			row.DawnStoneScore, row.DuskStoneScore = 30, 10
			row.DawnFestivalScore = 5
			row.SealOwners = [3]sevensigns.Cabal{sevensigns.NoCabal, sevensigns.Dusk, sevensigns.Dawn}
		}),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	if err := srv.SevenSigns.SetPlayerInfo(context.Background(), objID, sevensigns.Dawn, sevensigns.Avarice); err != nil {
		t.Fatalf("sign up: %v", err)
	}
	if _, ok := srv.SevenSigns.AddPlayerStoneContrib(objID, 0, 0, 1, 1000000); !ok {
		t.Fatal("stone contribution refused")
	}

	for _, tc := range []struct {
		page byte
		want []byte
	}{
		{1, []byte{
			0xf5, 0x01, 0x01,
			0x03, 0x00, 0x00, 0x00, // cycle 3
			0x98, 0x04, 0x00, 0x00, // QUEST_EVENT_PERIOD
			0x06, 0x05, 0x00, 0x00, // UNTIL_MONDAY_6PM
			0x02, 0x01, // dawn, avarice
			0x01, 0x00, 0x00, 0x00, // one stone
			0x0a, 0x00, 0x00, 0x00, // ten ancient adena
			0x64, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x64, 0x00, 0x00, 0x00, 0x14, // dusk 100, 0, 100, 20%
			0x90, 0x01, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00, 0x95, 0x01, 0x00, 0x00, 0x50, // dawn 400, 5, 405, 80%
		}},
		{3, []byte{
			0xf5, 0x03, 0x01, 0x0a, 0x23, 0x03,
			0x01, 0x00, 0x00, 0x64, // avarice: unowned, no dusk members, 1 of 1 dawn
			0x02, 0x01, 0x00, 0x00, // gnosis: dusk
			0x03, 0x02, 0x00, 0x00, // strife: dawn
		}},
		{4, []byte{
			0xf5, 0x04, 0x01, 0x02, 0x03,
			0x00, 0x02, 0x0a, 0x05, 0x00, 0x00, // avarice claimed by dawn at 100%
			0x01, 0x00, 0x0b, 0x05, 0x00, 0x00, // gnosis: dusk under 10%, lost
			0x02, 0x00, 0x0b, 0x05, 0x00, 0x00, // strife: dawn under 10%, lost
		}},
		{9, []byte{0xf5, 0x09, 0x01}},
	} {
		c.Send(encodeRequestSSQStatus(tc.page))
		if got := c.Read(); !bytes.Equal(got, tc.want) {
			t.Fatalf("page %d = % x, want % x", tc.page, got, tc.want)
		}
		assertQuietClient(t, c, "record page")
	}

	c.Send(encodeRequestSSQStatus(2))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "festival page")
	assertQuietClient(t, c, "festival page")
}

// Once the competition is over the prediction page is not answered at all;
// the other pages still are.
func TestRequestSSQStatusPredictionSilentAfterCompetition(t *testing.T) {
	for _, period := range []sevensigns.Period{sevensigns.Results, sevensigns.SealValidation} {
		t.Run(period.String(), func(t *testing.T) {
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 1, 0),
				gameservertest.WithWantChars(1),
				seedSevenSignsStatus(t, func(row *sevensigns.StatusRow) {
					row.Period = period
				}),
			)
			c := srv.Client
			startInWorld(t, c)
			c.Send(encodeRequestSSQStatus(4))
			assertQuietClient(t, c, "prediction page")
			c.Send(encodeRequestSSQStatus(9))
			if got, want := c.Read(), []byte{0xf5, 0x09, byte(period)}; !bytes.Equal(got, want) {
				t.Fatalf("page 9 = % x, want % x", got, want)
			}
		})
	}
}

// Choosing a character shows the sky of the cabal leading seal validation.
func TestGameStartShowsValidationWinnerSky(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		seedSevenSignsStatus(t, func(row *sevensigns.StatusRow) {
			row.Period = sevensigns.SealValidation
			row.DuskStoneScore = 1
		}),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	if got, want := c.Read(), []byte{serverpackets.OpcodeSSQInfo, 0x01, 0x01}; !bytes.Equal(got, want) {
		t.Fatalf("game start SSQInfo = % x, want % x (dusk sky 257)", got, want)
	}
}

// A period change reaches every player online in order: the sound, the
// competition-ended message, each seal a cabal obtained, the winner, and
// then the new period's sky.
func TestPeriodChangeBroadcastsToPlayersOnline(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		seedSevenSignsStatus(t, func(row *sevensigns.StatusRow) {
			row.Period = sevensigns.Competition
			row.DawnStoneScore = 10
			row.DawnSealVotes = [3]int{1, 0, 0}
		}),
	)
	c := srv.Client
	startInWorld(t, c)

	// A second state over the same database fires its period change on
	// demand through the production broadcaster.
	var fire func()
	changer := sevensigns.NewState(gamesql.NewSevenSignsStore(srv.DB), network.NewSevenSignsBroadcaster(srv.State), zerolog.Nop(), time.Now,
		func(_ time.Duration, fn func()) *time.Timer { fire = fn; return nil })
	if err := changer.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	changer.Start()
	fire()

	sound := c.Read()
	assertFrameOpcode(t, sound, serverpackets.OpcodePlaySound, "period sound")
	r := wire.NewReader(sound[1:])
	if typ, file := r.ReadInt32(), r.ReadString(); typ != 0 || file != "SSQ_Neutral_01" {
		t.Fatalf("sound = (%d, %q), want (0, SSQ_Neutral_01)", typ, file)
	}
	for _, id := range []int{
		serverpackets.SystemMessageQuestEventPeriodEnded,
		serverpackets.SystemMessageDawnObtainedAvarice,
		serverpackets.SystemMessageDawnWon,
	} {
		assertSystemMessageID(t, c.Read(), id)
	}
	if got, want := c.Read(), []byte{serverpackets.OpcodeSSQInfo, 0x00, 0x01}; !bytes.Equal(got, want) {
		t.Fatalf("sky = % x, want % x (regular sky during results)", got, want)
	}
	assertQuietClient(t, c, "period change")

	var period, owner string
	if err := srv.DB.QueryRow(`SELECT active_period, avarice_owner FROM seven_signs_status WHERE id = 0`).Scan(&period, &owner); err != nil {
		t.Fatal(err)
	}
	if period != "RESULTS" || owner != "DAWN" {
		t.Fatalf("saved status = (%s, avarice %s), want (RESULTS, DAWN)", period, owner)
	}
}
