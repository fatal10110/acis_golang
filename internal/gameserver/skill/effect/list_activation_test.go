package effect

import (
	"slices"
	"sync"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// modLedger is a StatOwner that counts the stat funcs each effect holds
// attached, safe for concurrent adds.
type modLedger struct {
	mu       sync.Mutex
	attached map[*Effect]int
}

func newModLedger() *modLedger { return &modLedger{attached: make(map[*Effect]int)} }

func (o *modLedger) AttachStatFuncs(fns []Mod) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, fn := range fns {
		o.attached[fn.Owner.effect]++
	}
}

func (o *modLedger) RemoveStatsByOwner(owner ModOwner) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.attached, owner.effect)
}

func (o *modLedger) held() map[*Effect]int {
	o.mu.Lock()
	defer o.mu.Unlock()
	held := make(map[*Effect]int, len(o.attached))
	for e, n := range o.attached {
		held[e] = n
	}
	return held
}

func (*modLedger) StatFuncsAttached([]Mod)                    {}
func (*modLedger) MaxBuffCount() int                          { return 20 }
func (*modLedger) UpdateEffectIcons()                         {}
func (*modLedger) NotifyEffectWornOff(modelskill.ID, int)     {}
func (*modLedger) NotifyEffectDisappeared(modelskill.ID, int) {}
func (*modLedger) NotifyEffectAborted(modelskill.ID, int)     {}
func (*modLedger) NotifyEffectFelt(modelskill.ID, int)        {}

// moddedEffect is a buff carrying one stat func, so its activation attaches
// something its removal must take back.
func moddedEffect(name string, stackType string, stackOrder float64) *Effect {
	e := newEffect(name, 1, stackType, stackOrder, false)
	e.Funcs = []Mod{{Stat: stat.PowerAttack, Op: OpAdd, Value: 10, Owner: ModOwnerEffect(e)}}
	return e
}

// TestListConcurrentAddLeavesNoOrphanMods lands a second buff from another
// goroutine while the first one's start hook runs outside the list lock:
// the same buff again, or a higher stack order of its group. The second add
// retires or displaces the first before the first's activation applies, and
// that stale activation must attach nothing: once every effect left is
// removed, the owner holds no stat funcs.
func TestListConcurrentAddLeavesNoOrphanMods(t *testing.T) {
	cases := []struct {
		name          string
		env           Env
		stackType     string
		first, second float64
	}{
		{name: "same buff, no stack group", stackType: "none"},
		{name: "same buff, stacked", stackType: "pa_up", first: 1, second: 1},
		{name: "higher stack order", stackType: "pa_up", first: 1, second: 2},
		{name: "higher stack order, lesser kept", env: Env{KeepLesser: true}, stackType: "pa_up", first: 1, second: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ledger := newModLedger()
			list := newTestList(ledger, WithEnv(tc.env))
			first := moddedEffect("first", tc.stackType, tc.first)
			second := moddedEffect("second", tc.stackType, tc.second)
			exits := 0
			first.OnExit = func(*Effect) { exits++ }
			first.OnStart = func(*Effect) bool {
				done := make(chan struct{})
				go func() {
					defer close(done)
					list.Add(second)
				}()
				<-done
				return true
			}

			list.Add(first)

			if first.InUse() {
				t.Error("first buff in use after a concurrent add replaced it, want it inactive")
			}
			if exits != 1 {
				t.Errorf("first buff exit hook ran %d times, want 1 to undo its start", exits)
			}
			if !second.InUse() {
				t.Error("second buff not in use, want it active")
			}
			for _, e := range list.All() {
				list.Remove(e)
			}
			if held := ledger.held(); len(held) != 0 {
				names := make([]string, 0, len(held))
				for e := range held {
					names = append(names, e.Template.Name)
				}
				slices.Sort(names)
				t.Fatalf("owner still holds stat funcs of %v after every effect was removed", names)
			}
		})
	}
}

// TestListFailedStartKeepsEffectHeld pins an effect whose start hook fails:
// it stays held and in use, its flag counts and it shows its icon, but it
// attaches no stat funcs and sends no felt message. Its first tick still
// runs its action, then ends it with the normal expiry message and no exit
// hook.
func TestListFailedStartKeepsEffectHeld(t *testing.T) {
	for _, tc := range []struct {
		name    string
		count   int
		message string
	}{
		{name: "last tick", count: 1, message: "worn-off:7:0"},
		{name: "ticks left", count: 3, message: "disappeared:7:0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			list := newTestList(iconEventOwner{eventOwner{events: &events}})
			e := newEffect("held", 7, "fear", 1, true)
			e.Template.Icon, e.Template.Count, e.Template.Time = true, tc.count, 30
			e.Flag = FlagFear
			e.Funcs = []Mod{{Stat: stat.PowerAttack, Op: OpAdd, Value: 10, Owner: ModOwnerEffect(e)}}
			e.OnStart = func(*Effect) bool { events = append(events, "start"); return false }
			e.OnAction = func(*Effect) bool { events = append(events, "action"); return true }
			e.OnExit = func(*Effect) { events = append(events, "exit") }

			list.Add(e)
			requireEvents(t, events, []string{"start", "icons"})
			if !e.InUse() || !list.IsAffected(FlagFear) {
				t.Fatalf("in use = %v, flag affected = %v after a failed start, want both", e.InUse(), list.IsAffected(FlagFear))
			}
			if entries := list.IconEntries(time.Now()); len(entries) != 1 || entries[0].ID != 7 {
				t.Fatalf("icon entries = %+v, want the held effect", entries)
			}

			events = nil
			list.tickAt(time.Now().Add(31 * time.Second))
			requireEvents(t, events, []string{"action", "owner:remove:held", tc.message, "icons"})
			if len(list.All()) != 0 {
				t.Fatal("failed-start effect still held after its first tick, want it gone")
			}
		})
	}
}

// TestListFailedStartTakesStackHead pins a stacked newcomer whose start
// fails: it still takes its stack group's head, so the previous head loses
// it (its exit hook runs) and, when promoted back by the newcomer's removal,
// that previous head starts again.
func TestListFailedStartTakesStackHead(t *testing.T) {
	var events []string
	list := newTestList(iconEventOwner{eventOwner{events: &events}}, WithEnv(Env{KeepLesser: true}))
	weak := namedEffect("weak", 1, "speed", 1, false, &events)
	strong := namedEffect("strong", 2, "speed", 2, false, &events)
	strong.OnStart = func(*Effect) bool { events = append(events, "strong:start"); return false }
	list.Add(weak)
	list.Add(strong)
	if weak.InUse() || !strong.InUse() {
		t.Fatalf("weak/strong in use = %v/%v, want the failed-start newcomer heading the group", weak.InUse(), strong.InUse())
	}

	events = nil
	list.Remove(strong)
	requireEvents(t, events, []string{"owner:remove:strong", "weak:start", "owner:add", "icons"})
	if !weak.InUse() {
		t.Fatal("weak not promoted after the failed-start head left")
	}
}
