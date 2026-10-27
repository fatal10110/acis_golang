package scenario

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// stepSkills are the shipped skills the steps use: a skill hit's and a land
// step's. The scripts read them, as they read the npc lines' skills.
func (sc *Scenario) stepSkills() []modelskill.Ref {
	var refs []modelskill.Ref
	for _, st := range sc.Steps {
		switch st.Verb {
		case "hit":
			if h, err := parseHit(st.Args); err == nil && h.hasSkill {
				refs = append(refs, h.skill)
			}
		case "land":
			ref, _ := skillRef(st.Args[1], st.Args[2])
			refs = append(refs, ref)
		}
	}
	return refs
}

// usesRandomSteps reports whether a step draws its own script draws.
func (sc *Scenario) usesRandomSteps() bool {
	return slices.ContainsFunc(sc.Steps, func(st Step) bool { return st.Random != nil })
}

// push makes rd the script random source until pop: the step's own draws.
func (d *draws) push(rd *Random) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.saved = append(d.saved, savedDraws{random: d.random, next: d.next})
	d.random, d.next = rd, 0
}

// pop restores the random line's source and reports how many of the step's
// own draws were never drawn.
func (d *draws) pop() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	unused := 0
	if !d.random.Always {
		unused = max(len(d.random.Draws)-d.next, 0)
	}
	top := d.saved[len(d.saved)-1]
	d.saved = d.saved[:len(d.saved)-1]
	d.random, d.next = top.random, top.next
	return unused
}

// savedDraws is the random source a step's own draws replaced.
type savedDraws struct {
	random *Random
	next   int
}

// anyDraw is the script random source of a step with no listed draws.
func anyDraw(n int) int { return rand.IntN(n) }

// scratchOf returns the script memory of role's NPC.
func (r *run) scratchOf(t *testing.T, role string) *npc.Scratch {
	t.Helper()
	n, _ := r.npcQueue(t, role)
	s, ok := n.(interface{ Scratch() *npc.Scratch })
	if !ok {
		t.Fatalf("%s: NPC %s (%T) keeps no script memory", r.sc.Name, role, n)
	}
	return s.Scratch()
}

// memCreature returns the creature a mem value names: the character or an
// NPC line's NPC, nil for none.
func (r *run) memCreature(t *testing.T, value string) npc.ScratchCreature {
	t.Helper()
	if value == noCreature {
		return nil
	}
	c, _ := r.creature(t, value)
	return c
}

// setMemory gives the fresh NPC its line's script memory, once its created
// hook ran: a created hook that sets a slot would otherwise overwrite it.
func (r *run) setMemory(t *testing.T, n NPC) {
	t.Helper()
	if n.Mem == nil {
		return
	}
	s := r.scratchOf(t, n.Role)
	for _, m := range n.Mem {
		slot := memSlots[m.Slot]
		switch {
		case slot.isCrea:
			s.SetCreature(slot.creature, r.memCreature(t, m.Value))
		case slot.isValue:
			v, _ := strconv.ParseInt(m.Value, 10, 32)
			s.SetScriptValue(int32(v))
		default:
			v, _ := strconv.ParseInt(m.Value, 10, 32)
			s.SetInt(slot.integer, int32(v))
		}
	}
}

// party returns role's NPC as a member of a master's party.
func (r *run) party(t *testing.T, role string) npc.PartyMember {
	t.Helper()
	n, _ := r.npcQueue(t, role)
	m, ok := n.(npc.PartyMember)
	if !ok {
		t.Fatalf("%s: NPC %s (%T) has no party", r.sc.Name, role, n)
	}
	return m
}

// link raises the fresh NPC as the private of its line's master.
func (r *run) link(t *testing.T, n NPC) {
	t.Helper()
	if n.Master == "" {
		return
	}
	master, private := r.party(t, n.Master), r.party(t, n.Role)
	private.SetMaster(master)
	master.AddMinion(private)
}

