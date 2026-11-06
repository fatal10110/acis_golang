package admin

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// opcodesOf returns the opcodes of frames.
func opcodesOf(frames [][]byte) []byte {
	ops := make([]byte, 0, len(frames))
	for _, f := range frames {
		ops = append(ops, f[0])
	}
	return ops
}

// requireNoSystemMessage fails when frames tell the GM anything.
func requireNoSystemMessage(t *testing.T, what string, frames [][]byte) {
	t.Helper()
	if slices.Contains(opcodesOf(frames), serverpackets.OpcodeSystemMessage) {
		t.Fatalf("%s told the GM %x, want nothing", what, testsupport.FrameOpcodes(frames))
	}
}

// TestAdminManagePlayerKillResHeal pins //kill, //res and //heal on the
// selected player: the kill puts it to death, its client seeing it die; the
// res stands it up, short of full HP; the heal fills its HP and CP. The GM
// is told nothing of the res or the heal; the kill credits the GM as any
// kill does.
func TestAdminManagePlayerKillResHeal(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)
	pc := onlineCharacter(t, srv, userID)
	vitals := func() player.Resources {
		var res player.Resources
		onLearnerQueue(t, srv, userID, func(pc *player.Character) { res = pc.ResourceValues() })
		return res
	}

	exchange(t, gm, encodeBuildCmd("kill"))
	if frames := settle(t, user); !slices.Contains(opcodesOf(frames), serverpackets.OpcodeDie) {
		t.Fatalf("killed player frames = %x, want Die", testsupport.FrameOpcodes(frames))
	}
	if !pc.Dead() {
		t.Fatal("player alive after //kill")
	}

	requireNoSystemMessage(t, "//res", exchange(t, gm, encodeBuildCmd("res")))
	if frames := settle(t, user); !slices.Contains(opcodesOf(frames), serverpackets.OpcodeRevive) {
		t.Fatalf("revived player frames = %x, want Revive", testsupport.FrameOpcodes(frames))
	}
	if pc.Dead() {
		t.Fatal("player dead after //res")
	}
	if res := vitals(); res.CurrentHP >= res.MaxHP {
		t.Fatalf("revived HP = %v of %v, want the revive share", res.CurrentHP, res.MaxHP)
	}

	requireNoSystemMessage(t, "//heal", exchange(t, gm, encodeBuildCmd("heal")))
	settle(t, user)
	if res := vitals(); res.CurrentHP != res.MaxHP || res.CurrentCP != res.MaxCP {
		t.Fatalf("healed HP %v/%v CP %v/%v, want both full", res.CurrentHP, res.MaxHP, res.CurrentCP, res.MaxCP)
	}
}

// TestAdminManageSelfAndName pins the creature //kill acts on: with nothing
// selected it is the GM, whom //kill spares; a name picks that online player
// whatever is selected, matched ignoring case; and a radius that is no
// number stops the command before it acts, telling the GM nothing.
func TestAdminManageSelfAndName(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	pc := onlineCharacter(t, srv, userID)

	requireNoSystemMessage(t, "//kill on self", exchange(t, gm, encodeBuildCmd("kill")))
	if onlineCharacter(t, srv, gmID).Dead() {
		t.Fatal("//kill with nothing selected killed the GM")
	}

	if frames := exchange(t, gm, encodeBuildCmd("kill player abc")); len(frames) != 0 {
		t.Fatalf("//kill player abc frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	settle(t, user)
	if pc.Dead() {
		t.Fatal("//kill with a bad radius killed the player")
	}

	exchange(t, gm, encodeBypass("admin_kill player"))
	settle(t, user)
	if !pc.Dead() {
		t.Fatal("//kill player left the named player alive")
	}
}

// TestAdminManageCancelStopsEffects pins //cancel on the selected player:
// its effects end and its icon list empties.
func TestAdminManageCancelStopsEffects(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	onLearnerQueue(t, srv, userID, func(pc *player.Character) {
		e, err := effect.New(effect.Skill{ID: 1040, Level: 1, SkillType: "BUFF"}, modelskill.EffectTemplate{Name: "Buff", Time: 60, Icon: true})
		if err != nil {
			t.Errorf("effect.New(Buff): %v", err)
			return
		}
		e.Effector, e.Effected = pc, pc
		pc.EffectList().Add(e)
	})
	settle(t, user)
	selectPlayer(t, gm, user, userID)

	requireNoSystemMessage(t, "//cancel", exchange(t, gm, encodeBuildCmd("cancel")))
	if frames := settle(t, user); !slices.Contains(opcodesOf(frames), serverpackets.OpcodeAbnormalStatusUpdate) {
		t.Fatalf("cancelled player frames = %x, want its icon list", testsupport.FrameOpcodes(frames))
	}
	onLearnerQueue(t, srv, userID, func(pc *player.Character) {
		if n := len(pc.EffectList().All()); n != 0 {
			t.Errorf("effects after //cancel = %d, want none", n)
		}
	})
}

// TestAdminManageMonsterHeal pins //heal on a selected monster: its HP and
// MP fill.
func TestAdminManageMonsterHeal(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	x, y, z := srv.PlayerPosition(t, gmID)
	h := srv.SpawnHostileNPCAt(t, location.Location{X: x + 60, Y: y, Z: z})
	drain(t, gm)
	onParaQueue(t, h.Queue(), func() {
		h.SetHP(10)
		h.ReduceMP(h.MPValue())
	})
	exchange(t, gm, encodeAction(h.ObjectID()))

	requireNoSystemMessage(t, "//heal", exchange(t, gm, encodeBuildCmd("heal")))
	onParaQueue(t, h.Queue(), func() {
		if h.HP() != h.MaxHPValue() || h.MPValue() != h.MaxMPValue() {
			t.Errorf("healed monster HP %v/%v MP %v/%v, want both full", h.HP(), h.MaxHPValue(), h.MPValue(), h.MaxMPValue())
		}
	})
}

// TestAdminManageRadius pins //kill and //res over a radius around the GM:
// the monster in range dies, one out of range lives, and the GM is spared.
// A dead monster cannot be revived yet, so //res over it answers
// ActionFailed and leaves it dead.
func TestAdminManageRadius(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	x, y, z := srv.PlayerPosition(t, gmID)
	near := srv.SpawnHostileNPCAt(t, location.Location{X: x + 100, Y: y, Z: z})
	far := srv.SpawnHostileNPCAt(t, location.Location{X: x + 900, Y: y, Z: z})
	drain(t, gm)

	exchange(t, gm, encodeBuildCmd("kill 300"))
	onParaQueue(t, near.Queue(), func() {})
	onParaQueue(t, far.Queue(), func() {})
	if !near.Dead() || far.Dead() {
		t.Fatalf("after //kill 300: near dead %v, far dead %v, want true, false", near.Dead(), far.Dead())
	}
	if onlineCharacter(t, srv, gmID).Dead() {
		t.Fatal("//kill 300 killed the GM")
	}
	drain(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("res 300"))
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("//res 300 over a dead monster frames = %x, want ActionFailed alone", testsupport.FrameOpcodes(frames))
	}
	if !near.Dead() {
		t.Fatal("//res 300 revived the monster")
	}
}
