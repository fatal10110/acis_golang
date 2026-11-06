package admin

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: AdminEffects.useAdminCommand admin_abnormal, admin_effect,
// admin_gmspeed and admin_jukebox, AdminEffects.showMainPage, and
// MagicSkillUse writeImpl.

const (
	effectsBuff  = 1020 // a 60 second buff
	effectsHaste = 7029 // the Super Haste toggle, levels 1 to 4
)

// effectsSkills is the skill table of the //effect and //gmspeed scenarios.
func effectsSkills(t *testing.T) gameservertest.Option {
	t.Helper()
	defs := []modelskill.Definition{{
		ID: effectsBuff, Level: 1, Name: "Buff", Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
	}}
	for level := 1; level <= 4; level++ {
		defs = append(defs, modelskill.Definition{
			ID: effectsHaste, Level: level, Name: "Super Haste", Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf,
			SkillType: "CONT", Effects: []modelskill.EffectTemplate{{Name: "ManaDamOverTime", Count: 0x7fffffff, Time: 10, Icon: true}},
		})
	}
	db := sqltest.SharedDB(t)
	return gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(defs), gamesql.NewCharacterSkillStore(db)))
}

// effectRow is one //effect list row: the row's band, the skill as a remove
// link and what is left of the effect.
func effectRow(row, id int, name, left string) string {
	band := "<table width=280><tr>"
	if row%2 == 0 {
		band = "<table width=280 bgcolor=000000><tr>"
	}
	return band + fmt.Sprintf(`<td width=220><a action="bypass -h admin_effect remove %d">%s</a></td><td width=60>%s</td>`, id, name, left) +
		`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`
}

// effectReply splits a //effect reply into its plain-text messages and its
// pages, other frames left out.
func effectReply(t *testing.T, frames [][]byte) (texts, pages []string) {
	t.Helper()
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeNpcHtmlMessage:
			pages = append(pages, htmlBody(t, f))
		case serverpackets.OpcodeSystemMessage:
			if r := wire.NewReader(f[1:]); r.ReadInt32() == serverpackets.SystemMessageS1 {
				_, text := systemText(t, f)
				texts = append(texts, text)
			}
		}
	}
	return texts, pages
}

// requireEffectReply fails unless frames tell exactly texts and open one
// page holding every one of parts, or no page when parts is nil.
func requireEffectReply(t *testing.T, what string, frames [][]byte, texts []string, parts ...string) string {
	t.Helper()
	gotTexts, pages := effectReply(t, frames)
	if strings.Join(gotTexts, "|") != strings.Join(texts, "|") {
		t.Fatalf("%s texts = %q, want %q", what, gotTexts, texts)
	}
	if parts == nil {
		if len(pages) != 0 {
			t.Fatalf("%s opened %d page(s), want none", what, len(pages))
		}
		return ""
	}
	if len(pages) != 1 {
		t.Fatalf("%s opened %d page(s), want one (frames %x)", what, len(pages), testsupport.FrameOpcodes(frames))
	}
	for _, part := range parts {
		if !strings.Contains(pages[0], part) {
			t.Fatalf("%s page = %q, want one holding %q", what, pages[0], part)
		}
	}
	return pages[0]
}

// effectIDs returns the skill id and level of every effect pc holds.
func effectIDs(t *testing.T, srv *gameservertest.Server, objID int32) map[int]int {
	t.Helper()
	held := map[int]int{}
	onLearnerQueue(t, srv, objID, func(pc *player.Character) {
		for _, e := range pc.EffectList().All() {
			held[int(e.Skill.ID)] = e.Skill.Level
		}
	})
	return held
}

// magicSkillUseBytes is MagicSkillUse writeImpl for a failed-crit cast of
// skill level 1, hit time 1 and no reuse from caster to target.
func magicSkillUseBytes(caster, target int32, cx, cy, cz, tx, ty, tz int, skill int32) []byte {
	w := wire.NewPacketWriter(0x48)
	for _, v := range []int32{caster, target, skill, 1, 1, 0, int32(cx), int32(cy), int32(cz), 0, int32(tx), int32(ty), int32(tz)} {
		w.WriteInt32(v)
	}
	return w.Bytes()
}

