package scenario

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// fixtureSpawn is where a seeded or created character first enters the
// world: the fixture class template's spawn point. Door lines stand at
// their offsets from it, as the doors are built at boot.
func fixtureSpawn() location.Location { return gameservertest.ClassTemplate().Spawns[0] }

// fightOptions are the boot options of the skill, door and summon lines
// and the npc lines' skills: the scripts read every listed shipped skill,
// the character knows its skill lines' skills, the doors stand at their
// offsets, and a summon line's siege summon has its template and a siege
// battlefield in progress over the scenario's ground to stay on.
func (r *run) fightOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	var refs []modelskill.Ref
	for _, n := range r.sc.NPCs {
		for _, ref := range n.Skills {
			refs = append(refs, ref)
		}
	}
	refs = append(refs, r.sc.Skills...)
	refs = append(refs, r.sc.stepSkills()...)
	var opts []gameservertest.Option
	if len(refs) > 0 {
		defs := make([]modelskill.Definition, 0, len(refs))
		for _, ref := range refs {
			defs = append(defs, shippedSkill(t, ref))
		}
		opts = append(opts, gameservertest.WithScriptSkills(defs...))
	}
	if len(r.sc.Skills) > 0 {
		// The fixture character's own skills, and the skill lines'.
		defs := []modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}}
		for _, ref := range r.sc.Skills {
			defs = append(defs, shippedSkill(t, ref))
		}
		db := sqltest.SharedDB(t)
		opts = append(opts, gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(defs), gamesql.NewCharacterSkillStore(db))))
	}
	if len(r.sc.Doors) > 0 {
		at := fixtureSpawn()
		tmpls := make([]*door.Template, 0, len(r.sc.Doors))
		for _, d := range r.sc.Doors {
			x, y := at.X+d.DX, at.Y+d.DY
			tmpls = append(tmpls, &door.Template{
				ID: d.ID, Name: "scenario_" + d.Role, Kind: door.KindDoor, Level: 1,
				Position: location.Location{X: x, Y: y, Z: at.Z + d.DZ},
				Coordinates: []location.Point{
					{X: x - 8, Y: y - 8}, {X: x + 8, Y: y - 8}, {X: x + 8, Y: y + 8}, {X: x - 8, Y: y + 8},
				},
				HP: doorHP, PDef: 100, MDef: 100, Height: 32,
			})
		}
		opts = append(opts, gameservertest.WithDoors(tmpls...))
	}
	if zones := r.zoneIndex(t); zones != nil {
		opts = append(opts, gameservertest.WithZones(zones))
	}
	return opts
}

// doorHP is a door line's door's hit points.
const doorHP = 600

// The siege battlefield a summon line's summon stands on: its zone id, and
// how far it reaches from the fixture spawn each way.
const (
	battlefieldID    = 90001
	battlefieldReach = 3000
)

// summonTemplate is the summon line's fixture template: a siege summon, or a
// servitor of the line's level.
func summonTemplate(s *Summon) *npc.Template {
	if s.Servitor {
		return &npc.Template{
			ID: int(s.ID), TemplateID: int(s.ID), Type: "Servitor", Name: "Servitor", Level: s.Level,
			HPMax: 5000, MPMax: 100, AtkSpd: 300, RunSpeed: 60, WalkSpeed: 30,
			CollisionRadius: 8, CollisionHeight: 20,
		}
	}
	return &npc.Template{
		ID: int(s.ID), TemplateID: int(s.ID), Type: "SiegeSummon", Name: "Siege Summon", Level: 60,
		HPMax: 5000, MPMax: 100, AtkSpd: 300, RunSpeed: 60, WalkSpeed: 30,
		CollisionRadius: 60, CollisionHeight: 60,
	}
}

// shippedSkill is the shipped definition of ref.
func shippedSkill(t *testing.T, ref modelskill.Ref) modelskill.Definition {
	t.Helper()
	def, ok := shippedSkills(t).Definition(ref)
	if !ok {
		t.Fatalf("no shipped skill %d level %d", ref.ID, ref.Level)
	}
	return def
}

// seedSkills stores the skill lines' skills as the character's, before it
// is selected.
func (r *run) seedSkills(t *testing.T) {
	t.Helper()
	for _, ref := range r.sc.Skills {
		if err := r.srv.KnownSkills.SetKnownSkill(context.Background(), r.player, 0, int(ref.ID), ref.Level); err != nil {
			t.Fatalf("%s: seed skill %d: %v", r.sc.Name, ref.ID, err)
		}
	}
}

