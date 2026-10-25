package script

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
)

// HandleOps lists, per Hooks field a script sets ("OnAttacked"), the
// kind-specific handle operations that hook reaches ("NPC.HP"). A scan of
// the script sources derives it; the boot passes it in Config.
type HandleOps map[string][]string

// hostileOnly is the kind set of an operation only a hostile NPC has.
const hostileOnly kindSet = 1 << KindHostile

// kindOps lists the NPC kinds each kind-specific handle operation exists
// for; an operation not listed exists for every NPC kind. These are the
// operations that go through (*NPC).hostile, which panics on any other NPC;
// a test checks the list against the source.
var kindOps = map[string]kindSet{
	"NPC.ActualSpoiler":               hostileOnly,
	"NPC.AttackingNow":                hostileOnly,
	"NPC.CanAutoAttack":               hostileOnly,
	"NPC.CanAttemptCast":              hostileOnly,
	"NPC.CastUnchecked":               hostileOnly,
	"NPC.CastingNow":                  hostileOnly,
	"NPC.CleanAllHate":                hostileOnly,
	"NPC.DisableCoreAI":               hostileOnly,
	"NPC.DropItem":                    hostileOnly,
	"NPC.Earthquake":                  hostileOnly,
	"NPC.EquipItem":                   hostileOnly,
	"NPC.HateList":                    hostileOnly,
	"NPC.InCombat":                    hostileOnly,
	"NPC.InsideZone":                  hostileOnly,
	"NPC.LastAttacker":                hostileOnly,
	"NPC.MaxMP":                       hostileOnly,
	"NPC.MeetsHPMPConditions":         hostileOnly,
	"NPC.MeetsHPMPDisabledConditions": hostileOnly,
	"NPC.Muted":                       hostileOnly,
	"NPC.PhysicalMuted":               hostileOnly,
	"NPC.RandomizeAttack":             hostileOnly,
	"NPC.RemoveAggro":                 hostileOnly,
	"NPC.RemoveAttackDesire":          hostileOnly,
	"NPC.RemoveDesires":               hostileOnly,
	"NPC.Rooted":                      hostileOnly,
	"NPC.SetEnchantEffect":            hostileOnly,
	"NPC.SetTopDesireTarget":          hostileOnly,
	"NPC.StopAttack":                  hostileOnly,
	"NPC.TopDesireTarget":             hostileOnly,
	"Spawner.DecayMe":                 hostileOnly,
	"Spawner.RespawnDelay":            hostileOnly,
	"Spawner.ScheduleRespawn":         hostileOnly,
}

// hookByField returns the hook of the Hooks field name; the fields are in
// hook order.
func hookByField(name string) (hook, bool) {
	t := reflect.TypeFor[Hooks]()
	for h := range hookCount {
		if t.Field(int(h)).Name == name {
			return h, true
		}
	}
	return 0, false
}

// gateHandles checks that every kind-specific handle operation a hook of s
// reaches exists for the kind of every NPC that hook is raised on: the NPCs
// bound to the hook's events, or, for a hook raised for no NPC event (timer,
// quest event, start, item use, zone entry, death), every NPC s is bound
// to.
func gateHandles(s *Script, bound Bindings, cfg *Config) error {
	ops := cfg.HandleOps[s.path]
	for _, field := range slices.Sorted(maps.Keys(ops)) {
		h, ok := hookByField(field)
		if !ok {
			return fmt.Errorf("handle operations of %s: no such hook", field)
		}
		ids := subjects(h, bound)
		for _, op := range ops[field] {
			ks, ok := kindOps[op]
			if !ok {
				return fmt.Errorf("%s calls %s, which is no kind-specific handle operation", h, op)
			}
			for _, id := range ids {
				if k, _ := cfg.KindOf(id); ks&kinds(k) == 0 && !silent(h, k) {
					return fmt.Errorf("%s on npc %d calls %s, which %s npcs do not have", h, id, op, k)
				}
			}
		}
	}
	return nil
}

// subjects returns, in event and id order, the NPC ids hook h of a script
// bound as bound runs for.
func subjects(h hook, bound Bindings) []int32 {
	npcHook := false
	for ev := range npcEventCount {
		if npcEvents[ev].hook == h {
			npcHook = true
		}
	}
	var out []int32
	for ev := range npcEventCount {
		if npcHook && npcEvents[ev].hook != h {
			continue
		}
		for _, id := range bound[ev] {
			if !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}
