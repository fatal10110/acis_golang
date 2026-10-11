package questlog

import (
	"slices"
	"testing"
)

// TestQuestsListsEveryStateInJournalOrder: Quests names the quest of every
// state, in journal order and whatever its status (created, started,
// completed, a non-real quest), once per state: a quest Create added a
// second state of is listed twice.
func TestQuestsListsEveryStateInJournalOrder(t *testing.T) {
	var j Journal
	j.Restore(join(
		seed("Q003_WillTheSealBeBroken", "COMPLETED", "0"),
		seed("NoblesseTeleporter", "CREATED", "0"),
		seed("Q001_LettersOfLove", "STARTED", "1"),
	), resolve)
	j.Create(Quest{Name: "Q003_WillTheSealBeBroken", ID: 3})

	var got []string
	for _, q := range j.Quests() {
		got = append(got, q.Name)
	}
	want := []string{"Q003_WillTheSealBeBroken", "NoblesseTeleporter", "Q001_LettersOfLove", "Q003_WillTheSealBeBroken"}
	if !slices.Equal(got, want) {
		t.Fatalf("Quests() = %v, want %v", got, want)
	}
	if got := (&Journal{}).Quests(); len(got) != 0 {
		t.Fatalf("Quests() of an empty journal = %v, want none", got)
	}
}