// placeFight names the door lines' doors by their roles and calls the
// summon line's summon, as the character first enters the world.
func (r *run) placeFight(t *testing.T) {
	t.Helper()
	if (len(r.sc.Doors) > 0 || len(r.sc.Zones) > 0 || (r.sc.Summon != nil && !r.sc.Summon.Servitor)) && r.origin != fixtureSpawn() {
		t.Fatalf("%s: the character entered the world at %v, not the fixture spawn %v the doors, the zones and the battlefield stand by", r.sc.Name, r.origin, fixtureSpawn())
	}
	for _, d := range r.sc.Doors {
		obj, ok := r.srv.WorldObjects.Door(d.ID)
		if !ok {
			t.Fatalf("%s: door %s is not in the world", r.sc.Name, d.Role)
		}
		r.roles[obj.ObjectID()], r.objs[d.Role] = d.Role, obj.ObjectID()
	}
	if s := r.sc.Summon; s != nil {
		r.callSummon(t, s)
	}
}

// callSummon has the character call the summon line's siege summon, on its
// own queue, and waits until it stands on the battlefield.
func (r *run) callSummon(t *testing.T, s *Summon) {
	t.Helper()
	obj, ok := r.srv.State.Player(r.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("the character %T is not an online character", obj)
	}
	if !r.srv.PlayerQueue(t, r.player).Post(func() {
		c.SummonServitor(modelskill.Definition{ID: 13, Level: 1, SkillType: "SUMMON", NpcID: int(s.ID), SummonTotalLifeTime: 1_200_000})
	}) {
		t.Fatal("the character's queue is closed")
	}
	var live *summon.Actor
	r.srv.AdvanceUntil(t, "the summon line's summon is in the world", func() bool {
		if obj, ok := r.srv.State.Summon(r.player); ok {
			live, ok = obj.(*summon.Actor)
			return ok && live.NPCID() == int(s.ID) && !live.OffBattlefield()
		}
		return false
	})
	r.summon = live
	r.roles[live.ObjectID()], r.objs[s.Role] = s.Role, live.ObjectID()
}

// creature returns role's creature: the character, a member line's
// character, an NPC line's NPC or the summon line's summon, with the post of
// its queue.
func (r *run) creature(t *testing.T, role string) (attackable.Combatant, func(func()) bool) {
	t.Helper()
	switch {
	case role == "player":
		return r.characterOf(t, r.player)
	case r.sc.summoned(role):
		return r.summon, r.summon.Queue().Post
	case r.sc.isMember(role):
		i := slices.IndexFunc(r.sc.Members, func(m Member) bool { return m.Name == role })
		return r.characterOf(t, r.members[i].obj)
	}
	return r.npcQueue(t, role)
}

// characterOf returns the character of an object id as a combatant, with the
// post of its queue.
func (r *run) characterOf(t *testing.T, id int32) (attackable.Combatant, func(func()) bool) {
	t.Helper()
	obj, ok := r.srv.State.Player(id)
	if !ok {
		t.Fatalf("%s: the character %d is not in the world", r.sc.Name, id)
	}
	p, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("%s: the character %T is not a combatant", r.sc.Name, obj)
	}
	return p, r.srv.PlayerQueue(t, id).Post
}

// on runs fn on post's queue and waits for it.
func (r *run) on(t *testing.T, role string, post func(func()) bool, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !post(func() {
		defer close(done)
		fn()
	}) {
		t.Fatalf("%s: %s's queue is closed", r.sc.Name, role)
	}
	<-done
}

// hit runs a hit step: the character, or the creature by names, lands a
// hit of damage on an NPC line's NPC or a door line's door, on the
// attacker's own queue.
func (r *run) hit(t *testing.T, args []string) {
	t.Helper()
	damage, _ := strconv.ParseInt(args[1], 10, 32)
	hit, _ := parseHit(args)
	by := hit.by
	var def modelskill.Definition
	if hit.hasSkill {
		def = shippedSkill(t, hit.skill)
	}
	attacker, post := r.creature(t, by)
	if d, ok := r.sc.door(args[0]); ok {
		obj, ok := r.srv.WorldObjects.Door(d.ID)
		if !ok {
			t.Fatalf("%s: door %s is not in the world", r.sc.Name, d.Role)
		}
		r.on(t, by, post, func() { obj.ReduceHP(float64(damage), attacker, def) })
		r.srv.Settle(t)
		return
	}
	h := r.live[args[0]]
	if h == nil || h.AlikeDead() {
		t.Fatalf("%s: NPC %s is not a live attackable", r.sc.Name, args[0])
	}
	r.on(t, by, post, func() {
		if hit.hasSkill {
			h.ReduceHP(float64(damage), attacker, def)
			return
		}
		h.TakeDamage(int(damage), attacker)
	})
	r.srv.Settle(t)
}

