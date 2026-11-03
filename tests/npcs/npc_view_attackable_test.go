package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// npcInfoAttackable reads the attackable flag of the last NpcInfo of objID
// among frames.
func npcInfoAttackable(t *testing.T, frames [][]byte, objID int32) int32 {
	t.Helper()
	infos := npcInfoFrames(frames, serverpackets.OpcodeNPCInfo, objID)
	if len(infos) == 0 {
		t.Fatalf("no NpcInfo of NPC %d", objID)
	}
	r := wire.NewReader(infos[len(infos)-1][1:])
	r.ReadInt32() // object id
	r.ReadInt32() // template id
	return r.ReadInt32()
}

// serverObjectInfoView is the attackable flag and the HP pair of a
// ServerObjectInfo frame.
type serverObjectInfoView struct{ attackable, hp, maxHP int32 }

// lastServerObjectInfo decodes the last ServerObjectInfo of objID among
// frames.
func lastServerObjectInfo(t *testing.T, frames [][]byte, objID int32) serverObjectInfoView {
	t.Helper()
	infos := npcInfoFrames(frames, serverpackets.OpcodeServerObjectInfo, objID)
	if len(infos) == 0 {
		t.Fatalf("no ServerObjectInfo of NPC %d", objID)
	}
	r := wire.NewReader(infos[len(infos)-1][1:])
	r.ReadInt32() // object id
	r.ReadInt32() // template id
	r.ReadString()
	attackable := r.ReadInt32()
	for range 4 { // x, y, z, heading
		r.ReadInt32()
	}
	for range 4 { // the two multipliers, collision radius and height
		r.ReadFloat64()
	}
	return serverObjectInfoView{attackable: attackable, hp: r.ReadInt32(), maxHP: r.ReadInt32()}
}

// TestNpcInfoAttackableFollowsWhetherTheViewerMayAttackWithoutForce pins
// the attackable flag of NpcInfo (AbstractNpcInfo.NpcInfo): whether the
// viewer may attack the NPC without Ctrl. A town guard needs Ctrl, so the
// player sees it as not attackable, while a living monster shows the attack
// cursor; once the monster is dead, its next NpcInfo shows it as not
// attackable.
func TestNpcInfoAttackableFollowsWhetherTheViewerMayAttackWithoutForce(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	guard := w.srv.SpawnHostileNPCKindAt(t, "Guard", location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z})
	monster := w.srv.SpawnHostileNPCKindAt(t, "Monster", location.Location{X: w.at.X - 100, Y: w.at.Y, Z: w.at.Z})
	frames := drainFrames(t, w.c)
	if got := npcInfoAttackable(t, frames, guard.ObjectID()); got != 0 {
		t.Fatalf("town guard NpcInfo attackable = %d, want 0", got)
	}
	if got := npcInfoAttackable(t, frames, monster.ObjectID()); got != 1 {
		t.Fatalf("monster NpcInfo attackable = %d, want 1", got)
	}

	// An abnormal-effect change shows each viewer its NpcInfo again.
	guard.UpdateAbnormalEffect()
	if got := npcInfoAttackable(t, drainFrames(t, w.c), guard.ObjectID()); got != 0 {
		t.Fatalf("town guard re-broadcast NpcInfo attackable = %d, want 0", got)
	}

	if !monster.Die(nil, nil) {
		t.Fatal("monster did not die")
	}
	drainFrames(t, w.c)
	monster.UpdateAbnormalEffect()
	if got := npcInfoAttackable(t, drainFrames(t, w.c), monster.ObjectID()); got != 0 {
		t.Fatalf("dead monster NpcInfo attackable = %d, want 0", got)
	}
}

// TestServerObjectInfoAttackableShowsHPOnlyWhenAttackable pins the
// stationary view (ServerObjectInfo): its attackable flag is whether the
// viewer may attack the NPC at all, Ctrl included, and the HP pair is sent
// only with it. A civilian NPC and a town guard that cannot move, entering
// water, show the player 1 with their HP.
func TestServerObjectInfoAttackableShowsHPOnlyWhenAttackable(t *testing.T) {
	t.Parallel()
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, zoneBox(t, 1_200, 2_000)))
	w := bootFolkWorld(t, nil, gameservertest.WithZones(zones))

	tmpl := folkTemplate("Merchant", 30002)
	tmpl.RunSpeed, tmpl.WalkSpeed = 0, 0
	statue := w.srv.SpawnFolkNPCAt(t, tmpl, location.Location{X: 1_500, Y: w.at.Y, Z: w.at.Z})
	frames := drainFrames(t, w.c)
	got := lastServerObjectInfo(t, frames, statue.ObjectID())
	if want := (serverObjectInfoView{attackable: 1, hp: int32(statue.CurrentHP()), maxHP: int32(statue.MaxHP())}); got != want || want.maxHP == 0 {
		t.Fatalf("still civilian NPC ServerObjectInfo = %+v, want %+v with its HP", got, want)
	}

	guard := w.srv.SpawnHostileNPCTemplateAt(t, stillHostileTemplate("Guard"), location.Location{X: 1_600, Y: w.at.Y, Z: w.at.Z})
	got = lastServerObjectInfo(t, drainFrames(t, w.c), guard.ObjectID())
	if want := (serverObjectInfoView{attackable: 1, hp: int32(guard.CurrentHP()), maxHP: int32(guard.MaxHPValue())}); got != want || want.maxHP == 0 {
		t.Fatalf("still town guard ServerObjectInfo = %+v, want %+v with its HP", got, want)
	}
}

// stillHostileTemplate is a hostile NPC template of kind that cannot move.
func stillHostileTemplate(kind string) *npc.Template {
	return &npc.Template{
		ID: 101, TemplateID: 101, Type: kind, Level: 1, HPMax: 1000, AtkSpd: 300,
		CollisionRadius: 8, CollisionHeight: 20,
	}
}
