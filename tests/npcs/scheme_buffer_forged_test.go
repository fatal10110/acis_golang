package npcs

import (
	"strconv"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
)

// TestSchemeBufferRefusesLinkBreakingNames pins the owner-approved
// divergence of #5153: a scheme name holding '$', '"', '<', '>' or a
// control character is refused with the name notice, and no scheme is
// made.
func TestSchemeBufferRefusesLinkBreakingNames(t *testing.T) {
	t.Parallel()
	buffer := schemebuffer.New(schemebuffer.DefaultConfig(), schemeBufferBuffs(t), schemeBufferSkills(t))
	w, f := schemeBufferWorld(t, buffer, 0)
	w.talkTo(t, f)
	schemePageOf(t, w.bypass(t, npcCommand(f, "support")))
	for _, name := range []string{"$", "a$b", `a"b`, "a<b", "a>b", "a\x01b", "ab\x7f"} {
		assertFrames(t, "createscheme "+strconv.Quote(name), w.bypass(t, npcCommand(f, "createscheme "+name)),
			noticeFrame("Scheme's name must contain up to 14 chars. Spaces are trimmed."), []byte{serverpackets.OpcodeActionFailed})
	}
	if rows := buffer.Rows(); len(rows) != 0 {
		t.Fatalf("schemes after the refused names = %+v, want none", rows)
	}
}

// TestSchemeBufferChargesItsOwnFee pins the owner-approved divergence of
// #5153 on a scheme whose '$' name predates the name rule: its links on
// the schemes page widen into prefixes the page whitelist admits with any
// fee, yet the give takes the scheme's own fee whatever the command
// names, and the editor adds no skill the buffer does not offer.
func TestSchemeBufferChargesItsOwnFee(t *testing.T) {
	t.Parallel()
	buffer := schemebuffer.New(schemebuffer.DefaultConfig(), schemeBufferBuffs(t), schemeBufferSkills(t))
	w, f := schemeBufferWorld(t, buffer, 5000)
	buffer.Restore([]schemebuffer.Row{{OwnerID: w.player, Name: "$", Skills: "1035,1068"}}, zerolog.Nop())
	oid := strconv.Itoa(int(f.ObjectID()))
	w.talkTo(t, f)

	page := schemePageOf(t, w.bypass(t, npcCommand(f, "support")))
	if !strings.Contains(page, `<a action="bypass npc_`+oid+`_givebuffs $ 3500">Use on Me</a>`) {
		t.Fatalf("schemes page = %s, want the $ scheme priced 3,500", page)
	}
	frames := w.bypass(t, npcCommand(f, "givebuffs $ 0"))
	if len(frames) < 2 || string(frames[0]) != string(sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(3500))) ||
		frames[len(frames)-1][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("givebuffs with fee 0 = %x, want the scheme's fee taken first and the release last", opcodes(frames))
	}
	if got := w.savedCount(t, item.AdenaID); got != 1500 {
		t.Fatalf("adena saved = %d, want 1500", got)
	}
	for id, want := range map[int]int{1035: 4, 1068: 3} {
		if level, ok := w.activeBuff(t, id); !ok || level != want {
			t.Fatalf("buff %d = level %d (%v), want level %d", id, level, ok, want)
		}
	}
	// A negative fee is no free pass either: 1,500 adena does not cover
	// the scheme.
	assertFrames(t, "givebuffs with a negative fee", w.bypass(t, npcCommand(f, "givebuffs $ -5")),
		sysMsg(serverpackets.SystemMessageYouNotEnoughAdena), []byte{serverpackets.OpcodeActionFailed})
	if got := w.savedCount(t, item.AdenaID); got != 1500 {
		t.Fatalf("adena saved after the refused give = %d, want 1500", got)
	}

	// The editor's links widen the same way; a skill the buffer does not
	// offer is not added, and the editor shows again.
	schemePageOf(t, w.bypass(t, npcCommand(f, "editschemes Buffs $ 1")))
	edit := schemePageOf(t, w.bypass(t, npcCommand(f, "skillselect Buffs $ 4 1")))
	if !strings.Contains(edit, "$</font> scheme holds 2 / 20 buffs.") {
		t.Fatalf("editor after an unoffered skill = %s, want the scheme still at 2 buffs", edit)
	}
	if got, want := buffer.Rows(), []schemebuffer.Row{{OwnerID: w.player, Name: "$", Skills: "1035,1068"}}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("schemes = %+v, want %+v", got, want)
	}
}
