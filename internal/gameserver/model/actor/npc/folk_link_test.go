package npc

import "testing"

// TestFolkLinkCountsUTF16Units pins a civilian NPC's "Link <path>" to the
// generic dialog's substring(5) plus trim: the path starts at UTF-16 unit
// five whatever the width of character four, a command too short to hold
// unit five stops the handling, and a path climbing out with ".." is
// refused.
func TestFolkLinkCountsUTF16Units(t *testing.T) {
	t.Parallel()
	merchant := digitsFolk(t, "Merchant", 30001)
	for _, tc := range []struct {
		name    string
		command string
		want    BypassReply
	}{
		// Character four is skipped as one unit, not as half of its UTF-8
		// encoding.
		{"non-ascii separator", "Linkéfoo.htm", BypassReply{Outcome: BypassPage, HTML: missingPage("data/html/foo.htm")}},
		{"non-ascii path", "Link é.htm", BypassReply{Outcome: BypassPage, HTML: missingPage("data/html/é.htm")}},
		{"trimmed", "Link \tmerchant/30001-1.htm \n", BypassReply{Outcome: BypassPage, HTML: missingPage("data/html/merchant/30001-1.htm")}},
		{"too short", "Link", BypassReply{Outcome: BypassAborted}},
		{"empty path", "Linké", BypassReply{Outcome: BypassPage, HTML: missingPage("data/html/")}},
		{"climbs out", "Link ../config.txt", BypassReply{Outcome: BypassRefused}},
	} {
		if got := merchant.Bypass(noPages{}, ChatRules{}, Talker{}, tc.command); got != tc.want {
			t.Errorf("%s: Bypass(%q) = %+v, want %+v", tc.name, tc.command, got, tc.want)
		}
	}
}

// TestParseDialogCommand pins the generic dialog commands every NPC that
// talks shares: TerritoryStatus in any case, Quest[ <name>], Chat <n> with
// page 0 for a missing or unparsable n, and Link <path>.
func TestParseDialogCommand(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		command string
		want    DialogReply
	}{
		{"territorystatus", DialogReply{Command: DialogTerritoryStatus}},
		{"TerritoryStatus 1", DialogReply{Command: DialogOther}},
		{"Quest", DialogReply{Command: DialogQuest}},
		{"Quest Q00001_LettersOfLove", DialogReply{Command: DialogQuest}},
		{"Chat 3", DialogReply{Command: DialogChat, Val: 3}},
		{"Chat", DialogReply{Command: DialogChat}},
		{"Chat x", DialogReply{Command: DialogChat}},
		{"Linkéfoo.htm", DialogReply{Command: DialogLink, Path: "data/html/foo.htm"}},
		{"Link", DialogReply{Command: DialogAborted}},
		{"Link a/../b", DialogReply{Command: DialogRefused}},
		{"Loto 1", DialogReply{Command: DialogOther}},
	} {
		if got := ParseDialogCommand(tc.command); got != tc.want {
			t.Errorf("ParseDialogCommand(%q) = %+v, want %+v", tc.command, got, tc.want)
		}
	}
}
