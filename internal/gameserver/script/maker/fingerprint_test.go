package maker

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes it ports: literals, engine calls and maker hook shapes.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.maker.DefaultMaker",
		"script.maker.NoOnStartMaker",
		"script.maker.EventMaker",
		"script.maker.CloseDoorMaker",
		"script.maker.DefaultUseDBMaker",
		"script.maker.FarmMaker",
		"script.maker.InstantSpawnMaker",
	},
		// Delays are durations: the reference's seconds-to-milliseconds
		// factor has no Go literal.
		"number 1000",
		// The catalog keys of the makers ported with the engine; the
		// reference binds its types in the npcmaker, not in a script class.
		`string "event_maker"`,
		`string "no_on_start_maker"`,
		// A maker reads its NPC's respawn delay from the spawn entry,
		// deletes the NPC through the group NPC handle and keeps its memo
		// writes in a cell: none is the script handle call the census names.
		"call ASpawn.getRespawnDelay",
		"call Npc.deleteMe",
		"call MemoSet.getBool",
		// The panic of a memo read before its write, where the reference
		// throws.
		`string "maker: memo enabled read before the maker started"`,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
