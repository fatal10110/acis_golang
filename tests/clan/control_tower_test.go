package clan

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: CastleManager.spawnEntities spawns each castles.xml control
// tower once, at boot (TowerSpawnLocation.spawnMe): an NPC of a template
// built from the tower's stats, id 13003 (life) or 13005 (flame), undying,
// run speed 0, radius 7 and height 35, given its castle. LifeTower and
// FlameTower show it with ServerObjectInfo while unpolymorphed and with
// NpcInfo once polymorphed, and let only a playable whose clan attacks the
// castle's siege in progress attack a polymorphed one. Siege.startSiege
// polymorphs every tower into 13002 / 13004 (Creature.polymorph: decayMe,
// then spawnMe); endSiege fills each one's HP and unpolymorphs it;
// midVictory unpolymorphs a polymorphed flame tower and fills and
// polymorphs an unpolymorphed life tower. A hit leaving a polymorphed tower
// at 1 HP or less during the siege unpolymorphs it; a life tower then tells
// the defenders TOWER_DESTROYED_NO_RESURRECTION (717) once no life tower
// stays polymorphed (Castle.getAliveLifeTowerCount).

// towerBase is where the founder and the rival stand, the towers in a row
// beside them.
var towerBase = location.Location{X: -18000, Y: 108000, Z: -2400}

// towerHP is each tower's HP in the test castle.
const towerHP = 1000

