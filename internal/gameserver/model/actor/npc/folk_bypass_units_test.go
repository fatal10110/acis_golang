package npc

import "testing"

type noPages struct{}

func (noPages) Get(string) (string, bool) { return "", false }

func digitsFolk(t *testing.T, kind string, id int) *Folk {
	t.Helper()
	inst, err := NewInstance(1, &Template{ID: id, TemplateID: id, Type: kind, Level: 1, HPMax: 100})
	if err != nil {
		t.Fatalf("%s: new instance: %v", kind, err)
	}
	f, err := NewFolk(inst)
	if err != nil {
		t.Fatalf("%s: new folk: %v", kind, err)
	}
	return f
}

// missingPage is the notice a dialog page absent from the pages reads as.
func missingPage(path string) string {
	return "<html><body>My html is missing:<br>" + path + "</body></html>"
}

// TestBypassArgumentsCountUTF16Units pins the dialog commands' character
// offsets to UTF-16 units: a non-ASCII separator ahead of the argument
// shifts it by one character, not by its UTF-8 length, and a supplementary
// character takes two. Only ASCII digits read as a number.
func TestBypassArgumentsCountUTF16Units(t *testing.T) {
	t.Parallel()
	merchant := digitsFolk(t, "Merchant", 30001)
	adventurer := digitsFolk(t, "Adventurer", 31729)
	smith := digitsFolk(t, "Trainer", 30300)

	for _, tc := range []struct {
		name    string
		f       *Folk
		command string
		want    BypassReply
	}{
		// Character four is skipped whatever its width.
		{"chat after ideographic space", merchant, "Chat　2", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/merchant/30001-2.htm")}},
		{"chat fullwidth digits", merchant, "Chat １２", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/merchant/30001.htm")}},
		// The supplementary character's trailing half starts the argument:
		// it reads as U+FFFD, no number, so page 0.
		{"chat split pair", merchant, "Chat\U0001D7CF1", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/merchant/30001.htm")}},
		{"buy fullwidth digits", merchant, "Buy １２３", BypassReply{Outcome: BypassAborted}},
		// The choice is character eight, after a one-character separator
		// of any width.
		{"augment after ideographic space", smith, "Augment　1", BypassReply{Outcome: BypassAugmentMake}},
		{"augment too short", smith, "Augment1", BypassReply{Outcome: BypassAborted}},
		{"raid info after ideographic space", adventurer, "raidInfo　20", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/adventurer_guildsman/raid_info/level20.htm")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.f.Bypass(noPages{}, ChatRules{}, Talker{Level: 20}, tc.command)
			got.LeadingActionFailed, got.CancelEnchant = false, false
			if got != tc.want {
				t.Fatalf("Bypass(%q) = %+v, want %+v", tc.command, got, tc.want)
			}
		})
	}
}

// TestCommandCharsCountsUTF16Units pins the offsets: a supplementary
// character takes two, and a range splitting one leaves its half as U+FFFD.
func TestCommandCharsCountsUTF16Units(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in         string
		begin, end int
		want       string
		ok         bool
	}{
		{"Chat", 5, -1, "", false},
		{"Chat ", 5, -1, "", true},
		{"Chat 1", 5, -1, "1", true},
		{"Chat　1", 5, -1, "1", true},
		{"Chat\U0001D7CF1", 5, -1, "�1", true},
		{"ab\U0001D7CF", 2, 4, "\U0001D7CF", true},
		{"ab\U0001D7CF", 3, 4, "�", true},
		{"Augment1", 8, 9, "", false},
	} {
		got, ok := commandChars(tc.in, tc.begin, tc.end)
		if got != tc.want || ok != tc.ok {
			t.Errorf("commandChars(%q, %d, %d) = %q, %v; want %q, %v", tc.in, tc.begin, tc.end, got, ok, tc.want, tc.ok)
		}
	}
}
