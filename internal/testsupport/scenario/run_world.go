package scenario

import (
	"encoding/binary"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// memberAccount is the account a member line's character is seeded on.
func memberAccount(m Member) string { return "member_" + strings.ToLower(m.Name) }

// enterMembers logs each member line's character in on its own client,
// enters it into the world, has the character invite it into its party,
// which it accepts, puts it at its offset and marks a dead one dead.
func (r *run) enterMembers(t *testing.T) {
	t.Helper()
	for i, m := range r.sc.Members {
		c := r.srv.DialClient(t, memberAccount(m), 1)
		c.Send(gameStart())
		for n := 0; ; n++ {
			if n == 100 {
				t.Fatalf("%s: member %s: no CharSelected within 100 frames", r.sc.Name, m.Name)
			}
			if c.Read()[0] == serverpackets.OpcodeCharSelected {
				break
			}
		}
		c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
		r.srv.ReadQueued(t, c)

		invite := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
		invite.WriteString(m.Name)
		invite.WriteInt32(0)
		r.srv.Client.Send(invite.Bytes())
		r.srv.ReadQueued(t, c)
		answer := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
		answer.WriteInt32(1)
		c.Send(answer.Bytes())
		joined := slices.ContainsFunc(r.srv.ReadQueued(t, c), func(f []byte) bool { return f[0] == serverpackets.OpcodePartySmallWindowAll })
		if !joined {
			t.Fatalf("%s: member %s did not join the character's party", r.sc.Name, m.Name)
		}
		r.srv.ReadQueued(t, r.srv.Client)
		if m.DX != 0 || m.DY != 0 || m.DZ != 0 {
			obj, ok := r.srv.State.Player(r.members[i].obj)
			if !ok {
				t.Fatalf("%s: member %s is not in the world", r.sc.Name, m.Name)
			}
			if err := r.srv.State.Move(obj, r.origin.X+m.DX, r.origin.Y+m.DY, r.origin.Z+m.DZ); err != nil {
				t.Fatalf("%s: member %s: %v", r.sc.Name, m.Name, err)
			}
		}
		if m.Dead {
			r.srv.MarkPlayerDead(t, r.members[i].obj)
		}
		r.members[i].client = c
	}
}

// drainMembers drops what the members' clients were sent: a scenario
// compares the character's packets only.
func (r *run) drainMembers(t *testing.T) {
	t.Helper()
	for _, m := range r.members {
		if m.client != nil {
			r.srv.ReadQueued(t, m.client)
		}
	}
}

// gameStart is a RequestGameStart of the account's first character.
func gameStart() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

// maker builds the npcmaker of territory line tr, its box placed at its
// offsets from where the character first entered the world.
func (r *run) maker(t *testing.T, tr Territory) *spawn.Maker {
	t.Helper()
	o := r.origin
	x1, y1, x2, y2 := o.X+tr.DX1, o.Y+tr.DY1, o.X+tr.DX2, o.Y+tr.DY2
	territory, err := spawn.NewTerritory(tr.Name, o.Z+tr.DZ1, o.Z+tr.DZ2,
		[]spawn.Node{{X: x1, Y: y1}, {X: x2, Y: y1}, {X: x2, Y: y2}, {X: x1, Y: y2}})
	if err != nil {
		t.Fatalf("%s: territory %s: %v", r.sc.Name, tr.Name, err)
	}
	return spawn.NewMaker(spawn.Maker{Name: "scenario_" + tr.Name, Territories: []*spawn.Territory{territory}, AIType: "default_maker", MaximumNPCs: 1}, 1)
}

// lay lays ground line g's item at its offset from the character, owned by
// no one, and names it by its role.
func (r *run) lay(t *testing.T, g Ground) {
	t.Helper()
	if _, ok := r.items.Get(g.Item); !ok {
		t.Fatalf("%s: ground %s: no item %d in the catalog; add an item line", r.sc.Name, g.Role, g.Item)
	}
	before := map[int32]bool{}
	for _, s := range r.srv.GroundItems.Snapshots(nil) {
		before[s.ObjectID] = true
	}
	if g.Cursed {
		r.startCursedLife(t, g)
	}
	at := location.Location{X: r.origin.X + g.DX, Y: r.origin.Y + g.DY, Z: r.origin.Z + g.DZ}
	r.srv.SeedGroundItem(t, 0, g.Item, g.Count, at.X, at.Y, at.Z)
	for _, s := range r.srv.GroundItems.Snapshots(nil) {
		if !before[s.ObjectID] && s.TemplateID == g.Item {
			r.roles[s.ObjectID], r.objs[g.Role], r.grounds[s.ObjectID] = g.Role, s.ObjectID, true
			if g.Cursed {
				r.srv.CursedWeapons.PlaceOnGround(g.Item, s.ObjectID, at)
			}
			return
		}
	}
	t.Fatalf("%s: ground %s: item %d not on the ground", r.sc.Name, g.Role, g.Item)
}

// startCursedLife starts the life of ground line g's cursed weapon as a
// monster kill's drop roll does: of the weapons not out, in the order the
// roll visits them, only g's rolls under its drop rate.
func (r *run) startCursedLife(t *testing.T, g Ground) {
	t.Helper()
	turn, found := 0, false
	for _, w := range r.srv.CursedWeapons.Weapons() {
		if w.Out {
			continue
		}
		if w.ItemID == g.Item {
			found = true
			break
		}
		turn++
	}
	if !found {
		t.Fatalf("%s: ground %s: item %d is no cursed weapon of the shipped data, or it is out already", r.sc.Name, g.Role, g.Item)
	}
	call := 0
	id, ok := r.srv.CursedWeapons.RollDrop(func(n int) int {
		defer func() { call++ }()
		if call == turn {
			return 0
		}
		return n - 1
	})
	if !ok || id != g.Item {
		t.Fatalf("%s: ground %s: the drop roll brought out %d (%v), want %d", r.sc.Name, g.Role, id, ok, g.Item)
	}
}

// groundDeleted renders a DeleteObject of a ground line's item; false for
// any other frame.
func (r *run) groundDeleted(f []byte) (string, bool) {
	if len(f) < 5 || f[0] != serverpackets.OpcodeDeleteObject {
		return "", false
	}
	obj := int32(binary.LittleEndian.Uint32(f[1:5]))
	if !r.grounds[obj] {
		return "", false
	}
	return "DeleteObject obj=" + r.roles[obj], true
}

// look runs a look step: the NPC looks for ground items on its own queue.
func (r *run) look(t *testing.T, args []string) {
	t.Helper()
	l, _ := lookArgs(args)
	n, post := r.npcQueue(t, args[0])
	done := make(chan struct{})
	if !post(func() {
		defer close(done)
		script.NPCOf(n).LookItem(l.radius, l.quantity, l.items...)
	}) {
		t.Fatalf("%s: NPC %s's queue is closed", r.sc.Name, args[0])
	}
	<-done
	r.srv.Settle(t)
}

// walk runs position ticks until role's NPC stops moving; the NPC must be
// walking as the step starts.
func (r *run) walk(t *testing.T, role string) {
	t.Helper()
	n, _ := r.npcQueue(t, role)
	moving, ok := n.(interface{ IsMoving() bool })
	if !ok {
		t.Fatalf("%s: NPC %s (%T) does not walk", r.sc.Name, role, n)
	}
	if !moving.IsMoving() {
		t.Fatalf("%s: NPC %s is not walking", r.sc.Name, role)
	}
	for i := 0; moving.IsMoving(); i++ {
		if i >= int(walkLimit/move.PositionUpdateInterval) {
			t.Fatalf("%s: NPC %s still walking after %v", r.sc.Name, role, walkLimit)
		}
		r.srv.TickPositions()
	}
}

// walkLimit bounds how long a walk step lets an NPC walk.
const walkLimit = 20 * time.Second

// npcQueue returns role's live NPC and the post of its queue.
func (r *run) npcQueue(t *testing.T, role string) (attackable.Combatant, func(func()) bool) {
	t.Helper()
	obj, ok := r.srv.State.Object(r.objs[role])
	if !ok {
		t.Fatalf("%s: NPC %s is not in the world", r.sc.Name, role)
	}
	switch n := obj.(type) {
	case *npc.Hostile:
		return n, n.Queue().Post
	case *npc.Folk:
		return n, n.Queue().Post
	}
	t.Fatalf("%s: %s (%T) is not an NPC", r.sc.Name, role, obj)
	return nil, nil
}

// pinPlayerRoll makes every combat draw of the character draw, n-1 for a
// draw of [0, n) it is not inside, set on the character's queue.
func (r *run) pinPlayerRoll(t *testing.T, draw int) {
	t.Helper()
	r.onPlayer(t, func(p attackable.Combatant) {
		pinned, ok := p.(interface{ SetRollSource(func(int) int) })
		if !ok {
			t.Errorf("%s: the character (%T) has no combat random source", r.sc.Name, p)
			return
		}
		pinned.SetRollSource(func(n int) int { return min(draw, n-1) })
	})
}

// pinRoll makes every combat draw of role's NPC draw, n-1 for a draw of
// [0, n) it is not inside, set on the NPC's own queue.
func (r *run) pinRoll(t *testing.T, role string, draw int) {
	t.Helper()
	n, post := r.npcQueue(t, role)
	pinned, ok := n.(interface{ SetRollSource(func(int) int) })
	if !ok {
		t.Fatalf("%s: NPC %s (%T) has no combat random source", r.sc.Name, role, n)
	}
	done := make(chan struct{})
	if !post(func() {
		defer close(done)
		pinned.SetRollSource(func(n int) int { return min(draw, n-1) })
	}) {
		t.Fatalf("%s: NPC %s's queue is closed", r.sc.Name, role)
	}
	<-done
}