// TestAdminEffectsEffectPageSelf pins //effect on the GM itself: the list
// page with one row per effect, a toggle labelled as such; set landing a
// skill's effects, a well-formed set naming no skill answering the usage
// alone; remove ending one skill's effects without a message; visual
// showing the cast; each malformed form answering its usage before the
// page; remove 0 and a page past the int range doing nothing at all.
func TestAdminEffectsEffectPageSelf(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, effectsSkills(t),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "char_effects.htm")))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)
	run := func(command string) [][]byte {
		t.Helper()
		return append(exchange(t, gm, encodeBuildCmd(command)), settle(t, gm)...)
	}
	links := `<button action="bypass admin_effect 1"`

	requireEffectReply(t, "//effect", run("effect"), nil, "admin_debug Admin", links)
	if strings.Contains(requireEffectReply(t, "//effect", run("effect 2"), nil, links), "<td width=220>") {
		t.Fatal("//effect with no effects lists a row")
	}

	requireEffectReply(t, "//effect set buff", run("effect set 1020 1"), nil, effectRow(0, effectsBuff, "Buff", "60s"))
	requireEffectReply(t, "//effect set haste", run("effect set 7029 2 1"), nil,
		effectRow(0, effectsBuff, "Buff", "60s"), effectRow(1, effectsHaste, "Super Haste", "toggle"))
	if held := effectIDs(t, srv, gmID); held[effectsBuff] != 1 || held[effectsHaste] != 2 {
		t.Fatalf("GM effects after //effect set = %v, want the buff and haste level 2", held)
	}

	requireEffectReply(t, "//effect set unknown", run("effect set 1020 9"), []string{"Usage: //effect set id level [page]"})
	for _, bad := range []string{"effect set", "effect set 1020", "effect set x 1", "effect set 1020 x"} {
		requireEffectReply(t, "//"+bad, run(bad), []string{"Usage: //effect set id level [page]"}, links)
	}
	for _, bad := range []string{"effect remove", "effect remove x"} {
		requireEffectReply(t, "//"+bad, run(bad), []string{"Usage: //effect remove id [page]"}, links)
	}
	for _, bad := range []string{"effect visual", "effect visual x"} {
		requireEffectReply(t, "//"+bad, run(bad), []string{"Usage: //effect visual id"}, links)
	}
	for _, silent := range []string{"effect remove 0", "effect remove -5", "effect 2147483648"} {
		if frames := run(silent); len(frames) != 0 {
			t.Fatalf("//%s frames = %x, want none", silent, testsupport.FrameOpcodes(frames))
		}
	}
	requireEffectReply(t, "//effect remove 1020 2147483648", run("effect remove 1020 2147483648"), nil)
	// The removal before the out-of-range page still happened.
	if held := effectIDs(t, srv, gmID); held[effectsBuff] != 0 || held[effectsHaste] != 2 {
		t.Fatalf("GM effects after //effect remove 1020 = %v, want haste alone", held)
	}
	requireEffectReply(t, "//effect remove haste", run("effect remove 7029"), nil, links)
	if held := effectIDs(t, srv, gmID); len(held) != 0 {
		t.Fatalf("GM effects after removing all = %v, want none", held)
	}

	x, y, z := srv.PlayerPosition(t, gmID)
	frames := run("effect visual 1234")
	if !holdsFrame(frames, magicSkillUseBytes(gmID, gmID, x, y, z, x, y, z, 1234)) {
		t.Fatalf("//effect visual frames = %x, want the GM's MagicSkillUse of 1234", frames)
	}
	requireEffectReply(t, "//effect visual", frames, nil, links)

	// The page's own links reach the same command.
	requireEffectReply(t, "admin_effect bypass", append(exchange(t, gm, encodeBypass("admin_effect set 1020 1")), settle(t, gm)...),
		nil, effectRow(0, effectsBuff, "Buff", "60s"))
}

