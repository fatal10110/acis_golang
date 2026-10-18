package quest

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// levelUpTemplate is the fixture class with per-level HP, MP and CP rows,
// so a level gain refills to maxima that rise with every level.
func levelUpTemplate() *player.Template {
	tmpl := gameservertest.ClassTemplate()
	tmpl.HPTable, tmpl.MPTable, tmpl.CPTable = make([]float64, 85), make([]float64, 85), make([]float64, 85)
	for i := range 85 {
		tmpl.HPTable[i] = float64(80 + 40*i)
		tmpl.MPTable[i] = float64(30 + 20*i)
		tmpl.CPTable[i] = float64(32 + 16*i)
	}
	return tmpl
}

// statusFrame is the StatusUpdate a player's own client gets for its
// current HP, MP and CP and its maximum CP.
func statusFrame(objID int32, hp, mp, cp, maxCP float64) wire.Frame {
	return serverpackets.FrameStatusUpdate(objID, []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentHP, Value: int(hp)},
		{Type: serverpackets.StatusCurrentMP, Value: int(mp)},
		{Type: serverpackets.StatusCurrentCP, Value: int(cp)},
		{Type: serverpackets.StatusMaxCP, Value: int(maxCP)},
	})
}

// levelUpFrames keeps the status reports, tutorial client events, social
// actions and system messages a level gain sends ahead of its first
// UserInfo.
func levelUpFrames(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeUserInfo:
			return out
		case serverpackets.OpcodeStatusUpdate, serverpackets.OpcodeTutorialEnableClientEvent,
			serverpackets.OpcodeSocialAction, serverpackets.OpcodeSystemMessage:
			out = append(out, f)
		}
	}
	return out
}

// TestLevelUpReportsStatus gains a level on a player short of HP, MP and CP,
// at under 30% HP. Every maximum rose, so each owes a report of the values
// before the refill — the CP one, the HP one with the low-HP tutorial
// event (CE45) right after it, then the MP one — ahead of the refill. The HP
// and MP refill reports once; the tutorial hears CE40; the CP refill reports
// once more; then come the level-up social action and message.
func TestLevelUpReportsStatus(t *testing.T) {
	t.Parallel()
	levels := flatLevels(t)
	srv, objID, heard := bootTutorial(t, true, gameservertest.WithLevels(levels),
		gameservertest.WithClassTemplate(levelUpTemplate()))
	enterWorld(t, srv)

	var before, after player.Resources
	onCharacter(t, srv, objID, func(c *player.Character) {
		c.SetHP(10)
		c.ReduceCurrentMP(5)
		c.SetCP(7)
		c.BroadcastStatus()
		before = c.ResourceValues()
	})
	srv.ReadQueued(t, srv.Client)
	heard.take()

	onCharacter(t, srv, objID, func(c *player.Character) {
		c.AddExpAndSp(levels.RequiredExpForLevel(2), 0)
		after = c.ResourceValues()
	})
	frames := levelUpFrames(srv.ReadQueued(t, srv.Client))
	if got := heard.take(); !slices.Equal(got, []string{"CE45", "CE40"}) {
		t.Fatalf("heard %q, want [CE45 CE40]", got)
	}
	if after.MaxHP == before.MaxHP || after.MaxMP == before.MaxMP || after.MaxCP == before.MaxCP {
		t.Fatalf("maxima %+v -> %+v; the level gain must raise all three", before, after)
	}

	unfilled := func() wire.Frame {
		return statusFrame(objID, before.CurrentHP, before.CurrentMP, before.CurrentCP, after.MaxCP)
	}
	expectFrames(t, "level gain", frames,
		unfilled(),
		unfilled(),
		serverpackets.FrameTutorialEnableClientEvent(45),
		unfilled(),
		statusFrame(objID, after.MaxHP, after.MaxMP, before.CurrentCP, after.MaxCP),
		serverpackets.FrameTutorialEnableClientEvent(40),
		statusFrame(objID, after.MaxHP, after.MaxMP, after.MaxCP, after.MaxCP),
		serverpackets.FrameSocialAction(objID, 15),
		serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouIncreasedYourLevel),
	)
	if after.CurrentHP != after.MaxHP || after.CurrentMP != after.MaxMP || after.CurrentCP != after.MaxCP {
		t.Fatalf("resources after the level gain = %+v, want full", after)
	}
}

// TestDeadLevelUpReportsNoStatus gains a level on a dead player: the corpse
// keeps its values and gets no status report, but the HP refill still
// raises the low-HP tutorial event ahead of CE40.
func TestDeadLevelUpReportsNoStatus(t *testing.T) {
	t.Parallel()
	levels := flatLevels(t)
	srv, objID, heard := bootTutorial(t, true, gameservertest.WithLevels(levels),
		gameservertest.WithClassTemplate(levelUpTemplate()))
	enterWorld(t, srv)

	onCharacter(t, srv, objID, func(c *player.Character) { c.TakeDamage(1_000_000, c) })
	srv.ReadQueued(t, srv.Client)
	heard.take()

	var after player.Resources
	onCharacter(t, srv, objID, func(c *player.Character) {
		c.AddExpAndSp(levels.RequiredExpForLevel(2), 0)
		after = c.ResourceValues()
	})
	frames := levelUpFrames(srv.ReadQueued(t, srv.Client))
	if got := heard.take(); !slices.Equal(got, []string{"CE45", "CE40"}) {
		t.Fatalf("heard %q, want [CE45 CE40]", got)
	}
	expectFrames(t, "dead level gain", frames,
		serverpackets.FrameTutorialEnableClientEvent(45),
		serverpackets.FrameTutorialEnableClientEvent(40),
		serverpackets.FrameSocialAction(objID, 15),
		serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouIncreasedYourLevel),
	)
	if after.CurrentHP != 0 {
		t.Fatalf("corpse HP after the level gain = %v, want 0", after.CurrentHP)
	}
}
