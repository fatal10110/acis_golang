package admin

import (
	"encoding/binary"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Ids of the decoration and signet fixtures.
const (
	killTreeID     = 13006
	killSignetID   = 454
	killPointNpcID = 13018
)

// killTemplates are a Christmas tree with a corpse time, mortal unless
// undying, and a signet effect point.
func killTemplates(undying bool) *npc.Table {
	return npc.NewTable([]*npc.Template{
		{
			ID: killTreeID, TemplateID: killTreeID, Type: "ChristmasTree", Name: "Christmas Tree", Level: 1,
			HPMax: 1000, CorpseTime: reviveCorpseTime, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true,
			Undying: undying,
		},
		{ID: killPointNpcID, TemplateID: killPointNpcID, Type: "EffectPoint", CollisionRadius: 5, RunSpeed: 20, WalkSpeed: 50},
	})
}

// bootTreeAdmin boots an admin in the world on a decay clock the test
// moves, then has it //spawn one Christmas tree on itself, which it
// returns.
func bootTreeAdmin(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, int32, *manualDecay, *npc.Decoration) {
	t.Helper()
	decay := newManualDecay(t)
	opts = append([]gameservertest.Option{
		gameservertest.WithNPCs(killTemplates(false)), gameservertest.WithNpcSpawns(nil), gameservertest.WithDecay(decay.task),
	}, opts...)
	srv, gmID := bootAdmin(t, adminLevel, opts...)
	decay.state = srv.State
	enterWorld(t, srv.Client)
	exchange(t, srv.Client, encodeBuildCmd("spawn 13006"))
	trees := npcsOf(srv, killTreeID)
	if len(trees) != 1 {
		t.Fatalf("trees in the world = %d, want 1", len(trees))
	}
	tree, ok := trees[0].(*npc.Decoration)
	if !ok {
		t.Fatalf("tree is %T, want a decoration", trees[0])
	}
	drain(t, srv.Client)
	return srv, gmID, decay, tree
}

// countFramesFor counts the frames of opcode op naming the object id.
func countFramesFor(frames [][]byte, op byte, id int32) int {
	n := 0
	for _, f := range frames {
		if len(f) >= 5 && f[0] == op && int32(binary.LittleEndian.Uint32(f[1:5])) == id {
			n++
		}
	}
	return n
}

// manageAround runs //<cmd> over a radius around the GM, which selects
// nothing, and returns every frame it caused once tree's queue ran it.
func manageAround(t *testing.T, srv *gameservertest.Server, tree *npc.Decoration, cmd string) [][]byte {
	t.Helper()
	frames := exchange(t, srv.Client, encodeBuildCmd(cmd+" 300"))
	if q := tree.Queue(); q != nil {
		onParaQueue(t, q, func() {})
	}
	return append(frames, settle(t, srv.Client)...)
}

// TestAdminManageDecorationKillRes pins //kill then //res on a Christmas
// tree: the kill shows its watchers its death and registers its corpse for
// the template corpse time, a second kill finds it dead and does nothing,
// and the res drops the corpse's decay and stands it up with the configured
// share of its HP. Nobody can select a tree, so no StatusUpdate goes out.
// The revived tree stays in the world past its old deadline, and a //heal
// fills its HP.
func TestAdminManageDecorationKillRes(t *testing.T) {
	t.Parallel()
	srv, _, decay, tree := bootTreeAdmin(t)

	frames := manageAround(t, srv, tree, "res")
	requireNoRevive(t, "//res on a living tree", frames)
	if tree.Dead() || tree.HP() != tree.MaxHPValue() {
		t.Fatalf("living tree after //res: dead %v HP %v, want alive at %v", tree.Dead(), tree.HP(), tree.MaxHPValue())
	}

	frames = manageAround(t, srv, tree, "kill")
	if got := countFramesFor(frames, serverpackets.OpcodeDie, tree.ObjectID()); got != 1 {
		t.Fatalf("//kill frames = %x, want one Die of the tree", testsupport.FrameOpcodes(frames))
	}
	if n := countFramesFor(frames, serverpackets.OpcodeStatusUpdate, tree.ObjectID()); n != 0 {
		t.Fatalf("//kill sent %d StatusUpdates of the tree, want none: nobody selects it", n)
	}
	requireNoSystemMessage(t, "//kill", frames)
	if !tree.Dead() || tree.HP() != 0 || !decay.task.Tracked(tree) {
		t.Fatalf("after //kill: dead %v HP %v decay tracked %v, want dead at 0 HP with its corpse tracked", tree.Dead(), tree.HP(), decay.task.Tracked(tree))
	}
	deadline, _ := decay.task.Deadline(tree)
	if want := decay.clock().Add(reviveCorpseTime * time.Second); !deadline.Equal(want) {
		t.Fatalf("corpse deadline %v, want the template corpse time on: %v", deadline, want)
	}

	frames = manageAround(t, srv, tree, "kill")
	if got := countFramesFor(frames, serverpackets.OpcodeDie, tree.ObjectID()); got != 0 {
		t.Fatalf("second //kill frames = %x, want no Die: a tree dies once", testsupport.FrameOpcodes(frames))
	}

	frames = manageAround(t, srv, tree, "res")
	if got := countFramesFor(frames, serverpackets.OpcodeRevive, tree.ObjectID()); got != 1 {
		t.Fatalf("//res frames = %x, want one Revive of the tree", testsupport.FrameOpcodes(frames))
	}
	requireNoSystemMessage(t, "//res", frames)
	if tree.Dead() || tree.HP() != revivedHP(tree.MaxHPValue()) || decay.task.Tracked(tree) {
		t.Fatalf("revived tree: dead %v HP %v decay tracked %v, want alive at %v with no decay", tree.Dead(), tree.HP(), decay.task.Tracked(tree), revivedHP(tree.MaxHPValue()))
	}

	decay.passAndTick(t, srv, 2*reviveCorpseTime*time.Second)
	if obj, ok := srv.State.Object(tree.ObjectID()); !ok || obj != world.Tracked(tree) || tree.Dead() {
		t.Fatal("revived tree left the world at its old corpse decay")
	}

	manageAround(t, srv, tree, "heal")
	if tree.HP() != tree.MaxHPValue() {
		t.Fatalf("revived tree after //heal HP %v, want its maximum %v", tree.HP(), tree.MaxHPValue())
	}
}

// TestAdminManageDecorationCorpseDecays pins a killed Christmas tree's
// corpse: it stays until its corpse time is up, then leaves the world for
// good, with no respawn although its //spawn declared one. A //res on it
// then finds nothing to stand up.
func TestAdminManageDecorationCorpseDecays(t *testing.T) {
	t.Parallel()
	srv, _, decay, tree := bootTreeAdmin(t)
	manageAround(t, srv, tree, "kill")
	if tree.RespawnDelay <= 0 {
		t.Fatalf("//spawn declared respawn %v, want a positive delay", tree.RespawnDelay)
	}

	decay.passAndTick(t, srv, reviveCorpseTime*time.Second-time.Second)
	if _, ok := srv.State.Object(tree.ObjectID()); !ok {
		t.Fatal("tree corpse left the world before its corpse time")
	}
	drain(t, srv.Client)
	decay.passAndTick(t, srv, time.Second)
	frames := settle(t, srv.Client)
	if _, ok := srv.State.Object(tree.ObjectID()); ok {
		t.Fatal("tree corpse still in the world past its corpse time")
	}
	if got := countFramesFor(frames, serverpackets.OpcodeDeleteObject, tree.ObjectID()); got != 1 {
		t.Fatalf("decay frames = %x, want one DeleteObject of the tree", testsupport.FrameOpcodes(frames))
	}
	if decay.task.Tracked(tree) {
		t.Fatal("decayed tree still tracked by the decay task")
	}

	// The decayed tree's queue is closed: there is nothing left to wait on.
	frames = append(exchange(t, srv.Client, encodeBuildCmd("res 300")), settle(t, srv.Client)...)
	requireNoRevive(t, "//res after the decay", frames)
	decay.passAndTick(t, srv, time.Hour)
	if trees := npcsOf(srv, killTreeID); len(trees) != 0 {
		t.Fatalf("trees in the world after the decay = %d, want none: a decoration never respawns", len(trees))
	}
}

// TestAdminManageDecorationCorpseDeleted pins //unspawnall on a tree
// corpse: the tree leaves the world and its pending decay goes with it.
func TestAdminManageDecorationCorpseDeleted(t *testing.T) {
	t.Parallel()
	srv, _, decay, tree := bootTreeAdmin(t)
	manageAround(t, srv, tree, "kill")
	if !decay.task.Tracked(tree) {
		t.Fatal("killed tree's corpse not tracked by the decay task")
	}
	exchange(t, srv.Client, encodeBuildCmd("unspawnall"))
	srv.Settle(t)
	if _, ok := srv.State.Object(tree.ObjectID()); ok || decay.task.Tracked(tree) {
		t.Fatalf("after //unspawnall: tree in world %v, decay tracked %v, want neither", ok, decay.task.Tracked(tree))
	}
}

// TestAdminManageSignetPointKill pins //kill over a signet effect point:
// the point is invulnerable, so only the effect strip runs, and its signet
// ending takes the point out of the world. It never dies: no Die, no
// corpse, and a //res finds nothing to stand up.
func TestAdminManageSignetPointKill(t *testing.T) {
	t.Parallel()
	def := modelskill.Definition{
		ID: killSignetID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "SIGNET", EffectNpcID: killPointNpcID, Radius: 180,
		Effects: []modelskill.EffectTemplate{{Name: "Signet", Count: 1, Time: 600}},
	}
	db := sqltest.SharedDB(t)
	decay := newManualDecay(t)
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithNPCs(killTemplates(false)), gameservertest.WithDecay(decay.task),
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{def}), gamesql.NewCharacterSkillStore(db))),
	)
	decay.state = srv.State
	if err := srv.KnownSkills.SetKnownSkill(t.Context(), gmID, 0, killSignetID, 1); err != nil {
		t.Fatalf("seed signet: %v", err)
	}
	gm := srv.Client
	enterWorld(t, gm)
	gm.Send(encodeMagicSkillUse(killSignetID))
	var point *npc.EffectPoint
	srv.AdvanceUntil(t, "signet effect point", func() bool {
		for _, obj := range srv.State.Objects() {
			if ep, ok := obj.(*npc.EffectPoint); ok {
				point = ep
			}
		}
		return point != nil
	})
	srv.Settle(t)
	drain(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("kill 300"))
	frames = append(frames, settle(t, gm)...)
	if _, ok := srv.State.Object(point.ObjectID()); ok {
		t.Fatal("signet point still in the world after //kill")
	}
	if got := countFramesFor(frames, serverpackets.OpcodeDie, point.ObjectID()); got != 0 {
		t.Fatalf("//kill frames = %x, want no Die of the point", testsupport.FrameOpcodes(frames))
	}
	if got := countFramesFor(frames, serverpackets.OpcodeDeleteObject, point.ObjectID()); got != 1 {
		t.Fatalf("//kill frames = %x, want one DeleteObject of the point", testsupport.FrameOpcodes(frames))
	}
	requireNoSystemMessage(t, "//kill", frames)
	if point.Dead() || decay.task.Tracked(point) {
		t.Fatalf("killed point: dead %v decay tracked %v, want neither", point.Dead(), decay.task.Tracked(point))
	}
}

