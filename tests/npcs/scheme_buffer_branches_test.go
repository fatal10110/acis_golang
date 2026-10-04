package npcs

import (
	"strconv"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
)

// addSchemeServitor gives the player a servitor sharing its queue.
func addSchemeServitor(t *testing.T, w *folkWorld) *summon.Actor {
	t.Helper()
	servitor, err := summon.NewServitor(summon.ServitorConfig{
		ObjectID: w.srv.NewObjectID(),
		Level:    44,
		Stats:    summon.CombatStats{MaxHP: 500, MaxMP: 200},
	})
	if err != nil {
		t.Fatal(err)
	}
	servitor.SetQueue(w.srv.PlayerQueue(t, w.player))
	w.srv.State.AddSummon(w.player, servitor)
	drainUntilQuiet(t, w.c)
	return servitor
}

// TestSchemeBufferOnSummon pins the summon branches of
// SchemeBuffer.onBypassFeedback: "Use on Pet" with a summon takes the fee
// first, then lands the scheme's buffs at the buffer's level on the summon
// and not on the talker; heal fills the summon's HP and MP; cleanup ends
// the summon's buffs.
func TestSchemeBufferOnSummon(t *testing.T) {
	t.Parallel()
	buffer := schemebuffer.New(schemebuffer.DefaultConfig(), schemeBufferBuffs(t), schemeBufferSkills(t))
	w, f := schemeBufferWorld(t, buffer, 3000)
	buffer.Restore([]schemebuffer.Row{{OwnerID: w.player, Name: "Pet", Skills: "1035,1036"}}, zerolog.Nop())
	servitor := addSchemeServitor(t, w)
	w.talkTo(t, f)
	w.bypass(t, npcCommand(f, "support"))

	frames := w.bypass(t, npcCommand(f, "givebuffs Pet 1000 pet"))
	if len(frames) < 2 || string(frames[0]) != string(sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(1000))) ||
		frames[len(frames)-1][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("givebuffs pet = %x, want the fee taken first and the release last", opcodes(frames))
	}
	if got := w.savedCount(t, item.AdenaID); got != 2000 {
		t.Fatalf("adena saved = %d, want 2000", got)
	}
	w.onPlayer(t, func(pc *player.Character) {
		for id, want := range map[int]int{1035: 4, 1036: 2} {
			if level, ok := servitor.EffectList().ActiveBySkillID(id); !ok || level != want {
				t.Errorf("summon buff %d = level %d (%v), want level %d", id, level, ok, want)
			}
			if _, ok := pc.EffectList().ActiveBySkillID(id); ok {
				t.Errorf("talker holds buff %d given to its summon", id)
			}
		}
	})

	menu := datapackPage(t, f, "50002.htm")
	w.bypass(t, npcCommand(f, "menu"))
	w.onPlayer(t, func(*player.Character) {
		servitor.SetHP(1)
		servitor.ReduceMP(150)
	})
	drainUntilQuiet(t, w.c)
	frames = w.bypass(t, npcCommand(f, "heal"))
	assertSchemePage(t, "heal", frames[len(frames)-2:], menu)
	w.onPlayer(t, func(*player.Character) {
		if hp, maxHP := servitor.HP(), servitor.MaxHPValue(); hp != maxHP {
			t.Errorf("summon HP after heal = %v, want %v", hp, maxHP)
		}
		if mp, maxMP := servitor.MPValue(), servitor.MaxMPValue(); mp != maxMP {
			t.Errorf("summon MP after heal = %v, want %v", mp, maxMP)
		}
	})

	frames = w.bypass(t, npcCommand(f, "cleanup"))
	assertSchemePage(t, "cleanup", frames[len(frames)-2:], menu)
	w.onPlayer(t, func(*player.Character) {
		for _, id := range []int{1035, 1036} {
			if _, ok := servitor.EffectList().ActiveBySkillID(id); ok {
				t.Errorf("cleanup left summon buff %d", id)
			}
		}
	})
}

// TestSchemeBufferRefusals pins three refusals of
// SchemeBuffer.onBypassFeedback, each its own frame order: skillselect on
// a scheme already holding the talker's maximum is told so, then the
// editor opens and the scheme does not grow; deletescheme naming nothing
// is told the name is invalid, then the schemes page opens; createscheme
// naming nothing is told the name rule alone.
func TestSchemeBufferRefusals(t *testing.T) {
	t.Parallel()
	buffer := schemebuffer.New(schemebuffer.DefaultConfig(), schemeBufferBuffs(t), schemeBufferSkills(t))
	w, f := schemeBufferWorld(t, buffer, 0)
	full := strings.TrimSuffix(strings.Repeat("1035,", 20), ",")
	stored := []schemebuffer.Row{{OwnerID: w.player, Name: "Full", Skills: full}}
	buffer.Restore(stored, zerolog.Nop())
	assertRows := func(what string) {
		t.Helper()
		if got := buffer.Rows(); len(got) != 1 || got[0] != stored[0] {
			t.Fatalf("%s: schemes = %+v, want %+v", what, got, stored)
		}
	}
	w.talkTo(t, f)

	w.openAnyNpcPage(t)
	frames := w.bypass(t, npcCommand(f, "skillselect Buffs Full 1036 1"))
	want := []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
	if got := opcodes(frames); string(got) != string(want) {
		t.Fatalf("skillselect at the maximum = %x, want %x", got, want)
	}
	if string(frames[0]) != string(noticeFrame("This scheme has reached the maximum amount of buffs.")) {
		t.Fatalf("skillselect at the maximum notice = %x", frames[0])
	}
	if page := schemePageOf(t, frames); !strings.Contains(page, "Full</font> scheme holds 20 / 20 buffs.") ||
		!strings.Contains(page, "_skillselect Buffs Full 1036 1\"") {
		t.Fatalf("editor after a refused select = %s", page)
	}
	assertRows("skillselect at the maximum")

	oid := strconv.Itoa(int(f.ObjectID()))
	listed := `<font color="LEVEL">Full [20 / 20] - cost: 20,000</font><br1>` +
		`<a action="bypass npc_` + oid + `_givebuffs Full 20000">Use on Me</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_givebuffs Full 20000 pet">Use on Pet</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_editschemes Buffs Full 1">Edit</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_deletescheme Full">Delete</a><br>`
	w.openAnyNpcPage(t)
	frames = w.bypass(t, npcCommand(f, "deletescheme"))
	if len(frames) != 3 || string(frames[0]) != string(noticeFrame("This scheme name is invalid.")) {
		t.Fatalf("deletescheme with no name = %x, want the notice, the schemes page and the release", opcodes(frames))
	}
	assertSchemePage(t, "deletescheme with no name", frames[1:],
		datapackPage(t, f, "50002-1.htm", "%schemes%", listed, "%max_schemes%", "4"))
	assertRows("deletescheme with no name")

	w.openAnyNpcPage(t)
	assertFrames(t, "createscheme with no name", w.bypass(t, npcCommand(f, "createscheme")),
		noticeFrame("Scheme's name must contain up to 14 chars. Spaces are trimmed."), []byte{serverpackets.OpcodeActionFailed})
	assertRows("createscheme with no name")
}
