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
// each rising by its step per level (a zero step keeps the row flat), so a
// level gain refills to maxima that rise or stay as the steps say.
func levelUpTemplate(hpStep, mpStep, cpStep float64) *player.Template {
	tmpl := gameservertest.ClassTemplate()
	tmpl.HPTable, tmpl.MPTable, tmpl.CPTable = make([]float64, 85), make([]float64, 85), make([]float64, 85)
	for i := range 85 {
		tmpl.HPTable[i] = 80 + hpStep*float64(i)
		tmpl.MPTable[i] = 30 + mpStep*float64(i)
		tmpl.CPTable[i] = 32 + cpStep*float64(i)
	}
	return tmpl
}

// levelUpRun is one level gain: the resources before and after it, the
// frames levelUpFrames keeps and the tutorial events it raised.
type levelUpRun struct {
	objID         int32
	before, after player.Resources
	frames        [][]byte
	heard         []string
}

// gainLevel boots a player on tmpl, leaves it short of HP (under 30%), MP
// and CP, then gains it one level.
func gainLevel(t *testing.T, tmpl *player.Template) levelUpRun {
	t.Helper()
	levels := flatLevels(t)
	srv, objID, heard := bootTutorial(t, true, gameservertest.WithLevels(levels),
		gameservertest.WithClassTemplate(tmpl))
	enterWorld(t, srv)

	run := levelUpRun{objID: objID}
	onCharacter(t, srv, objID, func(c *player.Character) {
		c.SetHP(10)
		c.ReduceCurrentMP(5)
		c.SetCP(7)
		c.BroadcastStatus()
		run.before = c.ResourceValues()
	})
	srv.ReadQueued(t, srv.Client)
	heard.take()

	onCharacter(t, srv, objID, func(c *player.Character) {
		c.AddExpAndSp(levels.RequiredExpForLevel(2), 0)
		run.after = c.ResourceValues()
	})
	run.frames = levelUpFrames(srv.ReadQueued(t, srv.Client))
	run.heard = heard.take()
	if run.after.CurrentHP != run.after.MaxHP || run.after.CurrentMP != run.after.MaxMP || run.after.CurrentCP != run.after.MaxCP {
		t.Fatalf("resources after the level gain = %+v, want full", run.after)
	}
	return run
}

// expectHeard fails unless the level gain raised exactly want.
func (r levelUpRun) expectHeard(t *testing.T, want ...string) {
	t.Helper()
	if !slices.Equal(r.heard, want) {
		t.Fatalf("heard %q, want %q", r.heard, want)
	}
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
	r := gainLevel(t, levelUpTemplate(40, 20, 16))
	r.expectHeard(t, "CE45", "CE40")
	b, a := r.before, r.after
	if a.MaxHP == b.MaxHP || a.MaxMP == b.MaxMP || a.MaxCP == b.MaxCP {
		t.Fatalf("maxima %+v -> %+v; the level gain must raise all three", b, a)
	}
	unfilled := func() wire.Frame { return statusFrame(r.objID, b.CurrentHP, b.CurrentMP, b.CurrentCP, a.MaxCP) }
	expectFrames(t, "level gain", r.frames,
		unfilled(),
		unfilled(),
		serverpackets.FrameTutorialEnableClientEvent(45),
		unfilled(),
		statusFrame(r.objID, a.MaxHP, a.MaxMP, b.CurrentCP, a.MaxCP),
		serverpackets.FrameTutorialEnableClientEvent(40),
		statusFrame(r.objID, a.MaxHP, a.MaxMP, a.MaxCP, a.MaxCP),
		serverpackets.FrameSocialAction(r.objID, 15),
		serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouIncreasedYourLevel),
	)
}

// TestLevelUpFlatMPReportsAfterMPFill gains a level whose MP maximum stays
// put: MP owes no report and fills first, silently, so the CP and HP
// reports of the unfilled values carry full MP, CE45 follows the HP one,
// and the HP fill reports after them.
func TestLevelUpFlatMPReportsAfterMPFill(t *testing.T) {
	t.Parallel()
	r := gainLevel(t, levelUpTemplate(40, 0, 16))
	r.expectHeard(t, "CE45", "CE40")
	b, a := r.before, r.after
	if a.MaxMP != b.MaxMP || b.CurrentMP == a.MaxMP {
		t.Fatalf("MP %+v -> %+v; want a flat maximum over a short MP", b, a)
	}
	unfilled := func() wire.Frame { return statusFrame(r.objID, b.CurrentHP, a.MaxMP, b.CurrentCP, a.MaxCP) }
	expectFrames(t, "level gain with flat MP", r.frames,
		unfilled(),
		unfilled(),
		serverpackets.FrameTutorialEnableClientEvent(45),
		statusFrame(r.objID, a.MaxHP, a.MaxMP, b.CurrentCP, a.MaxCP),
		serverpackets.FrameTutorialEnableClientEvent(40),
		statusFrame(r.objID, a.MaxHP, a.MaxMP, a.MaxCP, a.MaxCP),
		serverpackets.FrameSocialAction(r.objID, 15),
		serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouIncreasedYourLevel),
	)
}

// TestLevelUpOnlyCPReportsAfterHPFill gains a level that raises only the CP
// maximum: HP and MP fill first, so CP's report of its unfilled value
// carries full HP and MP and is followed by the HP fill's own report; the
// full HP raises no low-HP event.
func TestLevelUpOnlyCPReportsAfterHPFill(t *testing.T) {
	t.Parallel()
	r := gainLevel(t, levelUpTemplate(0, 0, 16))
	r.expectHeard(t, "CE40")
	b, a := r.before, r.after
	if a.MaxHP != b.MaxHP || a.MaxMP != b.MaxMP || a.MaxCP == b.MaxCP {
		t.Fatalf("maxima %+v -> %+v; want only CP's to rise", b, a)
	}
	filledHP := func() wire.Frame { return statusFrame(r.objID, a.MaxHP, a.MaxMP, b.CurrentCP, a.MaxCP) }
	expectFrames(t, "level gain raising only CP", r.frames,
		filledHP(),
		filledHP(),
		serverpackets.FrameTutorialEnableClientEvent(40),
		statusFrame(r.objID, a.MaxHP, a.MaxMP, a.MaxCP, a.MaxCP),
		serverpackets.FrameSocialAction(r.objID, 15),
		serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouIncreasedYourLevel),
	)
}

// TestDeadLevelUpReportsNoStatus gains a level on a dead player: the corpse
// keeps its values and gets no status report, but the HP refill still
// raises the low-HP tutorial event ahead of CE40.
func TestDeadLevelUpReportsNoStatus(t *testing.T) {
	t.Parallel()
	levels := flatLevels(t)
	srv, objID, heard := bootTutorial(t, true, gameservertest.WithLevels(levels),
		gameservertest.WithClassTemplate(levelUpTemplate(40, 20, 16)))
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