// actorOf returns c as an effect participant: the character's own, or the NPC
// or summon.
func (r *run) actorOf(t *testing.T, c attackable.Combatant) interface {
	effect.Actor
	skillhandler.Actor
} {
	t.Helper()
	if h, ok := c.(player.CharacterHolder); ok {
		return h.PlayerCharacter()
	}
	a, ok := c.(interface {
		effect.Actor
		skillhandler.Actor
	})
	if !ok {
		t.Fatalf("%s: %T takes no effects", r.sc.Name, c)
	}
	return a
}

// character returns the online character.
func (r *run) character(t *testing.T) *player.Character {
	t.Helper()
	obj, ok := r.srv.State.Player(r.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("the character %T is not an online character", obj)
	}
	return c
}

// hostile returns role's live hostile NPC with the post of its queue.
func (r *run) hostile(t *testing.T, role string) (*npc.Hostile, func(func()) bool) {
	t.Helper()
	h := r.liveNPC(t, role)
	return h, h.Queue().Post
}

// state runs an hp, die, top, land, stance or swing step.
func (r *run) state(t *testing.T, verb string, args []string) {
	t.Helper()
	switch verb {
	case "hp":
		v, _ := strconv.Atoi(args[1])
		if args[0] == "player" {
			r.onPlayer(t, func(attackable.Combatant) { r.character(t).SetHP(float64(v)) })
			return
		}
		h, post := r.hostile(t, args[0])
		r.on(t, args[0], post, func() { h.SetCurrentHP(v) })
	case "die":
		var killer attackable.Combatant
		if len(args) == 1 {
			killer, _ = r.npcQueue(t, args[0])
		}
		r.onPlayer(t, func(attackable.Combatant) { r.character(t).Die(killer) })
		r.srv.Settle(t)
	case "top":
		h, post := r.hostile(t, args[0])
		var top attackable.Combatant
		if args[1] != noCreature {
			top, _ = r.creature(t, args[1])
		}
		r.on(t, args[0], post, func() { h.AI().SetTopDesireTarget(top) })
	case "stance":
		h, post := r.hostile(t, args[0])
		r.on(t, args[0], post, func() { h.SetInCombat(args[1] == "on") })
	case "land":
		r.land(t, args)
	case "swing":
		r.swing(t, args[0])
	}
}

// holds reports whether c holds an effect of skill id.
func (r *run) holds(t *testing.T, c attackable.Combatant, id modelskill.ID) bool {
	t.Helper()
	holder, ok := r.actorOf(t, c).(interface{ EffectList() *effect.List })
	if !ok {
		t.Fatalf("%s: %T holds no effects", r.sc.Name, c)
	}
	return slices.ContainsFunc(holder.EffectList().All(), func(e *effect.Effect) bool { return e.Skill.ID == id })
}

// land lands the effects of a shipped skill on the target, from the target
// itself or the creature by names, on the target's own queue, and waits until
// the target holds one.
func (r *run) land(t *testing.T, args []string) {
	t.Helper()
	ref, _ := skillRef(args[1], args[2])
	by := args[0]
	if len(args) == 5 {
		by = args[4]
	}
	def := shippedSkill(t, ref)
	to, post := r.creature(t, args[0])
	from, _ := r.creature(t, by)
	r.on(t, args[0], post, func() { skillhandler.LandEffects(r.actorOf(t, from), r.actorOf(t, to), def) })
	r.srv.Settle(t)
	var held bool
	r.on(t, args[0], post, func() { held = r.holds(t, to, ref.ID) })
	if !held {
		r.srv.AdvanceUntil(t, fmt.Sprintf("%s holds an effect of skill %d", args[0], ref.ID), func() bool {
			r.on(t, args[0], post, func() { held = r.holds(t, to, ref.ID) })
			return held
		})
	}
}

// swingStep is how much time a swing step lets pass at a time, and swingLimit
// how long it waits for the swing.
const (
	swingStep  = 100 * time.Millisecond
	swingLimit = 30 * time.Second
)

// swing runs the AI and the position ticks until role's NPC is attacking: its
// swing is in flight.
func (r *run) swing(t *testing.T, role string) {
	t.Helper()
	h, post := r.hostile(t, role)
	for passed := time.Duration(0); ; passed += swingStep {
		var now bool
		r.on(t, role, post, func() { now = h.AttackingNow() })
		if now {
			return
		}
		if passed >= swingLimit {
			t.Fatalf("%s: swing %s: still not attacking after %v", r.sc.Name, role, swingLimit)
		}
		r.srv.TickPositions()
		if err := r.srv.AI.Tick(); err != nil {
			t.Fatalf("AI tick: %v", err)
		}
		r.srv.Settle(t)
		r.srv.Advance(t, swingStep)
	}
}

