package npc

import "testing"

// olympiadNoble is a talker every OlympiadNoble gate lets through.
var olympiadNoble = Talker{Level: 76, Noble: true, ThirdClass: true}

// TestOlympiadNobleCommand pins OlympiadManagerNpc.onBypassFeedback's
// OlympiadNoble branch: a cursed weapon, a subclass, then no noble or no
// third occupation turn the talker away with their page, in that order and
// whatever the command; the cursed weapon's page keeps %objectId% unfilled.
// The choice is read from the fifteenth character: none, or one that does
// not parse, stops the handling; any number is passed on.
func TestOlympiadNobleCommand(t *testing.T) {
	t.Parallel()
	manager := digitsFolk(t, "OlympiadManagerNpc", 31688)
	for _, tc := range []struct {
		name    string
		talker  Talker
		command string
		want    BypassReply
	}{
		{
			"cursed weapon first",
			Talker{CursedWeapon: true, SubclassActive: true},
			"OlympiadNoble 4",
			BypassReply{Outcome: BypassPage, HTML: missingPage(olympiadPages + "noble_cant_cw.htm")},
		},
		{
			"subclass",
			Talker{SubclassActive: true, Noble: true, ThirdClass: true},
			"OlympiadNobleX",
			BypassReply{Outcome: BypassPage, HTML: missingPage(olympiadPages + "noble_cant_sub.htm")},
		},
		{
			"not noble",
			Talker{ThirdClass: true},
			"OlympiadNoble 4",
			BypassReply{Outcome: BypassPage, HTML: missingPage(olympiadPages + "noble_cant_thirdclass.htm")},
		},
		{
			"second occupation",
			Talker{Noble: true},
			"OlympiadNoble 4",
			BypassReply{Outcome: BypassPage, HTML: missingPage(olympiadPages + "noble_cant_thirdclass.htm")},
		},
		{"register", olympiadNoble, "OlympiadNoble 5", BypassReply{Outcome: BypassOlympiadNoble, Index: 5}},
		{"trade", olympiadNoble, "OlympiadNoble 10", BypassReply{Outcome: BypassOlympiadNoble, Index: 10}},
		{"unknown choice", olympiadNoble, "OlympiadNoble 99", BypassReply{Outcome: BypassOlympiadNoble, Index: 99}},
		{"no choice", olympiadNoble, "OlympiadNoble", BypassReply{Outcome: BypassAborted}},
		{"choice not a number", olympiadNoble, "OlympiadNoble x", BypassReply{Outcome: BypassAborted}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := manager.Bypass(noPages{}, ChatRules{}, tc.talker, tc.command)
			got.LeadingActionFailed, got.CancelEnchant = false, false
			if got != tc.want {
				t.Fatalf("Bypass(%q) = %+v, want %+v", tc.command, got, tc.want)
			}
		})
	}
}

// TestOlympiadClassRankingCommand pins "Olympiad 2_<class>": the class is
// read from the twelfth character; 88 to 118 is ranked, any other class
// answers nothing, and a missing or unparsable class stops the handling.
func TestOlympiadClassRankingCommand(t *testing.T) {
	t.Parallel()
	manager := digitsFolk(t, "OlympiadManagerNpc", 31688)
	for _, tc := range []struct {
		command string
		want    BypassReply
	}{
		{"Olympiad 2_88", BypassReply{Outcome: BypassClassRanking, Index: 88}},
		{"Olympiad 2_118", BypassReply{Outcome: BypassClassRanking, Index: 118}},
		{"Olympiad 2_87", BypassReply{Outcome: BypassRefused}},
		{"Olympiad 2_119", BypassReply{Outcome: BypassRefused}},
		{"Olympiad 2", BypassReply{Outcome: BypassAborted}},
		{"Olympiad 2_x", BypassReply{Outcome: BypassAborted}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			got := manager.Bypass(noPages{}, ChatRules{}, olympiadNoble, tc.command)
			got.LeadingActionFailed, got.CancelEnchant = false, false
			if got != tc.want {
				t.Fatalf("Bypass(%q) = %+v, want %+v", tc.command, got, tc.want)
			}
		})
	}
}

// TestOlympiadManagerChat pins OlympiadManagerNpc.showChatWindow for a Chat
// command: the Grand Olympiad Manager shows noble_<val>.htm, noble.htm for
// 0, or noble_main.htm for a noble's 0; a Monument of Heroes its main page
// whatever val.
func TestOlympiadManagerChat(t *testing.T) {
	t.Parallel()
	manager := digitsFolk(t, "OlympiadManagerNpc", 31688)
	monument := digitsFolk(t, "OlympiadManagerNpc", 31690)
	for _, tc := range []struct {
		name    string
		f       *Folk
		talker  Talker
		command string
		page    string
	}{
		{"page", manager, olympiadNoble, "Chat 10", "noble_10.htm"},
		{"first page", manager, Talker{}, "Chat 0", "noble.htm"},
		{"noble first page", manager, olympiadNoble, "Chat 0", "noble_main.htm"},
		{"monument", monument, Talker{}, "Chat 3", "hero_main2.htm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.f.Bypass(noPages{}, ChatRules{}, tc.talker, tc.command)
			want := BypassReply{Outcome: BypassChatWindow, HTML: missingPage(olympiadPages + tc.page)}
			got.LeadingActionFailed, got.CancelEnchant = false, false
			if got != want {
				t.Fatalf("Bypass(%q) = %+v, want %+v", tc.command, got, want)
			}
		})
	}
}