// towerCastle is Gludio with two life towers and a flame tower beside
// towerBase.
func towerCastle(t *testing.T) *castledata.Table {
	t.Helper()
	tower := func(alias, kind string, dx int) castledata.ControlTower {
		ct, err := castledata.NewControlTower(alias, kind, towerBase.X+dx, towerBase.Y, towerBase.Z, towerHP, 100, 100, nil)
		if err != nil {
			t.Fatal(err)
		}
		return ct
	}
	c, err := castledata.NewCastle(castledata.CastleAttrs{ID: 1, Alias: "gludio_castle", Name: "Gludio"}, nil, []castledata.ControlTower{
		tower("life1", "LIFE_CONTROL", 100),
		tower("life2", "LIFE_CONTROL", 200),
		tower("flame1", "TRAP_CONTROL", 300),
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err := castledata.NewTable([]*castledata.Castle{c})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// workingTowerTemplates are the towers' working forms as the datapack
// defines them.
func workingTowerTemplates() *npc.Table {
	working := func(id int, kind, name string) *npc.Template {
		return &npc.Template{
			ID: id, TemplateID: id, Type: kind, Name: name, Level: 20, HPMax: 342, MPMax: 213,
			WalkSpeed: 49.5, CollisionRadius: 13, CollisionHeight: 54, CanBeAttacked: true, CanMove: true,
		}
	}
	return npc.NewTable([]*npc.Template{
		working(npc.LifeTowerWorkingNpcID, "LifeTower", "Life Control Tower"),
		working(npc.FlameTowerWorkingNpcID, "FlameTower", "Flame Control Tower"),
	})
}

// towerWorld boots the test castle owned by Rivals, Knights registered to
// attack its siege, with the NPC spawns placing its towers. The founder
// (Knights) and the rival (Rivals) stand at towerBase, in the world.
func towerWorld(t *testing.T) *clanWorld {
	t.Helper()
	at := func(who string) string {
		return `UPDATE characters SET x = ` + itoa(int32(towerBase.X)) + `, y = ` + itoa(int32(towerBase.Y)) + `, z = ` + itoa(int32(towerBase.Z)) + ` WHERE ` + who
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Founder", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCastles(towerCastle(t)),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithNPCs(workingTowerTemplates()),
		gameservertest.WithNpcSpawns(nil),
		seedRival(t),
		seedAllianceClans(t,
			`UPDATE clan_data SET hasCastle = 1 WHERE clan_id = `+itoa(rivalsClanID),
			`INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, `+itoa(knightsClanID)+`, 'ATTACKER')`,
			at(`char_name = 'Founder'`),
			at(`obj_Id = `+itoa(rivalLeaderID)),
		),
	)
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t), memberID: rivalLeaderID, at: towerBase}
	w.member = srv.DialClient(t, "player2", 1)
	return w
}

// towers returns the test castle's placed towers, life1, life2 and flame1.
func towers(t *testing.T, w *clanWorld) (gludio *siege.Siege, life1, life2, flame *npc.Folk) {
	t.Helper()
	gludio, ok := w.srv.Sieges.Get(1)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	placed := gludio.Castle().Towers()
	if len(placed) != 3 {
		t.Fatalf("towers placed = %d, want 3", len(placed))
	}
	var fs []*npc.Folk
	for _, p := range placed {
		f, ok := p.(*npc.Folk)
		if !ok || !f.Tower() || f.Instance.CastleID != 1 {
			t.Fatalf("placed tower %T, want a control tower of Gludio", p)
		}
		fs = append(fs, f)
	}
	return gludio, fs[0], fs[1], fs[2]
}

// towerView is how a frame shows a tower to one player.
type towerView struct {
	npcInfo    bool
	template   int32
	name       string
	attackable bool
	hp, maxHP  int32
}

// viewOf decodes frame, NpcInfo or ServerObjectInfo of id; ok is false for
// any other frame.
func viewOf(frame []byte, id int32) (towerView, bool) {
	if len(frame) < 5 || wire.NewReader(frame[1:]).ReadInt32() != id {
		return towerView{}, false
	}
	r := wire.NewReader(frame[1:])
	r.ReadInt32()
	switch frame[0] {
	case serverpackets.OpcodeNPCInfo:
		return towerView{npcInfo: true, template: r.ReadInt32() - 1_000_000, attackable: r.ReadInt32() == 1}, true
	case serverpackets.OpcodeServerObjectInfo:
		v := towerView{template: r.ReadInt32() - 1_000_000, name: r.ReadString(), attackable: r.ReadInt32() == 1}
		for range 4 {
			r.ReadInt32()
		}
		for range 4 {
			r.ReadFloat64()
		}
		v.hp, v.maxHP = r.ReadInt32(), r.ReadInt32()
		return v, true
	}
	return towerView{}, false
}

// shown returns how frames show the tower id, after it was taken out of
// sight first when deleted is set; it fails when they do not.
func shown(t *testing.T, frames [][]byte, id int32, deleted bool) towerView {
	t.Helper()
	deletedAt := -1
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeDeleteObject && wire.NewReader(f[1:]).ReadInt32() == id {
			deletedAt = i
			continue
		}
		if v, ok := viewOf(f, id); ok {
			if deleted && deletedAt < 0 {
				t.Fatalf("tower %d shown before it was taken out of sight: %x", id, opcodes(frames))
			}
			return v
		}
	}
	t.Fatalf("frames %x do not show tower %d", opcodes(frames), id)
	return towerView{}
}

func dead(life bool) towerView {
	if life {
		return towerView{template: npc.LifeTowerDeadNpcID, name: "Life Control Tower"}
	}
	return towerView{template: npc.FlameTowerDeadNpcID, name: "Flame Control Tower"}
}

func working(life, attackable bool) towerView {
	id := int32(npc.FlameTowerWorkingNpcID)
	if life {
		id = npc.LifeTowerWorkingNpcID
	}
	return towerView{npcInfo: true, template: id, attackable: attackable}
}