// cast runs a cast step: the character selects role's NPC, unless it has
// it selected, then casts its skill id on it, forced, and time passes
// (position ticks while it walks into cast range) until it neither walks
// nor casts; it returns what the cast sent, the selection's packets left
// out.
func (r *run) cast(t *testing.T, role string, id string) [][]byte {
	t.Helper()
	skill, _ := strconv.ParseInt(id, 10, 32)
	obj, ok := r.srv.State.Player(r.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("the character %T is not an online character", obj)
	}
	target := r.objs[role]
	var selected bool
	r.on(t, "the character", r.srv.PlayerQueue(t, r.player).Post, func() {
		cur := c.Target()
		selected = cur != nil && cur.ObjectID() == target
	})
	if !selected {
		n, _ := r.npcQueue(t, role)
		x, y, z := n.Position()
		w := wire.NewPacketWriter(clientpackets.OpcodeAction)
		w.WriteInt32(target)
		w.WriteInt32(int32(x))
		w.WriteInt32(int32(y))
		w.WriteInt32(int32(z))
		w.WriteUint8(0)
		r.srv.Client.Send(w.Bytes())
		r.srv.Settle(t)
		r.srv.ReadQueued(t, r.srv.Client)
	}
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(int32(skill))
	w.WriteInt32(1)
	w.WriteUint8(0)
	r.srv.Client.Send(w.Bytes())
	r.srv.Settle(t)
	var frames [][]byte
	for passed := time.Duration(0); ; passed += castStep {
		frames = append(frames, r.srv.ReadQueued(t, r.srv.Client)...)
		moving := r.srv.PlayerMove(t, r.player).Moving()
		if !moving && !r.srv.PlayerCastingNow(t, r.player) {
			break
		}
		if passed >= castLimit {
			t.Fatalf("%s: cast %s on %s: still walking or casting after %v", r.sc.Name, id, role, castLimit)
		}
		if moving {
			// A walk into cast range moves on position ticks.
			r.srv.TickPositions()
			r.srv.Settle(t)
			continue
		}
		r.srv.Advance(t, castStep)
	}
	r.srv.Settle(t)
	return frames
}

// castStep is how much time a cast step lets pass at a time, and castLimit
// how long it waits for the cast to end: a fixture character casts slowly.
const (
	castStep  = 100 * time.Millisecond
	castLimit = time.Minute
)

// jump runs a jump step: role's NPC jumps to its offset from where the
// character first entered the world, its spawn point kept.
func (r *run) jump(t *testing.T, args []string) {
	t.Helper()
	n, _ := ints(args[1:])
	h, post := r.npcQueue(t, args[0])
	to := script.Loc{X: r.origin.X + n[0], Y: r.origin.Y + n[1], Z: r.origin.Z + n[2]}
	r.on(t, args[0], post, func() { script.NPCOf(h).InstantTeleportTo(to) })
	r.srv.Settle(t)
}

// awayReach is how far behind (-x) where the character first entered the
// world an away step puts it: past the see range of every NPC standing at
// an x offset of -1000 or more, still within the regions around them, so
// their see-creature scans forget it.
const awayReach = 1500