// TestAdminEffectsEffectPageOthers pins //effect on another creature: a
// selected player and NPC take the effects the GM sets, effector the GM,
// and the GM is told of each removal; the page carries the creature's
// name, and visual shows the creature casting on the GM to the players
// that know it.
func TestAdminEffectsEffectPageOthers(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, effectsSkills(t),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "char_effects.htm")))
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)
	onUser := func(command string) [][]byte {
		t.Helper()
		frames := exchange(t, gm, encodeBuildCmd(command))
		onLearnerQueue(t, srv, userID, func(*player.Character) {})
		return append(frames, settle(t, gm)...)
	}

	requireEffectReply(t, "//effect set on player", onUser("effect set 1020 1"), nil, "admin_debug Player", effectRow(0, effectsBuff, "Buff", "60s"))
	if held := effectIDs(t, srv, userID); held[effectsBuff] != 1 {
		t.Fatalf("player effects after //effect set = %v, want the buff", held)
	}
	if held := effectIDs(t, srv, gmID); len(held) != 0 {
		t.Fatalf("GM effects after //effect set on the player = %v, want none", held)
	}
	requireEffectReply(t, "//effect remove id on player", onUser("effect remove 1020"), []string{"You removed 1020 skillId effect from Player."}, "admin_debug Player")
	onUser("effect set 1020 1")
	requireEffectReply(t, "//effect remove all on player", onUser("effect remove all"), []string{"You removed all effects from Player."}, "admin_debug Player")
	if held := effectIDs(t, srv, userID); len(held) != 0 {
		t.Fatalf("player effects after //effect remove all = %v, want none", held)
	}
	settle(t, user)
	ux, uy, uz := srv.PlayerPosition(t, userID)
	gx, gy, gz := srv.PlayerPosition(t, gmID)
	visual := magicSkillUseBytes(userID, gmID, ux, uy, uz, gx, gy, gz, 1020)
	if frames := onUser("effect visual 1020"); !holdsFrame(frames, visual) {
		t.Fatalf("//effect visual GM frames = %x, want the player's cast", frames)
	}
	if frames := settle(t, user); !holdsFrame(frames, visual) {
		t.Fatalf("//effect visual target frames = %x, want its own cast", frames)
	}

	folk := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 30100), location.Location{X: gx + 40, Y: gy, Z: gz})
	drain(t, gm)
	exchange(t, gm, encodeAction(folk.ObjectID()))
	onFolk := func(command string) [][]byte {
		t.Helper()
		frames := exchange(t, gm, encodeBuildCmd(command))
		onParaQueue(t, folk.Queue(), func() {})
		return append(frames, settle(t, gm)...)
	}
	requireEffectReply(t, "//effect set on NPC", onFolk("effect set 1020 1"), nil, "admin_debug Folk", effectRow(0, effectsBuff, "Buff", "60s"))
	var held []*effect.Effect
	onParaQueue(t, folk.Queue(), func() { held = folk.EffectList().All() })
	if len(held) != 1 || held[0].Skill.ID != effectsBuff {
		t.Fatalf("NPC effects after //effect set = %v, want the buff", held)
	}
	requireEffectReply(t, "//effect remove all on NPC", onFolk("effect remove all"), []string{"You removed all effects from Folk."}, "admin_debug Folk")
	if len(folk.EffectList().All()) != 0 {
		t.Fatal("NPC still holds effects after //effect remove all")
	}
}

// TestAdminEffectsGMSpeed pins //gmspeed: Super Haste first ends, then
// levels 1 to 4 put that level on the GM, 0 and 5 nothing; a missing or
// malformed level answers the usage; the GM's icons are refreshed last in
// every case.
func TestAdminEffectsGMSpeed(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, effectsSkills(t))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)
	// lastIcons returns the skill levels the last AbnormalStatusUpdate of
	// frames shows, failing without one.
	lastIcons := func(what string, frames [][]byte) map[int32]int {
		t.Helper()
		var icons map[int32]int
		for _, f := range frames {
			if f[0] != serverpackets.OpcodeAbnormalStatusUpdate {
				continue
			}
			icons = map[int32]int{}
			r := wire.NewReader(f[1:])
			for range r.ReadUint16() {
				id, level := r.ReadInt32(), r.ReadUint16()
				r.ReadInt32()
				icons[id] = int(level)
			}
		}
		if icons == nil {
			t.Fatalf("%s frames = %x, want an AbnormalStatusUpdate", what, testsupport.FrameOpcodes(frames))
		}
		return icons
	}

	for _, tc := range []struct {
		command string
		level   int // 0: no haste
		usage   bool
	}{
		{"gmspeed 2", 2, false},
		{"gmspeed 4", 4, false},
		{"gmspeed 0", 0, false},
		{"gmspeed 1", 1, false},
		{"gmspeed 5", 0, false},
		{"gmspeed 3", 3, false},
		{"gmspeed x", 0, true},
		{"gmspeed 1", 1, false},
		{"gmspeed", 0, true},
	} {
		frames := exchange(t, gm, encodeBuildCmd(tc.command))
		texts, _ := effectReply(t, frames)
		if tc.usage {
			if len(texts) != 1 || texts[0] != "Use: //gmspeed value (0-4)." {
				t.Fatalf("//%s texts = %q, want the usage", tc.command, texts)
			}
			if frames[len(frames)-1][0] != serverpackets.OpcodeAbnormalStatusUpdate {
				t.Fatalf("//%s frames = %x, want the icon refresh after the usage", tc.command, testsupport.FrameOpcodes(frames))
			}
		}
		if icon := lastIcons("//"+tc.command, frames)[effectsHaste]; icon != tc.level {
			t.Fatalf("//%s haste icon level = %d, want %d", tc.command, icon, tc.level)
		}
		if held := effectIDs(t, srv, gmID); held[effectsHaste] != tc.level {
			t.Fatalf("//%s haste level held = %d, want %d", tc.command, held[effectsHaste], tc.level)
		}
	}
}

