package character

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// sevenSignsWorld is one character, "Newbie", booted over a seeded Seven
// Signs status whose period changes fire only on demand.
type sevenSignsWorld struct {
	srv    *gameservertest.Server
	objID  int32
	change func()
}

// sevenSignsSetup is how "Newbie" and the competition stand before the
// login: the status row edit, Newbie's sign-up (none when cabal is
// NoCabal), and whether it is stored in a Seven Signs dungeon.
type sevenSignsSetup struct {
	status  func(*sevensigns.StatusRow)
	cabal   sevensigns.Cabal
	dungeon bool
}

// bootSevenSigns boots Newbie as setup describes. The character is not in
// the world yet.
func bootSevenSigns(t *testing.T, setup sevenSignsSetup, opts ...gameservertest.Option) *sevenSignsWorld {
	t.Helper()
	w := &sevenSignsWorld{}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) {
			if setup.status == nil {
				return
			}
			ctx := context.Background()
			row, found, err := store.LoadStatus(ctx)
			if err != nil || !found {
				t.Fatalf("load status row: found=%v err=%v", found, err)
			}
			setup.status(&row)
			if err := store.SaveStatus(ctx, row); err != nil {
				t.Fatalf("seed status row: %v", err)
			}
		}),
		// Boot and every period change run on this goroutine, so the
		// pending change needs no lock.
		gameservertest.WithSevenSignsTimer(func(_ time.Duration, fn func()) *time.Timer {
			w.change = fn
			return nil
		}),
	}, opts...)...)
	w.srv = srv
	w.objID = srv.SoleObjectID(t)
	if setup.cabal != sevensigns.NoCabal {
		if err := srv.SevenSigns.SetPlayerInfo(context.Background(), w.objID, setup.cabal, sevensigns.Strife); err != nil {
			t.Fatalf("sign up: %v", err)
		}
	}
	if setup.dungeon {
		if _, err := srv.DB.Exec("UPDATE characters SET isin7sdungeon = 1 WHERE obj_Id = ?", w.objID); err != nil {
			t.Fatalf("seed dungeon membership: %v", err)
		}
	}
	return w
}

// enter selects Newbie and sends EnterWorld, returning every frame up to
// the quiet that follows.
func (w *sevenSignsWorld) enter(t *testing.T) [][]byte {
	t.Helper()
	c := w.srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	return readUntilQuiet(c)
}

// online returns Newbie's live character.
func (w *sevenSignsWorld) online(t *testing.T) *player.Character {
	t.Helper()
	obj, ok := w.srv.State.Player(w.objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	return c
}

// fire runs the pending period change and returns what the client heard.
func (w *sevenSignsWorld) fire(t *testing.T) [][]byte {
	t.Helper()
	if w.change == nil {
		t.Fatal("no period change pending")
	}
	w.change()
	return readUntilQuiet(w.srv.Client)
}
