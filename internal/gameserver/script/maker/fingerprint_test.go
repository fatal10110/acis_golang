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
		"script.maker.InstantSpawnSerialLastMaker",
		"script.maker.InstantSpawnSerialMaker",
		"script.maker.OnDayNightSpawnMaker",
		"script.maker.OpenDoorMaker",
		"script.maker.ParentSpawnAllMaker",
		"script.maker.ExclusiveDespawnNormalMaker",
		"script.maker.ExclusiveSpawnNormalMaker",
		"script.maker.InstantSpawnRandomMaker",
		"script.maker.RandomSpawnMaker",
		"script.maker.RandomSpawnPawnMaker",
		"script.maker.UniqueNpcKillEventMaker",
		"script.maker.AlarmDeviceMaker1",
		"script.maker.AlarmDeviceMaker2",
		"script.maker.AlarmDeviceMaker3",
		"script.maker.AlarmDeviceMaker4",
		"script.maker.HallKeeperDefaultMaker",
		"script.maker.RainbowMaker",
		"script.maker.UndeadBandmasterMaker",
		"script.maker.WarriorPassiveWeaknessMaker",
		"script.maker.ManageTeleportDungeonMaker",
		"script.maker.RoyalRushMaker",
		"script.maker.RoyalReqNextMaker",
		"script.maker.RoyalSpawnTreasureBoxMaker",
		"script.maker.BenomMaker",
		"script.maker.FreyaDeaconKeeperMaker",
		"script.maker.FrintezzaEvilateMaker",
		"script.maker.SailrenDummyMaker",
		"script.maker.SculptureGardenMaker",
		"script.maker.SculptureIceFairyMaker",
		"script.maker.StatueOfShilenMaker",
		"script.maker.VelociraptorMaker",
		"script.maker.RhamphorhynchusMaker",
		"script.maker.TyrannosaurusMaker",
		"script.maker.DefaultUseDBMakerForFrintezza",
		"script.maker.IceFairySirrMaker",
		"script.maker.SailrenMaker",
	},
		// Delays are durations: the reference's seconds-to-milliseconds
		// factor and its one-minute period in milliseconds have no Go
		// literal.
		"number 1000",
		"number 60000",
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
		// A Royal Rush boss maker reads the id of the NPC that left off its
		// spawn entry, which every NPC of the spawn has.
		"call Npc.getNpcId",
		// A rift room tells its managers script event 2 or 3 as a number,
		// where the reference parses it from the event's name.
		"number 3",
		// The panics of a memo read before its write, of a memo value that
		// is no integer and of a memo the npcmaker does not declare, where
		// the reference throws.
		`string "maker: memo enabled read before the maker started"`,
		`string "maker: memo %s read before the maker wrote it"`,
		`string "maker: memo %s = %q is not an integer"`,
		`string "maker: memo manager_npc_name is not declared"`,
		// The panic of a door looked up by name that does not exist, where
		// the reference throws.
		`string "maker: no door %q"`,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