// TestControlTowersFollowTheSiege walks the towers through a siege: dead
// at boot, working once it starts (attackable by the attackers only),
// falling dead at 1 HP, the defenders told once no life tower stands,
// turned by a mid-victory, and dead at full HP once it ends.
func TestControlTowersFollowTheSiege(t *testing.T) {
	t.Parallel()
	w := towerWorld(t)
	gludio, life1, life2, flame := towers(t, w)
	// The castles.xml HP is the template's base, which the tower's CON
	// bonus scales as any creature's.
	full := life1.MaxHP()
	founderFrames := startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)

	// At boot every tower stands dead: ServerObjectInfo of its own
	// template, attackable by no one and showing no HP.
	for _, tc := range []struct {
		f    *npc.Folk
		life bool
	}{{life1, true}, {life2, true}, {flame, false}} {
		if got, want := shown(t, founderFrames, tc.f.ObjectID(), false), dead(tc.life); got != want {
			t.Fatalf("tower %d at boot shown %+v, want %+v", tc.f.ObjectID(), got, want)
		}
	}
	if got := gludio.AliveLifeTowers(); got != 2 {
		t.Fatalf("alive life towers before the siege = %d, want 2", got)
	}

	// The siege starts: every tower leaves sight and comes back working,
	// attackable by the attacker, not by the owner.
	gludio.Start()
	if !gludio.InProgress() {
		t.Fatal("Gludio siege not under way")
	}
	founder, rival := drainFrames(t, w.leader), drainFrames(t, w.member)
	for _, tc := range []struct {
		f    *npc.Folk
		life bool
	}{{life1, true}, {life2, true}, {flame, false}} {
		if got, want := shown(t, founder, tc.f.ObjectID(), true), working(tc.life, true); got != want {
			t.Fatalf("attacker sees tower %d at the start as %+v, want %+v", tc.f.ObjectID(), got, want)
		}
		if got, want := shown(t, rival, tc.f.ObjectID(), true), working(tc.life, false); got != want {
			t.Fatalf("owner sees tower %d at the start as %+v, want %+v", tc.f.ObjectID(), got, want)
		}
		if got := tc.f.CurrentHP(); got != full {
			t.Fatalf("tower %d HP at the start = %d, want %d", tc.f.ObjectID(), got, full)
		}
	}

	founderObj, _ := w.srv.State.Object(w.leaderID)
	attacker, ok := founderObj.(attackable.Combatant)
	if !ok {
		t.Fatalf("founder is %T, no combatant", founderObj)
	}

	// A hit that leaves the flame tower above 1 HP keeps it working.
	flame.TakeDamage(300, attacker)
	if !flame.TowerWorking() || flame.CurrentHP() != full-300 {
		t.Fatalf("flame tower after 300 damage working %v HP %d, want working at %d", flame.TowerWorking(), flame.CurrentHP(), full-300)
	}

	// The first life tower falls dead at 1 HP; one still stands, so no one
	// is told.
	life1.TakeDamage(full*2, attacker)
	founder, rival = drainFrames(t, w.leader), drainFrames(t, w.member)
	if got, want := shown(t, founder, life1.ObjectID(), true), dead(true); got != want {
		t.Fatalf("attacker sees the felled life tower as %+v, want %+v", got, want)
	}
	if life1.TowerWorking() || life1.CurrentHP() != 1 {
		t.Fatalf("felled life tower working %v HP %d, want dead at 1", life1.TowerWorking(), life1.CurrentHP())
	}
	if got := gludio.AliveLifeTowers(); got != 1 {
		t.Fatalf("alive life towers = %d, want 1", got)
	}
	if ids := messages(t, rival); slices.Contains(ids, siege.MsgTowerDestroyedNoResurrection) {
		t.Fatalf("owner told %v with a life tower standing", ids)
	}

	// The last one falls: the owner is told, the attacker is not.
	life2.TakeDamage(full*2, attacker)
	founder, rival = drainFrames(t, w.leader), drainFrames(t, w.member)
	if ids := messages(t, rival); !slices.Contains(ids, siege.MsgTowerDestroyedNoResurrection) {
		t.Fatalf("owner told %v once no life tower stands, want TOWER_DESTROYED_NO_RESURRECTION", ids)
	}
	if ids := messages(t, founder); slices.Contains(ids, siege.MsgTowerDestroyedNoResurrection) {
		t.Fatalf("attacker told %v, want no TOWER_DESTROYED_NO_RESURRECTION", ids)
	}
	if got := gludio.AliveLifeTowers(); got != 0 {
		t.Fatalf("alive life towers = %d, want 0", got)
	}
	// A dead tower takes more hits without telling anyone again.
	life2.TakeDamage(10, attacker)
	if ids := messages(t, drainFrames(t, w.member)); slices.Contains(ids, siege.MsgTowerDestroyedNoResurrection) {
		t.Fatalf("owner told %v again by a hit on a dead tower", ids)
	}

	// Knights take the castle: the working flame tower falls dead with the
	// HP it had, the dead life towers are filled and work for the new
	// owner, attackable by the former one.
	knights, ok := w.srv.Clans.Table().Get(knightsClanID)
	if !ok {
		t.Fatal("Knights not loaded")
	}
	if _, ok := w.srv.Castles.SetOwner(gludio.Castle(), knights); !ok {
		t.Fatal("Knights could not take Gludio")
	}
	gludio.MidVictory()
	founder, rival = drainFrames(t, w.leader), drainFrames(t, w.member)
	if got, want := shown(t, founder, flame.ObjectID(), true), dead(false); got != want {
		t.Fatalf("flame tower after the mid-victory shown %+v, want %+v", got, want)
	}
	if flame.CurrentHP() != full-300 {
		t.Fatalf("flame tower HP after the mid-victory = %d, want %d", flame.CurrentHP(), full-300)
	}
	for _, f := range []*npc.Folk{life1, life2} {
		if got, want := shown(t, founder, f.ObjectID(), true), working(true, false); got != want {
			t.Fatalf("new owner sees life tower %d as %+v, want %+v", f.ObjectID(), got, want)
		}
		if got, want := shown(t, rival, f.ObjectID(), true), working(true, true); got != want {
			t.Fatalf("former owner sees life tower %d as %+v, want %+v", f.ObjectID(), got, want)
		}
		if got := f.CurrentHP(); got != full {
			t.Fatalf("life tower %d HP after the mid-victory = %d, want %d", f.ObjectID(), got, full)
		}
	}
	if got := gludio.AliveLifeTowers(); got != 2 {
		t.Fatalf("alive life towers after the mid-victory = %d, want 2", got)
	}

	// The siege ends: every tower stands dead at full HP.
	gludio.End()
	founder = drainFrames(t, w.leader)
	drainFrames(t, w.member)
	for _, tc := range []struct {
		f    *npc.Folk
		life bool
	}{{life1, true}, {life2, true}, {flame, false}} {
		if got, want := shown(t, founder, tc.f.ObjectID(), true), dead(tc.life); got != want {
			t.Fatalf("tower %d after the siege shown %+v, want %+v", tc.f.ObjectID(), got, want)
		}
		if tc.f.TowerWorking() || tc.f.CurrentHP() != full {
			t.Fatalf("tower %d after the siege working %v HP %d, want dead at %d", tc.f.ObjectID(), tc.f.TowerWorking(), tc.f.CurrentHP(), full)
		}
	}
	if got := gludio.AliveLifeTowers(); got != 2 {
		t.Fatalf("alive life towers after the siege = %d, want 2", got)
	}
}