// away runs an away step: the character steps out of every NPC's sight,
// then one AI tick runs, whose see-creature scans forget it; the run fails
// if an NPC line's NPC still has it in its see range.
func (r *run) away(t *testing.T) {
	t.Helper()
	obj, ok := r.srv.State.Player(r.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	x, y, z := r.origin.X-awayReach, r.origin.Y, r.origin.Z
	if err := r.srv.State.Move(obj, x, y, z); err != nil {
		t.Fatalf("away: %v", err)
	}
	for role, h := range r.live {
		hx, hy, hz := h.Position()
		if dx, dy, dz := float64(hx-x), float64(hy-y), float64(hz-z); dx*dx+dy*dy+dz*dz <= npc.DefaultSeeRange*npc.DefaultSeeRange {
			t.Fatalf("%s: away: NPC %s still sees the character", r.sc.Name, role)
		}
	}
	if err := r.srv.AI.Tick(); err != nil {
		t.Fatalf("AI tick: %v", err)
	}
	r.srv.Settle(t)
}

// fightCheck runs one hate, desires or at line, describing the first
// difference.
func (r *run) fightCheck(t *testing.T, ck Check) string {
	t.Helper()
	switch ck.Kind {
	case "hate":
		h := r.liveNPC(t, ck.Args[0])
		of := "player"
		if len(ck.Args) == 3 {
			of = ck.Args[2]
		}
		c, _ := r.creature(t, of)
		want, _ := strconv.ParseFloat(ck.Args[1], 64)
		return FirstDiff(ck.Args[0]+" hate of "+of, []string{weight(h.AggroHate(c))}, []string{weight(want)})
	case "desires":
		h := r.liveNPC(t, ck.Args[0])
		want := slices.Clone(ck.Args[1:])
		if len(want) == 1 && want[0] == "-" {
			want = nil
		}
		pool := map[string]int{}
		for _, w := range want {
			pool[w]++
		}
		var got []string
		for _, d := range h.AI().Desires().Snapshot() {
			// A desire is shown in full when the line lists it so, else in
			// the short form that leaves out what the line did not ask.
			line := r.renderDesire(d, true)
			if pool[line] == 0 {
				line = r.renderDesire(d, false)
			}
			pool[line]--
			got = append(got, line)
		}
		slices.Sort(got)
		slices.Sort(want)
		return FirstDiff(ck.Args[0]+" desire", got, want)
	case "at":
		var x, y, z int
		if ck.Args[0] == "player" {
			x, y, z = r.srv.PlayerPosition(t, r.player)
		} else {
			n, _ := r.npcQueue(t, ck.Args[0])
			x, y, z = n.Position()
		}
		got := fmt.Sprintf("%d %d %d", x-r.origin.X, y-r.origin.Y, z-r.origin.Z)
		return FirstDiff(ck.Args[0]+" offset", []string{got}, []string{strings.Join(ck.Args[1:], " ")})
	}
	return ""
}

// liveNPC returns role's live hostile NPC.
func (r *run) liveNPC(t *testing.T, role string) *npc.Hostile {
	t.Helper()
	h := r.live[role]
	if h == nil {
		t.Fatalf("%s: NPC %s is no hostile NPC: it has no hate nor desires", r.sc.Name, role)
	}
	return h
}

// weight renders a hate or desire weight, in the fewest digits that read
// back as it.
func weight(w float64) string { return strconv.FormatFloat(w, 'f', -1, 64) }

// desire renders a queued desire as <kind>(<args>)=<weight> in its short form.
func (r *run) desire(d actorai.Desire) string { return r.renderDesire(d, false) }

// renderDesire renders a queued desire as <kind>(<args>)=<weight>: the args
// are an attack's, follow's or flee's target (a flee's distance after it), a
// cast's target and skill id ("move" after them when the NPC walks to its
// target), a move's destination as an offset from where the character
// first entered the world, a do-nothing's timer, and none for any other
// kind. A creature renders as its role, "?<object id>" for none. In full, an
// attack that holds its ground shows "hold" after its target and a cast its
// skill level after its id as <id>:<level>.
func (r *run) renderDesire(d actorai.Desire, full bool) string {
	var args []string
	switch d.Kind {
	case actorai.IntentionAttack:
		args = []string{r.named(d.FinalTarget)}
		if full && !d.MoveToTarget {
			args = append(args, "hold")
		}
	case actorai.IntentionFollow:
		args = []string{r.named(d.FinalTarget)}
	case actorai.IntentionFlee:
		args = []string{r.named(d.FinalTarget), strconv.Itoa(d.Distance)}
	case actorai.IntentionCast:
		skill := strconv.Itoa(int(d.Skill.ID))
		if full {
			skill += ":" + strconv.Itoa(d.Skill.Level)
		}
		args = []string{r.named(d.FinalTarget), skill}
		if d.MoveToTarget {
			args = append(args, "move")
		}
	case actorai.IntentionMoveTo:
		l := d.Location
		args = []string{strconv.Itoa(l.X - r.origin.X), strconv.Itoa(l.Y - r.origin.Y), strconv.Itoa(l.Z - r.origin.Z)}
	case actorai.IntentionNothing:
		args = []string{strconv.Itoa(d.Timer)}
	}
	return fmt.Sprintf("%s(%s)=%s", d.Kind, strings.Join(args, ","), weight(d.Weight))
}

// named renders a desire's creature by its role.
func (r *run) named(c attackable.Combatant) string {
	if c == nil {
		return "-"
	}
	if role, ok := r.roles[c.ObjectID()]; ok {
		return role
	}
	return "?" + strconv.Itoa(int(c.ObjectID()))
}
