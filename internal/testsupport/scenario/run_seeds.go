package scenario

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// stanceClock is the time the attack stances run on in a scenario that ends
// the character's: it starts with the driven clock and moves with the advance
// steps.
type stanceClock struct{ ns atomic.Int64 }

func (c *stanceClock) now() time.Time { return time.Unix(0, c.ns.Load()) }

// stancePlayer reports whether a step sets or ends the character's attack
// stance.
func (sc *Scenario) stancePlayer() bool {
	for _, st := range sc.Steps {
		if st.Verb == "stance" && st.Args[0] == "player" {
			return true
		}
	}
	return false
}

// stanceNow is the clock the attack stances time out by: the wall clock,
// which nothing ends a stance by in a run, or, when a step sets the
// character's stance, the driven one.
func (r *run) stanceNow() func() time.Time {
	if !r.sc.stancePlayer() {
		return time.Now
	}
	r.stance.ns.Store(r.clockStart().UnixNano())
	return r.stance.now
}

// passStanceTime moves the stance clock by d and ends the stances that are
// over, as the production ticker does as time passes.
func (r *run) passStanceTime(t *testing.T, d time.Duration) {
	t.Helper()
	if !r.sc.stancePlayer() {
		return
	}
	r.stance.ns.Add(int64(d))
	if err := r.srv.AttackStance.Tick(); err != nil {
		t.Fatalf("%s: attack stance tick: %v", r.sc.Name, err)
	}
	r.srv.Settle(t)
}

