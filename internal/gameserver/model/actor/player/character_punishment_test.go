package player

import "testing"

// TestSetPunishmentBanKeepsState pins that a character or account ban never
// becomes the stored punishment (Punishment.setType changes the access level
// and logs out for CHAR/ACC without assigning the type): a served chat ban
// and its timer stay as they were.
func TestSetPunishmentBanKeepsState(t *testing.T) {
	for _, kind := range []Punishment{PunishCharacter, PunishAccount} {
		c := &Character{ID: 1}
		c.RestorePunishment(int(PunishChat), 60000)
		if got := c.SetPunishment(kind, 5); got != PunishmentKept {
			t.Fatalf("SetPunishment(%d) = %d, want PunishmentKept", kind, got)
		}
		if got, timer := c.Punishment(); got != PunishChat || timer != 60000 {
			t.Fatalf("after SetPunishment(%d): punishment = %d/%d, want the chat ban 60000", kind, got, timer)
		}
	}
}

// TestReplacePunishmentChecksCurrent pins #5129: a replacement applies only
// over the punishment the caller expects, so an unban of a chat ban never
// lifts a jail term that landed meanwhile, and a chat ban over one that did
// is reported kept.
func TestReplacePunishmentChecksCurrent(t *testing.T) {
	c := &Character{ID: 1}
	c.RestorePunishment(int(PunishJail), 60000)
	if got := c.ReplacePunishment(PunishChat, PunishNone, 0); got != PunishmentKept {
		t.Fatalf("lift of a chat ban over a jail = %d, want PunishmentKept", got)
	}
	if got := c.ReplacePunishment(PunishNone, PunishChat, 5); got != PunishmentKept {
		t.Fatalf("chat ban over a jail = %d, want PunishmentKept", got)
	}
	if got, timer := c.Punishment(); got != PunishJail || timer != 60000 {
		t.Fatalf("punishment = %d/%d, want the jail 60000", got, timer)
	}

	c.RestorePunishment(int(PunishChat), 0)
	if got := c.ReplacePunishment(PunishChat, PunishNone, 0); got != ChatBanLifted {
		t.Fatalf("lift of a served chat ban = %d, want ChatBanLifted", got)
	}
	if got := c.ReplacePunishment(PunishNone, PunishChat, 5); got != ChatBanStarted {
		t.Fatalf("chat ban over none = %d, want ChatBanStarted", got)
	}
	if got, timer := c.Punishment(); got != PunishChat || timer != PunishmentMillis(5) {
		t.Fatalf("punishment = %d/%d, want a 5-minute chat ban", got, timer)
	}
}