// TestControlTowerAttackedOnlyByAttackers has the attacker and the owner
// each click a working tower twice: the attacker's second click attacks
// it, the owner's does not; before the siege neither does.
func TestControlTowerAttackedOnlyByAttackers(t *testing.T) {
	t.Parallel()
	w := towerWorld(t)
	gludio, life1, _, _ := towers(t, w)
	startInWorld(t, w.leader)
	startInWorld(t, w.member)

	if !w.srv.DrivesClock() {
		t.Skip("the walk to the tower needs the driven clock")
	}
	// The second click starts the player's attack, or its walk to talk;
	// the walk ends and the first swing lands within a few seconds.
	clickTwice := func(client *testsupport.ScriptedClient) [][]byte {
		client.Send(encodeAction(life1.ObjectID(), w.at))
		drainFrames(t, client)
		client.Send(encodeAction(life1.ObjectID(), w.at))
		frames := drainFrames(t, client)
		w.srv.Advance(t, 5*time.Second)
		return append(frames, drainFrames(t, client)...)
	}
	attacks := func(frames [][]byte) bool {
		return slices.ContainsFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeAttack })
	}
	if attacks(clickTwice(w.leader)) {
		t.Fatal("the attacker attacked a dead tower before the siege")
	}

	gludio.Start()
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	if attacks(clickTwice(w.member)) {
		t.Fatal("the owner attacked its own working tower")
	}
	if !attacks(clickTwice(w.leader)) {
		t.Fatal("the attacker's second click did not attack the working tower")
	}
}
