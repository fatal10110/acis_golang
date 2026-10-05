package script

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
)

// TestJournalQuestResolvesNamesLikeTheScriptList pins how a journal row's
// quest name finds its script: by the last element of the listed path,
// ignoring case, the first registered match in list order winning; a name
// no registered script carries, including a listed but missing script's,
// resolves to nothing.
func TestJournalQuestResolvesNamesLikeTheScriptList(t *testing.T) {
	catalog := Catalog{
		"quest.Q001_LettersOfLove":       func() Script { return Script{QuestID: 1} },
		"ai.group.q001_lettersoflove":    func() Script { return Script{QuestID: -1} },
		"teleport.NoblesseTeleporter":    func() Script { return Script{} },
		"quest.Q002_WhatWomenWant.Inner": func() Script { return Script{QuestID: 2} },
	}
	r, _ := build(t, listOf("quest.Q001_LettersOfLove", "ai.group.q001_lettersoflove", "teleport.NoblesseTeleporter", "quest.Q002_WhatWomenWant.Inner", "quest.Gone"), catalog)

	for _, tc := range []struct {
		name string
		want questlog.Quest
		ok   bool
	}{
		{"Q001_LettersOfLove", questlog.Quest{Name: "Q001_LettersOfLove", ID: 1}, true},
		{"q001_LETTERSOFLOVE", questlog.Quest{Name: "Q001_LettersOfLove", ID: 1}, true},
		{"NoblesseTeleporter", questlog.Quest{Name: "NoblesseTeleporter"}, true},
		{"Inner", questlog.Quest{Name: "Inner", ID: 2}, true},
		{"Q002_WhatWomenWant", questlog.Quest{}, false},
		{"Gone", questlog.Quest{}, false},
		{"", questlog.Quest{}, false},
	} {
		got, ok := r.JournalQuest(tc.name)
		if got != tc.want || ok != tc.ok {
			t.Errorf("JournalQuest(%q) = %+v, %v; want %+v, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}
