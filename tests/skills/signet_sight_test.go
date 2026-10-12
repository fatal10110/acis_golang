package skills

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// bootSignetSight boots a level-5 caster that knows a plain SIGNET (454-1)
// whose effect point, template 13018, lasts lifetime seconds.
func bootSignetSight(t *testing.T, lifetime int) (*gameservertest.Server, int32) {
	t.Helper()
	def := modelskill.Definition{
		ID: 454, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "SIGNET", EffectNpcID: 13018, Radius: 180,
		Effects: []modelskill.EffectTemplate{{Name: "Signet", Count: 1, Time: lifetime}},
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{
			ID: 13018, TemplateID: 13018, Type: "EffectPoint", CollisionRadius: 5, RunSpeed: 20, WalkSpeed: 50,
		}})),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	objID := srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 454, 1)
	return srv, objID
}

// castSignetPoint has c cast the signet and returns the point it places.
func castSignetPoint(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient) *npc.EffectPoint {
	t.Helper()
	c.Send(encodeRequestMagicSkillUse(454, false, false))
	var point *npc.EffectPoint
	srv.AdvanceUntil(t, "signet effect point", func() bool {
		for _, obj := range srv.State.Objects() {
			if ep, ok := obj.(*npc.EffectPoint); ok {
				point = ep
			}
		}
		return point != nil
	})
	return point
}

// readObjectFrames reads c until quiet and returns every frame with opcode
// whose first field is objID.
func readObjectFrames(c *testsupport.ScriptedClient, opcode byte, objID int32) [][]byte {
	var out [][]byte
	for f := c.ReadWithTimeout(300 * time.Millisecond); f != nil; f = c.ReadWithTimeout(300 * time.Millisecond) {
		if len(f) >= 5 && f[0] == opcode && wire.NewReader(f[1:]).ReadInt32() == objID {
			out = append(out, f)
		}
	}
	return out
}

// assertPointNPCInfo fails unless frames hold exactly one NpcInfo of the
// point: its template shown as 13018 and attackable 0, since no creature may
// attack an effect point (EffectPoint.isAttackableBy).
func assertPointNPCInfo(t *testing.T, frames [][]byte, what string) {
	t.Helper()
	if len(frames) != 1 {
		t.Fatalf("%s: %d NpcInfo of the signet point, want 1", what, len(frames))
	}
	r := wire.NewReader(frames[0][1:])
	r.ReadInt32() // object id
	if got := r.ReadInt32(); got != 13018+1_000_000 {
		t.Fatalf("%s: NpcInfo template = %d, want %d", what, got, 13018+1_000_000)
	}
	if got := r.ReadInt32(); got != 0 {
		t.Fatalf("%s: NpcInfo attackable = %d, want 0", what, got)
	}
}

// TestSignetPointShownToCasterOnSpawn pins that a signet effect point is
// shown like any NPC (EffectPoint does not override Npc.sendInfo): the
// caster, who knows it as soon as it is placed, gets its NpcInfo, and its
// DeleteObject when the signet ends and the point leaves the world.
func TestSignetPointShownToCasterOnSpawn(t *testing.T) {
	t.Parallel()
	srv, _ := bootSignetSight(t, 1)
	c := srv.Client
	startInWorldAmongPlayers(t, c)
	drainUntilQuiet(t, c)

	point := castSignetPoint(t, srv, c)
	assertPointNPCInfo(t, readObjectFrames(c, serverpackets.OpcodeNPCInfo, point.ObjectID()), "placed point")

	srv.Advance(t, 1100*time.Millisecond)
	srv.TickEffects()
	if _, ok := srv.State.Object(point.ObjectID()); ok {
		t.Fatal("signet point still in the world after its signet ended")
	}
	if got := readObjectFrames(c, serverpackets.OpcodeDeleteObject, point.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject of the removed signet point = %d, want 1", len(got))
	}
}