// TestAdminManageUndyingDecorationKill pins //kill over an undying
// Christmas tree, as the shipped trees are: the hit floors its HP at one
// point and it never dies, so no Die goes out and no corpse is tracked, and
// a following //res finds a living tree and stands nothing up.
func TestAdminManageUndyingDecorationKill(t *testing.T) {
	t.Parallel()
	srv, _, decay, tree := bootTreeAdmin(t, gameservertest.WithNPCs(killTemplates(true)))

	frames := manageAround(t, srv, tree, "kill")
	if got := countFramesFor(frames, serverpackets.OpcodeDie, tree.ObjectID()); got != 0 {
		t.Fatalf("//kill frames = %x, want no Die of an undying tree", testsupport.FrameOpcodes(frames))
	}
	if tree.Dead() || tree.HP() != 1 || decay.task.Tracked(tree) {
		t.Fatalf("undying tree after //kill: dead %v HP %v decay tracked %v, want alive at 1 HP with no decay", tree.Dead(), tree.HP(), decay.task.Tracked(tree))
	}

	frames = manageAround(t, srv, tree, "res")
	requireNoRevive(t, "//res on an undying tree", frames)
	if tree.Dead() || tree.HP() != 1 {
		t.Fatalf("undying tree after //res: dead %v HP %v, want alive at 1 HP", tree.Dead(), tree.HP())
	}
}

