package quest

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// levelStep is one level the tutorial hears of: the question mark, voice
// and radar mark (zero for none) it answers with, or nothing at all when
// mark is 0.
type levelStep struct {
	level int
	mark  int32
	voice string
	radar [3]int32
}

// levelRig is a newbie whose class and level the tests move.
type levelRig struct {
	t      *testing.T
	srv    *gameservertest.Server
	levels *player.LevelTable
	objID  int32
	at     location.Location
}

// bootLevelRig boots the real tutorial with a human fighter newbie in the
// world, playing classID.
func bootLevelRig(t *testing.T, classID int) *levelRig {
	t.Helper()
	levels := flatLevels(t)
	srv := bootNewbie(t, true, false, gameservertest.WithLevels(levels))
	objID := createNewbie(t, srv)
	enterBurst(t, srv)
	r := &levelRig{t: t, srv: srv, levels: levels, objID: objID}
	onCharacter(t, srv, objID, func(c *player.Character) {
		c.SetClassID(classID)
		r.at = c.CurrentLocation()
	})
	return r
}

// gainTo gives the newbie the experience of level, which raises the
// tutorial's level event once when the level went up, and returns the
// tutorial frames it answered with.
func (r *levelRig) gainTo(level int) [][]byte {
	r.t.Helper()
	onCharacter(r.t, r.srv, r.objID, func(c *player.Character) {
		c.AddExpAndSp(r.levels.RequiredExpForLevel(level)-c.Exp, 0)
		if c.Level() != level {
			r.t.Errorf("level = %d, want %d", c.Level(), level)
		}
	})
	return tutorialFrames(r.srv.ReadQueued(r.t, r.srv.Client))
}

// loseTo takes experience off down to the start of level; a level loss
// is not heard by the tutorial.
func (r *levelRig) loseTo(level int) {
	r.t.Helper()
	onCharacter(r.t, r.srv, r.objID, func(c *player.Character) {
		c.RemoveExp(r.levels, nil, c.Exp-r.levels.RequiredExpForLevel(level))
		if c.Level() != level {
			r.t.Errorf("level after the loss = %d, want %d", c.Level(), level)
		}
	})
	if got := tutorialFrames(r.srv.ReadQueued(r.t, r.srv.Client)); len(got) != 0 {
		r.t.Fatalf("level loss to %d sent %x, want nothing", level, got)
	}
}

// want is what the step answers with: its question mark, the tutorial
// sound, its voice and its radar mark; none for a step that does not fire.
func (r *levelRig) want(s levelStep) []wire.Frame {
	if s.mark == 0 {
		return nil
	}
	out := []wire.Frame{serverpackets.FrameTutorialShowQuestionMark(s.mark), serverpackets.FramePlaySound(script.SoundTutorial)}
	if s.voice != "" {
		out = append(out, voiceFrame(r.objID, r.at, s.voice))
	}
	if s.radar != [3]int32{} {
		out = append(out, radarMark(s.radar)...)
	}
	return out
}

// radarMark is the two packets that mark xyz on the radar.
func radarMark(xyz [3]int32) []wire.Frame {
	add := serverpackets.Radar{Show: 2, Type: 2, X: xyz[0], Y: xyz[1], Z: xyz[2]}
	show := serverpackets.Radar{Show: 0, Type: 1, X: xyz[0], Y: xyz[1], Z: xyz[2]}
	return []wire.Frame{serverpackets.FrameRadarControl(add), serverpackets.FrameRadarControl(show)}
}

// run gains each step's level in turn and requires its answer and the
// level the tutorial remembers afterwards, wantLvl[i].
func (r *levelRig) run(steps []levelStep, wantLvl []string) {
	r.t.Helper()
	for i, s := range steps {
		expectFrames(r.t, "level "+strconv.Itoa(s.level), r.gainTo(s.level), r.want(s)...)
		if got := r.srv.QuestVars(r.t, r.objID, tutorial)["lvl"]; got != wantLvl[i] {
			r.t.Fatalf("lvl after level %d = %q, want %q", s.level, got, wantLvl[i])
		}
	}
}

// TestTutorialLevelStepsFighter: a human fighter is shown question mark 24
// at level 6, 25 at 9, 17 at 15 (no voice), 13 at 19, 15 at 35 and 16 at
// 75, each with the tutorial sound and its voice. Levels 7, 10 and 12
// have no step for a fighter of the human line, and a level already heard
// of is not heard again after a loss.
func TestTutorialLevelStepsFighter(t *testing.T) {
	t.Parallel()
	r := bootLevelRig(t, 0)
	r.run([]levelStep{
		{level: 6, mark: 24, voice: "tutorial_voice_020"},
		{level: 7},
		{level: 9, mark: 25, voice: "tutorial_voice_021"},
		{level: 10},
		{level: 12},
		{level: 15, mark: 17},
		{level: 19, mark: 13, voice: "tutorial_voice_022"},
		{level: 35, mark: 15, voice: "tutorial_voice_023"},
		{level: 75, mark: 16, voice: "tutorial_voice_024"},
	}, []string{"6", "6", "9", "9", "9", "15", "19", "35", "75"})

	r.loseTo(34)
	expectFrames(t, "level 35 again", r.gainTo(35))
	r.loseTo(18)
	expectFrames(t, "level 19 again", r.gainTo(19))
}

// TestTutorialLevelStepsMystic: a human mystic hears the mystic voice at
// level 5, no step at 9, and question mark 25 at 10 where a fighter of the
// human line hears none; level 7 shows question mark 11 with voice 019 and
// marks the mystic's guide on the radar.
func TestTutorialLevelStepsMystic(t *testing.T) {
	t.Parallel()
	r := bootLevelRig(t, 10)
	r.run([]levelStep{
		{level: 5, mark: 9, voice: "tutorial_voice_015"},
		{level: 6, mark: 24, voice: "tutorial_voice_020"},
		{level: 7, mark: 11, voice: "tutorial_voice_019", radar: [3]int32{-84981, 244764, -3726}},
		{level: 9},
		{level: 10, mark: 25, voice: "tutorial_voice_021"},
		{level: 12},
	}, []string{"5", "6", "7", "7", "10", "10"})
}

// TestTutorialLevelStepsOrcFighter: an orc fighter hears question mark 25
// at level 12 and not at 10, and not again once it has.
func TestTutorialLevelStepsOrcFighter(t *testing.T) {
	t.Parallel()
	r := bootLevelRig(t, 44)
	r.run([]levelStep{
		{level: 7},
		{level: 10},
		{level: 12, mark: 25, voice: "tutorial_voice_021"},
	}, []string{"", "", "12"})
	r.loseTo(11)
	expectFrames(t, "level 12 again", r.gainTo(12))
}

// TestTutorialLevelStepsOrcMystic: an orc mystic marks its guide at level
// 7 and hears question mark 25 at level 12, and hears it again whenever it
// regains level 12: the orc mystic is told at level 12 whatever level it
// was told of last, where the orc fighter is not.
func TestTutorialLevelStepsOrcMystic(t *testing.T) {
	t.Parallel()
	r := bootLevelRig(t, 49)
	r.run([]levelStep{
		{level: 7, mark: 11, voice: "tutorial_voice_019", radar: [3]int32{-46225, -113312, -21}},
		{level: 10},
		{level: 12, mark: 25, voice: "tutorial_voice_021"},
	}, []string{"7", "7", "12"})
	r.loseTo(11)
	step := levelStep{level: 12, mark: 25, voice: "tutorial_voice_021"}
	expectFrames(t, "level 12 again", r.gainTo(12), r.want(step)...)
}