// TestSignetPointShownToPlayerWalkingIntoSight pins the point's NpcInfo to a
// player who comes to know it later: a player entering the world out of the
// point's sight is not shown it, and is shown its NpcInfo once its walk
// brings the point's region into its neighborhood.
func TestSignetPointShownToPlayerWalkingIntoSight(t *testing.T) {
	t.Parallel()
	srv, _ := bootSignetSight(t, 600)
	if !srv.DrivesClock() {
		t.Skip("stepping a walk tick by tick needs the driven clock")
	}
	c := srv.Client
	startInWorldAmongPlayers(t, c)
	drainUntilQuiet(t, c)
	point := castSignetPoint(t, srv, c)
	drainUntilQuiet(t, c)

	// Regions are 2048 units wide and a player sees its region's 3x3
	// neighborhood. The point stands in the caster's region, X 0-2047; from
	// X 6200 the walker sees X 4096-10239 only, and sees the point once it
	// steps below X 4096.
	px, py, pz := point.Position()
	if px < 0 || px >= 2048 {
		t.Fatalf("signet point placed at X %d, want in the region at X 0-2047", px)
	}
	from := [3]int32{6200, int32(py), int32(pz)}
	walker := srv.SeedCharacterFor(t, "walker", "Walker", 5, 0)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?", from[0], from[1], from[2], walker.ID); err != nil {
		t.Fatalf("place walker: %v", err)
	}
	wc := srv.DialClient(t, "walker", 1)
	wc.Send(encodeRequestGameStart(0))
	wc.Read() // SSQInfo
	wc.Read() // CharSelected
	wc.Send(encodeEnterWorld())
	if got := readObjectFrames(wc, serverpackets.OpcodeNPCInfo, point.ObjectID()); len(got) != 0 {
		t.Fatalf("walker entering out of sight got %d NpcInfo of the signet point, want none", len(got))
	}

	wc.Send(encodeMoveFrom([3]int32{int32(px), from[1], from[2]}, from))
	mover := srv.PlayerMove(t, walker.ID)
	srv.AdvanceUntil(t, "walk starts", mover.Moving)
	for mover.Position().X >= 4096 {
		if !mover.Moving() {
			t.Fatalf("walk stopped at %v before crossing X=4096", mover.Position())
		}
		srv.TickPositions()
	}
	assertPointNPCInfo(t, readObjectFrames(wc, serverpackets.OpcodeNPCInfo, point.ObjectID()), "point walked into sight")
}

// TestSignetPointClickAnswersActionFailed pins EffectPoint.onAction: a click
// on the point, plain, shifted or forced, answers ActionFailed alone and
// never selects it.
func TestSignetPointClickAnswersActionFailed(t *testing.T) {
	t.Parallel()
	srv, objID := bootSignetSight(t, 600)
	c := srv.Client
	startInWorldAmongPlayers(t, c)
	drainUntilQuiet(t, c)
	point := castSignetPoint(t, srv, c)
	drainUntilQuiet(t, c)

	for _, click := range []struct {
		name  string
		frame []byte
	}{
		{"plain", encodeAction(point.ObjectID(), 0, 0, 0, false)},
		{"shift", encodeAction(point.ObjectID(), 0, 0, 0, true)},
		{"forced", encodeAttackRequestOn(point.ObjectID())},
		// A second plain click would act on a selected point; it is not
		// selected, so it is refused like the first.
		{"again", encodeAction(point.ObjectID(), 0, 0, 0, false)},
	} {
		c.Send(click.frame)
		failed := 0
		for f := c.ReadWithTimeout(300 * time.Millisecond); f != nil; f = c.ReadWithTimeout(300 * time.Millisecond) {
			switch f[0] {
			case serverpackets.OpcodeActionFailed:
				failed++
			case serverpackets.OpcodeMyTargetSelected, serverpackets.OpcodeTargetSelected:
				t.Fatalf("%s click on the signet point selected it", click.name)
			}
		}
		if failed != 1 {
			t.Fatalf("%s click on the signet point: %d ActionFailed, want 1", click.name, failed)
		}
		if target := selectedTarget(t, srv, objID); target != nil {
			t.Fatalf("%s click on the signet point left target %d, want none", click.name, target.ObjectID())
		}
	}
}

// selectedTarget is player objID's selected target.
func selectedTarget(t *testing.T, srv *gameservertest.Server, objID int32) world.Tracked {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d not in world", objID)
	}
	return obj.(interface{ Target() world.Tracked }).Target()
}

// encodeMoveFrom is MoveBackwardToLocation to target, sent from origin.
func encodeMoveFrom(target, origin [3]int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	for _, v := range target {
		w.WriteInt32(v)
	}
	for _, v := range origin {
		w.WriteInt32(v)
	}
	w.WriteInt32(1)
	return w.Bytes()
}

// encodeAttackRequestOn is an AttackRequest on objectID.
func encodeAttackRequestOn(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAttackRequest)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}
