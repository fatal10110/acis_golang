package flagwar

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// FlagWar class. The reference names its system messages by their
// SystemMessageId constants, which the port writes as ids, and adds the
// number of a countdown through SystemMessage.addNumber, which the port
// sets on the message. It keeps its registrations in SQL of its own and
// logs; the port's hall runtime stores them, and nothing is logged. Its
// delayed steps are pool tasks, which the port runs as script timers of
// its own names. Its warriors and flags spawn through ASpawn.doSpawn, the
// port's through Spawner.SpawnForClan (the Spawn.new row). The siege's end
// sets every attacker's flag, siege state and user info through the hall's
// ReleaseAttackers (Clan.getOnlineMembers, Player.broadcastUserInfo), and
// arms the next close through ArmPrepare (getNextSiegeTime, its hour
// ahead, and the date it logs). The page directory, "siegablehall", is
// the hall script's, and a member list's capacity, 18, has no Go
// counterpart. The port panics where the reference reads a new owner it
// never registered or a maker it lacks.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.siegablehall.FlagWar"},
		"number 789", "number 790", "number 791", "number 815", "number 844", "number 857", "number 858", "number 1826", "number 1828",
		"call SystemMessage.addNumber",
		`string "Couldn't clear data tables for {}."`, `string "Couldn't generate spawns for {}."`, `string "Couldn't load attackers for {}."`,
		`string "Couldn't save NPCs for {}."`, `string "Couldn't save clans for {}."`, `string "Couldn't save members for {}."`,
		`string "Couldn't save owner's npc for {}."`, `string "Siege of {} scheduled for {}."`,
		`string "DELETE FROM clanhall_flagwar_attackers WHERE clanhall_id = ?"`, `string "DELETE FROM clanhall_flagwar_members WHERE clanhall_id = ?"`,
		`string "DELETE FROM clanhall_flagwar_owner_npcs WHERE clan_id = ?"`, `string "INSERT INTO clanhall_flagwar_attackers VALUES(?,?,?,?)"`,
		`string "INSERT INTO clanhall_flagwar_members VALUES (?,?,?)"`, `string "INSERT INTO clanhall_flagwar_owner_npcs VALUES (?,?,?)"`,
		`string "SELECT * FROM clanhall_flagwar_attackers WHERE clanhall_id = ?"`, `string "SELECT * FROM clanhall_flagwar_owner_npcs WHERE clanhall_id = ?"`,
		`string "SELECT object_id FROM clanhall_flagwar_members WHERE clan_id = ?"`, `string "UPDATE clanhall_flagwar_attackers SET npc = ? WHERE clan_id = ?"`,
		`string "clan_id"`, `string "flag"`, `string "npc"`, `string "npc_id"`, `string "object_id"`,
		"call ThreadPool.schedule",
		`string "flagwar-final"`, `string "flagwar-move-to-center"`, `string "flagwar-one-minute"`, `string "flagwar-seconds-"`, `string "flagwar-tryouts"`,
		"call ASpawn.doSpawn",
		"call Clan.getOnlineMembers", "call Player.broadcastUserInfo",
		"call SiegableHall.getNextSiegeTime", "call SiegableHall.getSiegeDate", "number 3600000",
		`string "siegablehall"`, "number 18",
		`string "flagwar: hall %d's new owner %d is not registered"`, `string "flagwar: no npcmaker "`,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