// roleOf renders the creature of an object id by its role, "?<id>" for none.
func (r *run) roleOf(id int32) string {
	if role, ok := r.roles[id]; ok {
		return role
	}
	return "?" + strconv.Itoa(int(id))
}

// yesNo renders a flag as yes or no.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// stateCheck runs one mem, hp, privates, master, top, effect, dead or stance
// or inworld line, describing the first difference.
func (r *run) stateCheck(t *testing.T, ck Check) string {
	t.Helper()
	role := ck.Args[0]
	switch ck.Kind {
	case "mem":
		s := r.scratchOf(t, role)
		var got, want []string
		for _, a := range ck.Args[1:] {
			sets, _ := parseMem(a)
			slot := memSlots[sets[0].Slot]
			var v string
			switch {
			case slot.isCrea:
				v = noCreature
				if c := s.Creature(slot.creature); c != nil {
					v = r.roleOf(c.ObjectID())
				}
			case slot.isValue:
				v = strconv.Itoa(int(s.ScriptValue()))
			default:
				v = strconv.Itoa(int(s.Int(slot.integer)))
			}
			got, want = append(got, sets[0].Slot+"="+v), append(want, a)
		}
		return FirstDiff(role+" memory", got, want)
	case "hp":
		var hp int
		if role == "player" {
			hp = r.character(t).CurrentHP()
		} else {
			hp = r.liveNPC(t, role).CurrentHP()
		}
		return FirstDiff(role+" hp", []string{strconv.Itoa(hp)}, []string{ck.Args[1]})
	case "dead":
		var dead bool
		if role == "player" {
			dead = r.character(t).Dead()
		} else {
			dead = r.liveNPC(t, role).AlikeDead()
		}
		return FirstDiff(role+" dead", []string{yesNo(dead)}, []string{ck.Args[1]})
	case "inworld":
		_, in := r.srv.State.Object(r.objs[role])
		return FirstDiff(role+" in the world", []string{yesNo(in)}, []string{ck.Args[1]})
	case "stance":
		on := r.liveNPC(t, role).InCombat()
		return FirstDiff(role+" attack stance", []string{map[bool]string{true: "on", false: "off"}[on]}, []string{ck.Args[1]})
	case "top":
		got := noCreature
		if c := r.liveNPC(t, role).AI().TopDesireTarget(); c != nil {
			got = r.roleOf(c.ObjectID())
		}
		return FirstDiff(role+" top desire target", []string{got}, []string{ck.Args[1]})
	case "master":
		got := noCreature
		if m := r.party(t, role).Master(); m != nil {
			got = r.roleOf(m.ObjectID())
		}
		return FirstDiff(role+" master", []string{got}, []string{ck.Args[1]})
	case "privates":
		counts := map[int]int{}
		for _, m := range r.party(t, role).Minions() {
			if dead, ok := m.(interface{ AlikeDead() bool }); ok && dead.AlikeDead() {
				continue
			}
			counts[m.Template().ID]++
		}
		var got []string
		for id, n := range counts {
			got = append(got, fmt.Sprintf("%d=%d", id, n))
		}
		want := slices.Clone(ck.Args[1:])
		if len(want) == 1 && want[0] == "-" {
			want = nil
		}
		slices.Sort(got)
		slices.Sort(want)
		return FirstDiff(role+" live privates", got, want)
	case "effect":
		id, _ := strconv.ParseInt(ck.Args[1], 10, 32)
		c, post := r.creature(t, role)
		var held bool
		r.on(t, role, post, func() { held = r.holds(t, c, modelskill.ID(id)) })
		return FirstDiff(fmt.Sprintf("%s effect of skill %d", role, id), []string{map[bool]string{true: "held", false: "none"}[held]}, []string{ck.Args[2]})
	}
	return strings.Join(ck.Args, " ")
}
