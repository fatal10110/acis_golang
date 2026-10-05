package scriptcontract

// Contract is one engine contract of the script engine plan (section 11, item
// 3), with the goldens that pin it and the slices that implement it.
type Contract struct {
	Name string
	// Slices are the plan's slice ids that implement the contract and must run
	// its goldens.
	Slices []string
	Tables []string
}

// Contracts lists every engine contract and its goldens.
var Contracts = []Contract{
	{
		Name:   "cond and flags bits",
		Slices: []string{"E3"},
		Tables: []string{"journal.cond_flags", "journal.get_flags"},
	},
	{
		Name:   "set cond and exit: packet and statement order",
		Slices: []string{"E3"},
		Tables: []string{"journal.write_order"},
	},
	{
		Name:   "quest list contents",
		Slices: []string{"E2", "E3"},
		Tables: []string{"questlist.packet"},
	},
	{
		Name:   "the four drop types with roll counts and a fractional rate",
		Slices: []string{"E4"},
		Tables: []string{"drop.divmod", "drop.fixed_rate", "drop.fixed_count", "drop.fixed_both", "drop.multiple"},
	},
	{
		Name:   "range checks: strict less-than, centre to centre",
		Slices: []string{"E5"},
		Tables: []string{"range.quest_event", "range.interact"},
	},
	{
		Name:   "dialog rules (plan section 7)",
		Slices: []string{"E5"},
		Tables: []string{"dialog.last_quest_npc", "dialog.general_window", "dialog.single_window", "dialog.quest_bypass", "dialog.npc_bypass", "dialog.quest_equality"},
	},
	{
		Name:   "fan-out rules (plan section 5)",
		Slices: []string{"A6", "A7"},
		Tables: []string{"fanout.hit", "fanout.aggression", "fanout.skill", "fanout.party_died", "fanout.clan_died", "fanout.clan_range", "fanout.line_of_sight"},
	},
	{
		Name:   "timer rules",
		Slices: []string{"E7"},
		Tables: []string{"timers.identity", "timers.one_shot", "timers.fixed_rate", "timers.cancel", "timers.liveness"},
	},
	{
		Name:   "the first-aggro tick",
		Slices: []string{"A10"},
		Tables: []string{"aggro.first_tick", "aggro.see_once", "aggro.scan_gates", "aggro.look_neighbor", "aggro.teleport_see"},
	},
	{
		Name:   "schedule calendar",
		Slices: []string{"T1"},
		Tables: []string{"schedule.calendar", "schedule.rescan"},
	},
}
