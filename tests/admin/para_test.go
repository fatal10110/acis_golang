package admin

import (
	"encoding/binary"
	"slices"
	"testing"
	"time"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// paraFolkHeal is the aura heal the civilian NPC casts in the //para
// scenario; paraOtherSkill only ever waits for its reuse.
const (
	paraFolkHeal   = modelskill.ID(9201)
	paraOtherSkill = modelskill.ID(9299)
)

// onParaQueue runs fn on q and waits for it to finish.
func onParaQueue(t *testing.T, q interface{ Post(func()) bool }, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !q.Post(func() { fn(); close(done) }) {
		t.Fatal("post to queue: queue closed")
	}
	<-done
}

// paraHeld reports whether an abnormal-effect mask shows the hold visual.
func paraHeld(abnormal uint32) bool { return abnormal&modelskill.AbnormalHold2 != 0 }

// paraShown returns whether each frame of opcode about objID shows the hold
// visual. UserInfo (objID 0) carries a fixed 73 bytes after its abnormal
// field, CharInfo 60 and NpcInfo 42.
func paraShown(frames [][]byte, opcode byte, objID int32) []bool {
	var shown []bool
	for _, f := range frames {
		if len(f) == 0 || f[0] != opcode {
			continue
		}
		var id int32
		var tail int
		switch opcode {
		case serverpackets.OpcodeUserInfo:
			tail = 77
		case serverpackets.OpcodeCharInfo:
			id, tail = int32(binary.LittleEndian.Uint32(f[17:21])), 64
		case serverpackets.OpcodeNPCInfo:
			id, tail = int32(binary.LittleEndian.Uint32(f[1:5])), 46
		}
		if id == objID {
			shown = append(shown, paraHeld(binary.LittleEndian.Uint32(f[len(f)-tail:])))
		}
	}
	return shown
}

// requireShown fails unless frames hold a frame of opcode about objID, and
// every such frame shows the hold visual exactly when held. A monster let
// go takes up its intention again, which may show it once more.
func requireShown(t *testing.T, what string, frames [][]byte, opcode byte, objID int32, held bool) {
	t.Helper()
	shown := paraShown(frames, opcode, objID)
	if len(shown) == 0 || slices.Contains(shown, !held) {
		t.Fatalf("%s: frames %x show hold %v, want frames of opcode %#x showing hold %v", what, testsupport.FrameOpcodes(frames), shown, opcode, held)
	}
}

// castsOf counts the MagicSkillUse frames frames hold of caster.
func castsOf(frames [][]byte, caster int32) int {
	n := 0
	for _, f := range frames {
		if len(f) >= 5 && f[0] == serverpackets.OpcodeMagicSkillUse && int32(binary.LittleEndian.Uint32(f[1:5])) == caster {
			n++
		}
	}
	return n
}

// TestAdminParaWithoutTargetIsInvalid pins //para with nothing selected:
// INVALID_TARGET alone.
func TestAdminParaWithoutTargetIsInvalid(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("para"))
	if len(frames) != 1 {
		t.Fatalf("//para frames = %x, want INVALID_TARGET alone", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)
}

// TestAdminParaTogglesPlayer pins //para on a selected player: the first
// holds it, shown held to itself and to the GM, and stops its actions; the
// second lets it go, shown no longer held. The GM is told nothing.
func TestAdminParaTogglesPlayer(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)
	obj, ok := srv.State.Player(userID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}

	for _, held := range []bool{true, false} {
		gmFrames := exchange(t, gm, encodeBuildCmd("para"))
		for _, f := range gmFrames {
			if f[0] == serverpackets.OpcodeSystemMessage {
				t.Fatalf("//para told the GM %x, want nothing", f)
			}
		}
		userFrames := settle(t, user)
		gmFrames = append(gmFrames, settle(t, gm)...)
		requireShown(t, "player's own view", userFrames, serverpackets.OpcodeUserInfo, 0, held)
		requireShown(t, "GM's view", gmFrames, serverpackets.OpcodeCharInfo, userID, held)
		if pc.Paralyzed() != held {
			t.Fatalf("player paralyzed = %v, want %v", pc.Paralyzed(), held)
		}
	}
}

