package npcs

import (
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
)

// TestSchemeBufferUnofferedSkillKeepsPagesAnswering pins the parity
// exception of #5033: a skill id the buffer does not offer never enters a
// scheme, neither from the stored list nor from the editor, and costs
// nothing, so the schemes page opens with a fee that leaves it out and
// support, createscheme and deletescheme each answer with the page. The
// '$' name widens the editor's links so the unoffered pick reaches the
// buffer.
func TestSchemeBufferUnofferedSkillKeepsPagesAnswering(t *testing.T) {
	t.Parallel()
	buffer := schemebuffer.New(schemebuffer.DefaultConfig(), schemeBufferBuffs(t), schemeBufferSkills(t))
	w, f := schemeBufferWorld(t, buffer, 0)
	buffer.Restore([]schemebuffer.Row{{OwnerID: w.player, Name: "$", Skills: "1035,9999"}}, zerolog.Nop())
	w.talkTo(t, f)

	priced := `$ [1 / 20] - cost: 1,000</font>`
	if page := schemePageOf(t, w.bypass(t, npcCommand(f, "support"))); !strings.Contains(page, priced) {
		t.Fatalf("support = %s, want the stored $ scheme priced 1,000", page)
	}
	schemePageOf(t, w.bypass(t, npcCommand(f, "editschemes Buffs $ 1")))
	edit := schemePageOf(t, w.bypass(t, npcCommand(f, "skillselect Buffs $ 9999 1")))
	if !strings.Contains(edit, "$</font> scheme holds 1 / 20 buffs.") {
		t.Fatalf("editor after an unoffered skill = %s, want the scheme at 1 buff", edit)
	}
	if page := schemePageOf(t, w.bypass(t, npcCommand(f, "support"))); !strings.Contains(page, priced) {
		t.Fatalf("support after the unoffered pick = %s, want the $ scheme priced 1,000", page)
	}
	page := schemePageOf(t, w.bypass(t, npcCommand(f, "createscheme Fight")))
	if !strings.Contains(page, priced) || !strings.Contains(page, `Fight [0 / 20]</font>`) {
		t.Fatalf("createscheme = %s, want both schemes listed", page)
	}
	page = schemePageOf(t, w.bypass(t, npcCommand(f, "deletescheme $")))
	if strings.Contains(page, priced) || !strings.Contains(page, `Fight [0 / 20]</font>`) {
		t.Fatalf("deletescheme = %s, want Fight alone", page)
	}
	if got, want := buffer.Rows(), []schemebuffer.Row{{OwnerID: w.player, Name: "Fight"}}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("schemes = %+v, want %+v", got, want)
	}
}