// TestAdminEffectsJukebox pins //jukebox: the first sound list page.
func TestAdminEffectsJukebox(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, "songs/songs.htm")))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)
	assertPage(t, exchange(t, gm, encodeBuildCmd("jukebox")), "jukeBox -- Main")
	assertPage(t, exchange(t, gm, encodeBypass("admin_jukebox")), "admin_play_sound ls01_f")
}

// abnormalMasks returns the abnormal mask each frame of opcode about objID
// shows, read as paraShown reads it.
func abnormalMasks(frames [][]byte, opcode byte, objID int32) []uint32 {
	var masks []uint32
	for _, f := range frames {
		if len(f) == 0 || f[0] != opcode {
			continue
		}
		var id int32
		var tail int
		switch opcode {
		case serverpackets.OpcodeUserInfo:
			tail = 77
		case serverpackets.OpcodeNPCInfo:
			id, tail = int32(binary.LittleEndian.Uint32(f[1:5])), 46
		}
		if id == objID {
			masks = append(masks, binary.LittleEndian.Uint32(f[len(f)-tail:]))
		}
	}
	return masks
}

// TestAdminEffectsAbnormal pins //abnormal: a creature already showing
// every bit of the hex mask stops showing them, any other starts, the GM
// itself without a selection and a selected NPC otherwise; a missing or
// malformed mask answers the usage.
func TestAdminEffectsAbnormal(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)
	last := func(what string, frames [][]byte, opcode byte, objID int32) uint32 {
		t.Helper()
		masks := abnormalMasks(frames, opcode, objID)
		if len(masks) == 0 {
			t.Fatalf("%s frames = %x, want the creature shown again", what, testsupport.FrameOpcodes(frames))
		}
		return masks[len(masks)-1]
	}

	for _, tc := range []struct {
		command string
		want    uint32
	}{
		{"abnormal 1", 0x1},
		{"abnormal 3", 0x3}, // not every bit shown: both start
		{"abnormal 3", 0x0}, // every bit shown: both stop
		{"abnormal 0000000A", 0xa},
		{"abnormal a", 0x0},
	} {
		frames := append(exchange(t, gm, encodeBuildCmd(tc.command)), settle(t, gm)...)
		if got := last("//"+tc.command, frames, serverpackets.OpcodeUserInfo, 0) & 0xf; got != tc.want {
			t.Fatalf("//%s GM mask = %#x, want %#x", tc.command, got, tc.want)
		}
	}
	for _, bad := range []string{"abnormal", "abnormal zz", "abnormal -1", "abnormal +1", "abnormal 0x1", "abnormal 80000000"} {
		assertTexts(t, exchange(t, gm, encodeBuildCmd(bad)), "Usage: //abnormal mask")
	}

	x, y, z := srv.PlayerPosition(t, gmID)
	h := srv.SpawnHostileNPCAt(t, location.Location{X: x + 60, Y: y, Z: z})
	drain(t, gm)
	exchange(t, gm, encodeAction(h.ObjectID()))
	for _, want := range []uint32{0x2, 0x0} {
		frames := exchange(t, gm, encodeBypass("admin_abnormal 2"))
		onParaQueue(t, h.Queue(), func() {})
		frames = append(frames, settle(t, gm)...)
		if got := last("admin_abnormal 2 on NPC", frames, serverpackets.OpcodeNPCInfo, h.ObjectID()) & 0x2; got != want {
			t.Fatalf("NPC mask = %#x, want %#x", got, want)
		}
		if got := uint32(h.AbnormalEffect()) & 0x2; got != want {
			t.Fatalf("NPC abnormal state = %#x, want %#x", got, want)
		}
	}
}