// TestAdminParaTogglesMonster pins //para on a selected monster: the first
// holds it, shown held to the players around it, the second lets it go.
func TestAdminParaTogglesMonster(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	x, y, z := srv.PlayerPosition(t, gmID)
	h := srv.SpawnHostileNPCAt(t, location.Location{X: x + 60, Y: y, Z: z})
	drain(t, gm)
	exchange(t, gm, encodeAction(h.ObjectID()))

	for _, held := range []bool{true, false} {
		frames := exchange(t, gm, encodeBuildCmd("para"))
		onParaQueue(t, h.Queue(), func() {})
		requireShown(t, "monster", append(frames, settle(t, gm)...), serverpackets.OpcodeNPCInfo, h.ObjectID(), held)
		if h.Paralyzed() != held {
			t.Fatalf("monster paralyzed = %v, want %v", h.Paralyzed(), held)
		}
	}
}

// TestAdminParaLocksFolkSkills pins //para on a selected civilian NPC: it
// is shown held and can neither move nor swing, and while any of its
// skills waits for its reuse it may use none, so a cast desire draws no
// cast. Let go, it is shown no longer held and casts.
func TestAdminParaLocksFolkSkills(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithAITask())
	gm := srv.Client
	enterWorld(t, gm)
	x, y, z := srv.PlayerPosition(t, gmID)
	def := modelskill.Definition{
		ID: paraFolkHeal, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetAura, Radius: 200, SkillType: "HEAL", Power: 50,
		StaticHitTime: true, StaticReuse: true,
	}
	tmpl := gameservertest.FolkTemplate("Folk", 30100)
	tmpl.CanMove = true
	folk := srv.SpawnCastingFolkNPCAt(t, tmpl,
		location.Location{X: x + 40, Y: y, Z: z}, modelskill.NewTable([]modelskill.Definition{def}))
	tick := func() [][]byte {
		t.Helper()
		if err := srv.AI.Tick(); err != nil {
			t.Fatalf("AI.Tick() = %v", err)
		}
		onParaQueue(t, folk.Queue(), func() {})
		return settle(t, gm)
	}
	for range 3 {
		tick()
	}
	drain(t, gm)
	exchange(t, gm, encodeAction(folk.ObjectID()))
	ref := modelskill.Ref{ID: paraFolkHeal, Level: 1}
	ownKey := actorcast.ReuseKey(def)
	folk.AddCastDesire(folk, ref, 1_000_000, true, true)
	if frames := tick(); castsOf(frames, folk.ObjectID()) != 1 {
		t.Fatalf("folk not held: frames %x, want its heal cast", testsupport.FrameOpcodes(frames))
	}
	srv.AdvanceUntil(t, "folk cast end", func() bool { return !folk.CastingNow() })

	frames := exchange(t, gm, encodeBuildCmd("para"))
	onParaQueue(t, folk.Queue(), func() {})
	requireShown(t, "held folk", append(frames, settle(t, gm)...), serverpackets.OpcodeNPCInfo, folk.ObjectID(), true)
	if !folk.Paralyzed() || !folk.MovementDisabled() || !folk.AttackDisabled() {
		t.Fatalf("held folk paralyzed %v, movement disabled %v, attack disabled %v, want all true",
			folk.Paralyzed(), folk.MovementDisabled(), folk.AttackDisabled())
	}
	if folk.SkillDisabled(ownKey) {
		t.Fatal("held folk with no skill waiting refuses its heal")
	}
	onParaQueue(t, folk.Queue(), func() {
		folk.DisableSkill(actorcast.ReuseKey(modelskill.Definition{ID: paraOtherSkill, Level: 1}), time.Minute)
	})
	if !folk.SkillDisabled(ownKey) {
		t.Fatal("held folk with another skill waiting may use its heal")
	}
	folk.AddCastDesire(folk, ref, 1_000_000, true, true)
	if frames := tick(); castsOf(frames, folk.ObjectID()) != 0 {
		t.Fatalf("held folk cast: frames %x", testsupport.FrameOpcodes(frames))
	}

	frames = exchange(t, gm, encodeBuildCmd("para"))
	onParaQueue(t, folk.Queue(), func() {})
	requireShown(t, "released folk", append(frames, settle(t, gm)...), serverpackets.OpcodeNPCInfo, folk.ObjectID(), false)
	if folk.Paralyzed() || folk.MovementDisabled() || folk.AttackDisabled() || folk.SkillDisabled(ownKey) {
		t.Fatalf("released folk paralyzed %v, movement disabled %v, attack disabled %v, heal disabled %v, want all false",
			folk.Paralyzed(), folk.MovementDisabled(), folk.AttackDisabled(), folk.SkillDisabled(ownKey))
	}
	folk.AddCastDesire(folk, ref, 1_000_000, true, true)
	if frames := tick(); castsOf(frames, folk.ObjectID()) != 1 {
		t.Fatalf("released folk frames %x, want its heal cast", testsupport.FrameOpcodes(frames))
	}
}