// TestAdminManageDecorationKillWithoutDamage pins //kill over a mortal tree
// by a GM whose access level gives no damage: the hit takes nothing, so the
// tree keeps its full HP, never dies and leaves no corpse.
func TestAdminManageDecorationKillWithoutDamage(t *testing.T) {
	t.Parallel()
	srv, gmID, decay, tree := bootTreeAdmin(t)
	obj, ok := srv.State.Player(gmID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", gmID)
	}
	denied, ok := obj.(interface{ SetCanGiveDamage(bool) })
	if !ok {
		t.Fatalf("world.Player(%d) = %T has no SetCanGiveDamage", gmID, obj)
	}
	denied.SetCanGiveDamage(false)

	frames := manageAround(t, srv, tree, "kill")
	if got := countFramesFor(frames, serverpackets.OpcodeDie, tree.ObjectID()); got != 0 {
		t.Fatalf("//kill frames = %x, want no Die from a GM that gives no damage", testsupport.FrameOpcodes(frames))
	}
	if tree.Dead() || tree.HP() != tree.MaxHPValue() || decay.task.Tracked(tree) {
		t.Fatalf("tree after a damageless //kill: dead %v HP %v decay tracked %v, want alive at %v with no decay", tree.Dead(), tree.HP(), decay.task.Tracked(tree), tree.MaxHPValue())
	}
}
