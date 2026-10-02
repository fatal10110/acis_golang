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