// stanceActor is the character as the attack stance task tracks it.
func (r *run) stanceActor(t *testing.T) task.AttackStanceActor {
	t.Helper()
	obj, ok := r.srv.State.Player(r.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	a, ok := obj.(task.AttackStanceActor)
	if !ok {
		t.Fatalf("the character %T holds no attack stance", obj)
	}
	return a
}

// playerStance sets or ends the character's attack stance, as a landed hit
// sets it.
func (r *run) playerStance(t *testing.T, on bool) {
	t.Helper()
	if on {
		r.srv.AttackStance.Add(r.stanceActor(t))
	} else {
		r.srv.AttackStance.Remove(r.stanceActor(t))
	}
	r.character(t).SetInCombat(on)
	r.srv.Settle(t)
}

// playerInStance reports whether the character holds an attack stance: it is
// in combat and the stance task still tracks it.
func (r *run) playerInStance(t *testing.T) bool {
	t.Helper()
	return r.character(t).InCombat() && r.srv.AttackStance.InAttackStance(r.stanceActor(t))
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// zoneIndex is the zone index the server runs with: the zone lines' zones
// and the summon line's siege battlefield; nil for none.
func (r *run) zoneIndex(t *testing.T) *zone.Index {
	t.Helper()
	if len(r.sc.Zones) == 0 && (r.sc.Summon == nil || r.sc.Summon.Servitor) {
		return nil
	}
	at := fixtureSpawn()
	zones := zone.NewIndex()
	if r.sc.Summon != nil && !r.sc.Summon.Servitor {
		form, err := zone.NewCuboid(at.X-battlefieldReach, at.X+battlefieldReach, at.Y-battlefieldReach, at.Y+battlefieldReach,
			at.Z-battlefieldReach, at.Z+battlefieldReach)
		if err != nil {
			t.Fatalf("%s: summon battlefield: %v", r.sc.Name, err)
		}
		field := zone.NewSiege(battlefieldID, form)
		field.SetActive(true)
		zones.Add(field)
	}
	for _, z := range r.sc.Zones {
		form, err := zone.NewCuboid(at.X+z.DX1, at.X+z.DX2, at.Y+z.DY1, at.Y+z.DY2, at.Z+z.DZ1, at.Z+z.DZ2)
		if err != nil {
			t.Fatalf("%s: zone %d: %v", r.sc.Name, z.ID, err)
		}
		switch z.Kind {
		case "boss":
			zones.Add(zone.NewBoss(z.ID, form))
		case "peace":
			zones.Add(zone.NewPeace(z.ID, form))
		}
	}
	return zones
}

// seedMember gives a member line's character its standing and its items,
// before it enters the world.
func (r *run) seedMember(t *testing.T, obj int32, m Member) {
	t.Helper()
	if m.Standing != 0 {
		if _, err := r.srv.DB.ExecContext(context.Background(), "UPDATE characters SET varka_ketra_ally = ? WHERE obj_Id = ?", m.Standing, obj); err != nil {
			t.Fatalf("%s: member %s: seed standing: %v", r.sc.Name, m.Name, err)
		}
	}
	for _, h := range m.Holds {
		tmpl, ok := r.items.Get(h.Item)
		switch {
		case !ok:
			t.Fatalf("%s: member %s: hold %d: no such template in the catalog; add an item line", r.sc.Name, m.Name, h.Item)
		case tmpl.Stackable:
			r.srv.GiveItem(t, obj, h.Item, h.Count)
		default:
			for range h.Count {
				r.srv.GiveItem(t, obj, h.Item, 1)
			}
		}
	}
}

// otherCharacter is the object id of the member or mate line named name.
func (r *run) otherCharacter(t *testing.T, name string) int32 {
	t.Helper()
	for i, m := range r.sc.Members {
		if m.Name == name {
			return r.members[i].obj
		}
	}
	return mateObject(r.mateIndex(t, name))
}

// seedCheck runs an open, dungeon or standing check.
func (r *run) seedCheck(t *testing.T, ck Check) string {
	t.Helper()
	switch ck.Kind {
	case "open":
		d, _ := r.sc.door(ck.Args[0])
		obj, ok := r.srv.WorldObjects.Door(d.ID)
		if !ok {
			t.Fatalf("%s: door %s is not in the world", r.sc.Name, d.Role)
		}
		return FirstDiff("door "+d.Role+" open", []string{yesNo(obj.Opened())}, []string{ck.Args[1]})
	case "dungeon":
		return FirstDiff("Seven Signs dungeon flag", []string{yesNo(r.character(t).In7sDungeon())}, []string{ck.Args[0]})
	}
	// standing
	got := r.character(t).VarkaKetraAlliance()
	if ck.Args[0] != "player" {
		obj, ok := r.srv.State.Player(r.otherCharacter(t, ck.Args[0]))
		if !ok {
			t.Fatalf("%s: member %s is not in the world", r.sc.Name, ck.Args[0])
		}
		c, ok := network.OnlineCharacter(obj)
		if !ok {
			t.Fatalf("%s: member %s (%T) has no standing", r.sc.Name, ck.Args[0], obj)
		}
		got = c.VarkaKetraAlliance()
	}
	return FirstDiff(fmt.Sprintf("%s standing", ck.Args[0]), []string{strconv.Itoa(got)}, []string{ck.Args[1]})
}

// A teleport line that lands anywhere in a box: the point and how far each of
// x and y may be off it (a script's TeleportTo with an offset draws it from
// the server's own source, not the random line's).
var (
	wantNearTeleport = regexp.MustCompile(`^(TeleportToLocation obj=\S+ loc=)(-?\d+),(-?\d+),(-?\d+)\+-(\d+)( fast=[01])$`)
	gotTeleport      = regexp.MustCompile(`^(TeleportToLocation obj=\S+ loc=)(-?\d+),(-?\d+),(-?\d+)( fast=[01])$`)
)

// matchNearTeleports returns got with each TeleportToLocation line that lands
// inside the box its wanted line (loc=x,y,z+-n) names, at the box's height,
// replaced by the wanted line.
func matchNearTeleports(got, want []string) []string {
	out := slices.Clone(got)
	for i := range min(len(got), len(want)) {
		w, g := wantNearTeleport.FindStringSubmatch(want[i]), gotTeleport.FindStringSubmatch(got[i])
		if w == nil || g == nil || w[1] != g[1] || w[6] != g[5] {
			continue
		}
		n := atoi(w[5])
		if abs(atoi(g[2])-atoi(w[2])) <= n && abs(atoi(g[3])-atoi(w[3])) <= n && g[4] == w[4] {
			out[i] = want[i]
		}
	}
	return out
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
