package npc

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
)

// ChamberlainStanding is how a castle chamberlain, or a wyvern manager
// running the chamberlain's dialog, answers a talker.
type ChamberlainStanding int

const (
	// StandingStranger is a talker of no clan, or of a clan not owning the
	// residence: every command opens the refusal page.
	StandingStranger ChamberlainStanding = iota
	// StandingUnderSiege is a clan member while the castle's siege runs:
	// every command opens the busy page.
	StandingUnderSiege
	// StandingMember is a member of the owning clan who does not lead it.
	StandingMember
	// StandingLord is the leader of the owning clan.
	StandingLord
)

// ChamberlainStanding is the talker's standing at the chamberlain dialog of
// an NPC of kind. A chamberlain answers only a clan member at an NPC of a
// castle as more than a stranger, as under siege while the castle's siege
// runs; a wyvern manager answers a member of the clan owning its residence,
// and never as under siege.
func (r ResidenceTalk) ChamberlainStanding(kind InstanceKind) ChamberlainStanding {
	switch {
	case r.ClanID == 0:
		return StandingStranger
	case kind == "CastleChamberlain" && r.CastleID == 0:
		return StandingStranger
	case kind == "CastleChamberlain" && r.SiegeInProgress:
		return StandingUnderSiege
	case !r.owns():
		return StandingStranger
	case r.ClanLeader:
		return StandingLord
	}
	return StandingMember
}

// ChamberlainRefusal is the page the chamberlain dialog of an NPC of kind
// answers a command with when the talker may not run it, "" when the talker
// may: the busy page while its castle's siege runs, the refusal page for a
// stranger or for a member not holding need.
func (r ResidenceTalk) ChamberlainRefusal(kind InstanceKind, need clan.Privilege) string {
	const dir = "data/html/chamberlain/"
	switch r.ChamberlainStanding(kind) {
	case StandingUnderSiege:
		return dir + "busy.htm"
	case StandingStranger:
		return dir + "noprivs.htm"
	case StandingMember, StandingLord:
	}
	if need != clan.PrivNone && !r.holds(need) {
		return dir + "noprivs.htm"
	}
	return ""
}

// Holds reports whether the talker holds privilege p in its clan.
func (r ResidenceTalk) Holds(p clan.Privilege) bool { return r.holds(p) }

// chamberlainCommands are the chamberlain dialog's own commands, by their
// first space-separated word matched in any case; caseSensitiveCommands
// match exactly.
var (
	chamberlainCommands = []string{
		"banish_foreigner", "banish_foreigner_show", "manage_functions", "products", "list_siege_clans",
		"receive_report", "items", "manage_vault", "operate_door", "tax_set", "tax_adjust", "manor",
		"siege_change", "siege_time_set", "castle_devices", "doors_update", "doors_choose_upgrade",
		"doors_confirm_upgrade", "traps_update", "traps_choose_upgrade", "traps_confirm_upgrade",
	}
	caseSensitiveCommands = []string{"give_crown", "manor_certificate", "validate_certificate"}
)

// ChamberlainWords splits a chamberlain dialog command into its words,
// around runs of spaces.
func ChamberlainWords(command string) []string {
	return strings.FieldsFunc(command, func(r rune) bool { return r == ' ' })
}

// chamberlainCommand reports whether command is one of the chamberlain
// dialog's own commands, manor_menu_select windows included. Any other
// command reaches the merchant's.
func chamberlainCommand(command string) bool {
	words := ChamberlainWords(command)
	if len(words) == 0 {
		return false
	}
	for _, c := range chamberlainCommands {
		if strings.EqualFold(words[0], c) {
			return true
		}
	}
	for _, c := range caseSensitiveCommands {
		if words[0] == c {
			return true
		}
	}
	return strings.HasPrefix(command, "manor_menu_select")
}

// chamberlainBypass answers a command at the chamberlain dialog of an NPC
// of kind: the busy page or the refusal page, alone, to a talker under
// siege or a stranger; the dialog's own commands for anyone else. ok is
// false for a command left to the merchant's.
func (f *Folk) chamberlainBypass(pages Pages, kind InstanceKind, r ResidenceTalk, command string, reply BypassReply) (BypassReply, bool) {
	if page := r.ChamberlainRefusal(kind, clan.PrivNone); page != "" {
		reply.Outcome, reply.HTML = BypassPage, rawPage(pages, page)
		return reply, true
	}
	if !chamberlainCommand(command) {
		return reply, false
	}
	reply.Outcome, reply.Standing = BypassChamberlain, r.ChamberlainStanding(kind)
	return reply, true
}

// DefaultSiegePreHour is a chamberlain's siege hour choice before one is
// made: the hour a siege starts at by default, less 12, which leaves the
// siege date as it is.
const DefaultSiegePreHour = 6

// SiegePreHour is this chamberlain's siege hour choice, 12 hours short of
// the hour it moves its castle's siege to.
func (f *Folk) SiegePreHour() int { return int(f.siegePreHour.Load()) }

// SetSiegePreHour makes hour this chamberlain's siege hour choice.
func (f *Folk) SetSiegePreHour(hour int) { f.siegePreHour.Store(int32(hour)) }
