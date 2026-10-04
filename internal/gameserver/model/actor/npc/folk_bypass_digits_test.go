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

// TestBypassArgumentsReadUnicodeDigits pins the dialog commands' integer
// arguments to the reference's Integer.parseInt over String.substring: any
// Basic Multilingual Plane decimal digit reads as its value (fullwidth
// "１２" and Arabic-Indic "١٢" are 12, as a Java probe on OpenJDK 21
// prints), a digit outside that plane does not (parseInt reads UTF-16
// units, and a surrogate is no digit), and character offsets count UTF-16
// units, so a non-ASCII character ahead of the argument shifts it by one
// character, not by its UTF-8 length.
func TestBypassArgumentsReadUnicodeDigits(t *testing.T) {
	t.Parallel()
	merchant := digitsFolk(t, "Merchant", 30001)
	adventurer := digitsFolk(t, "Adventurer", 31729)
	gatekeeper := digitsFolk(t, "Gatekeeper", 30006)
	smith := digitsFolk(t, "Trainer", 30300)

	for _, tc := range []struct {
		name    string
		f       *Folk
		command string
		want    BypassReply
	}{
		{"chat fullwidth", merchant, "Chat １２", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/merchant/30001-12.htm")}},
		{"chat arabic-indic", merchant, "Chat ١٢", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/merchant/30001-12.htm")}},
		{"chat devanagari", merchant, "Chat १", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/merchant/30001-1.htm")}},
		{"chat negative fullwidth", merchant, "Chat -１", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/merchant/30001--1.htm")}},
		{"chat mixed ascii and fullwidth", merchant, "Chat 1２", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/merchant/30001-12.htm")}},
		// Character four is skipped whatever its width.
		{"chat after ideographic space", merchant, "Chat　２", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/merchant/30001-2.htm")}},
		// A mathematical bold digit is outside the plane: page 0.
		{"chat supplementary digit", merchant, "Chat \U0001D7CF", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/merchant/30001.htm")}},
		// Its low half starts the argument: page 0.
		{"chat split surrogate", merchant, "Chat\U0001D7CF1", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/merchant/30001.htm")}},
		// A fullwidth letter is a digit only above base ten.
		{"chat fullwidth letter", merchant, "Chat ａ", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/merchant/30001.htm")}},
		{"chat overflow", merchant, "Chat ２１４７４８３６４８", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/merchant/30001.htm")}},

		{"buy fullwidth", merchant, "Buy １２３", BypassReply{Outcome: BypassBuyList, ListID: 123}},
		{"buy arabic-indic", merchant, "buy ١٢٣", BypassReply{Outcome: BypassBuyList, ListID: 123}},
		{"buy supplementary", merchant, "Buy \U0001D7CF", BypassReply{Outcome: BypassAborted}},

		{"teleport fullwidth", gatekeeper, "teleport ７", BypassReply{Outcome: BypassTeleport, Index: 7}},
		{"instant teleport arabic-indic", gatekeeper, "instant_teleport ٧", BypassReply{Outcome: BypassInstantTeleport, Index: 7}},
		{"teleport fullwidth letter", gatekeeper, "teleport ７ａ", BypassReply{Outcome: BypassReleased}},

		{"augment fullwidth", smith, "Augment １", BypassReply{Outcome: BypassAugmentMake}},
		{"augment arabic-indic", smith, "Augment ٢", BypassReply{Outcome: BypassAugmentCancel}},
		{"augment fullwidth other", smith, "Augment ３", BypassReply{Outcome: BypassRefused}},
		// The choice is character eight, after a one-character separator
		// of any width.
		{"augment after ideographic space", smith, "Augment　１", BypassReply{Outcome: BypassAugmentMake}},
		{"augment too short", smith, "Augment１", BypassReply{Outcome: BypassAborted}},
		{"augment supplementary", smith, "Augment \U0001D7CF", BypassReply{Outcome: BypassAborted}},

		{"raid info fullwidth", adventurer, "raidInfo ２０", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/adventurer_guildsman/raid_info/level20.htm")}},
		{"raid info arabic-indic zero", adventurer, "raidInfo ٠", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/adventurer_guildsman/raid_info/info.htm")}},
		{"raid info after ideographic space", adventurer, "raidInfo　２０", BypassReply{Outcome: BypassChatWindow, HTML: missingPage("data/html/adventurer_guildsman/raid_info/level20.htm")}},
		{"raid info supplementary", adventurer, "raidInfo \U0001D7CF", BypassReply{Outcome: BypassAborted}},
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
// character takes two, and a range splitting one keeps its half as a
// replacement character.
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
		{"Chat １", 5, -1, "１", true},
		{"Chat　１", 5, -1, "１", true},
		{"Chat\U0001D7CF1", 5, -1, "�1", true},
		{"ab\U0001D7CF", 2, 4, "\U0001D7CF", true},
		{"ab\U0001D7CF", 3, 4, "�", true},
		{"Augment１", 8, 9, "", false},
	} {
		got, ok := commandChars(tc.in, tc.begin, tc.end)
		if got != tc.want || ok != tc.ok {
			t.Errorf("commandChars(%q, %d, %d) = %q, %v; want %q, %v", tc.in, tc.begin, tc.end, got, ok, tc.want, tc.ok)
		}
	}
}
