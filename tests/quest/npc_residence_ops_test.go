package quest

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The NPC handle's cast animation (#3609) and the door and castle
// operations of the behavior base's helpers (#3597), driven through a
// booted world as a hook calls them.

// fullMSU is a whole MagicSkillUse: its head, then the target's position.
type fullMSU struct {
	head       msu
	targetLoc  [3]int32
	successful int32
}

func readFullMSU(f opsFrame) fullMSU {
	m := fullMSU{head: readMSU(f)}
	m.successful = f.r.ReadInt32()
	m.targetLoc = [3]int32{f.r.ReadInt32(), f.r.ReadInt32(), f.r.ReadInt32()}
	return m
}

// TestNPCBroadcastMagicSkillUse: Q641's npc.broadcastPacket(new
// MagicSkillUse(npc, player, 5089, 1, 3000, 0)) from a civilian NPC, and
// the same from a hostile NPC with the player casting on the NPC. Every
// player who knows the NPC sees one MagicSkillUse naming the caster and the
// target where each stands, and nothing else; a player who does not know
// the NPC sees nothing.
func TestNPCBroadcastMagicSkillUse(t *testing.T) {
	t.Parallel()
	srv, leader, _, mateClient := partyPair(t, "1", "1")
	x, y, z := srv.PlayerPosition(t, leader)
	statue := script.NPCOf(srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 32109), location.Location{X: x + 40, Y: y, Z: z}))
	monster := script.NPCOf(srv.SpawnHostileNPCAt(t, location.Location{X: x - 40, Y: y, Z: z}))
	far := script.NPCOf(srv.SpawnHostileNPCAt(t, location.Location{X: x + 30000, Y: y, Z: z}))
	srv.ReadQueued(t, srv.Client)
	srv.ReadQueued(t, mateClient)

	run(t, srv, leader, func(_ *script.Script, p *script.Player) {
		statue.BroadcastMagicSkillUse(statue, p, 5089, 1, 3000, 0)
		monster.BroadcastMagicSkillUse(p, monster, 4339, 2, 6000, 1)
		far.BroadcastMagicSkillUse(far, far, 4339, 1, 1, 1)
	})

	st, mo := script.LocOf(statue), script.LocOf(monster)
	want := []fullMSU{
		{
			head:      msu{caster: statue.ObjectID(), target: leader, skill: 5089, level: 1, hitTime: 3000, reuse: 0, x: int32(st.X), y: int32(st.Y), z: int32(st.Z)},
			targetLoc: [3]int32{int32(x), int32(y), int32(z)},
		},
		{
			head:      msu{caster: leader, target: monster.ObjectID(), skill: 4339, level: 2, hitTime: 6000, reuse: 1, x: int32(x), y: int32(y), z: int32(z)},
			targetLoc: [3]int32{int32(mo.X), int32(mo.Y), int32(mo.Z)},
		},
	}
	for who, c := range map[string][][]byte{"leader": srv.ReadQueued(t, srv.Client), "mate": srv.ReadQueued(t, mateClient)} {
		frames := framesOf(c, serverpackets.OpcodeMagicSkillUse)
		var got []fullMSU
		for _, f := range frames {
			got = append(got, readFullMSU(f))
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s saw %+v, want %+v", who, got, want)
		}
	}
}

// scriptGateID is a door the scripts below open and close by name.
const scriptGateID = 19210002

func scriptGate() *door.Template {
	const x = 40
	return &door.Template{
		ID: scriptGateID, Name: "script_gate", Kind: door.KindDoor, Level: 1,
		Position:    location.Location{X: x, Y: 0, Z: 0},
		Coordinates: []location.Point{{X: x - 8, Y: -8}, {X: x + 8, Y: -8}, {X: x + 8, Y: 8}, {X: x - 8, Y: 8}},
		HP:          100, PDef: 10, MDef: 10, Height: 32,
	}
}

// doorUpdates returns the closed flag of each DoorStatusUpdate of the door
// objID among frames.
func doorUpdates(frames [][]byte, objID int32) []int32 {
	var out []int32
	for _, f := range framesOf(frames, serverpackets.OpcodeDoorStatusUpdate) {
		if f.r.ReadInt32() == objID {
			out = append(out, f.r.ReadInt32())
		}
	}
	return out
}

// TestOpenCloseDoorByName drives openCloseDoor (DefaultNpc lines 270-280):
// the door is found by name ignoring case (DoorData.getDoor), state 0 opens
// it and any other state closes it, each change shown once to the players
// around it; a door already in that state, or a name no door has, changes
// nothing.
func TestOpenCloseDoorByName(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t, gameservertest.WithDoors(scriptGate()))
	gate, ok := srv.WorldObjects.Door(scriptGateID)
	if !ok {
		t.Fatal("door not spawned")
	}
	srv.ReadQueued(t, srv.Client)

	step := func(name string, state int32, wantOpen bool, wantUpdates ...int32) {
		t.Helper()
		run(t, srv, objID, func(sc *script.Script, _ *script.Player) {
			defaultnpc.OpenCloseDoor(sc, name, state)
		})
		if gate.Opened() != wantOpen {
			t.Fatalf("%q state %d: opened %v, want %v", name, state, gate.Opened(), wantOpen)
		}
		if got := doorUpdates(srv.ReadQueued(t, srv.Client), gate.ObjectID()); !slices.Equal(got, wantUpdates) {
			t.Fatalf("%q state %d: DoorStatusUpdate closed flags %v, want %v", name, state, got, wantUpdates)
		}
	}
	step("SCRIPT_GATE", 0, true, 0)
	step("script_gate", 0, true)
	step("script_gate", 1, false, 1)
	step("script_gate", 2, false)
	step("Script_Gate", 0, true, 0)
	step("no_such_gate", 1, true)

	run(t, srv, objID, func(sc *script.Script, _ *script.Player) {
		if sc.Door("no_such_gate") != nil {
			t.Error("Door(no_such_gate) found a door")
		}
	})
}

// Clans of the castle world below: the player's clan, and the clan that
// owns Gludio Castle when the player's does not.
const (
	pledgeClanID   = 0x70000031
	pledgeOwnerID  = 0x70000032
	pledgeLordID   = 0x70000033
	gludioNPC      = 35100 // in Gludio Castle's residence NPCs
	gludioCastleID = 1
	noCastleNPC    = 31646
)

// seedRow is one statement of a castle world's seed.
type seedRow struct {
	q    string
	args []any
}

// bootPledgeWorld boots the player in a clan, with the castles and their
// sieges; seed adds rows once the clans are in.
func bootPledgeWorld(t *testing.T, seed ...seedRow) (*gameservertest.Server, int32) {
	t.Helper()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	return bootHelpers(t,
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			rows := append([]seedRow{
				{"UPDATE characters SET clanid = ? WHERE char_name = 'Newbie'", []any{pledgeClanID}},
				{`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
					SELECT ?, 'Pledge', 5, obj_Id FROM characters WHERE char_name = 'Newbie'`, []any{pledgeClanID}},
				{"INSERT INTO characters (account_name, obj_Id, char_name, level, clanid) VALUES ('owner', ?, 'Lord', 40, ?)", []any{pledgeLordID, pledgeOwnerID}},
				{"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (?, 'Lords', 5, ?)", []any{pledgeOwnerID, pledgeLordID}},
			}, seed...)
			for _, r := range rows {
				if _, err := db.ExecContext(context.Background(), r.q, r.args...); err != nil {
					t.Fatalf("seed clans: %s: %v", r.q, err)
				}
			}
		}),
	)
}

// pledgeStates returns, from one invocation, getPledgeCastleState of the
// Gludio NPC towards the player, then of the castle-less NPC towards the
// player, then of the Gludio NPC towards the castle-less NPC.
func pledgeStates(t *testing.T, srv *gameservertest.Server, objID int32, gludio, other *script.NPC) [3]int32 {
	t.Helper()
	var got [3]int32
	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		got = [3]int32{
			defaultnpc.PledgeCastleState(sc, gludio, p),
			defaultnpc.PledgeCastleState(sc, other, p),
			defaultnpc.PledgeCastleState(sc, gludio, other),
		}
	})
	return got
}

// spawnPledgeNPCs places an NPC of Gludio Castle and one of no castle by
// the player.
func spawnPledgeNPCs(t *testing.T, srv *gameservertest.Server, objID int32) (gludio, other *script.NPC) {
	t.Helper()
	x, y, z := srv.PlayerPosition(t, objID)
	gludio = script.NPCOf(srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", gludioNPC), location.Location{X: x + 40, Y: y, Z: z}))
	other = script.NPCOf(srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", noCastleNPC), location.Location{X: x - 40, Y: y, Z: z}))
	return gludio, other
}

func gludioSiege(t *testing.T, srv *gameservertest.Server) *siege.Siege {
	t.Helper()
	s, ok := srv.Sieges.Get(gludioCastleID)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	return s
}

// TestPledgeCastleStateAttacker drives getPledgeCastleState (DefaultNpc
// lines 282-304) for a clan attacking Gludio Castle, owned by another clan:
// 0 before the siege, 1 while it is fought. An NPC of no castle, and a
// creature no player controls, give 0 throughout.
func TestPledgeCastleStateAttacker(t *testing.T) {
	t.Parallel()
	srv, objID := bootPledgeWorld(t,
		seedRow{"UPDATE clan_data SET hasCastle = ? WHERE clan_id = ?", []any{gludioCastleID, pledgeOwnerID}},
		seedRow{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, 'ATTACKER')", []any{gludioCastleID, pledgeClanID}},
	)
	gludio, other := spawnPledgeNPCs(t, srv, objID)

	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		c := sc.Castle(gludio)
		if c == nil || c.ID() != gludioCastleID {
			t.Errorf("Castle(Gludio NPC) = %v, want castle %d", c, gludioCastleID)
			return
		}
		if sc.Castle(other) != nil {
			t.Error("an NPC of no castle has a castle")
		}
		if c.SiegeInProgress() {
			t.Error("siege in progress before it starts")
		}
		if !c.OnSiegeSide(p, script.SiegeAttacker) || c.OnSiegeSide(p, script.SiegeDefender, script.SiegeOwner) {
			t.Error("the player's clan is not on the attacking side alone")
		}
		if !p.InClan() || p.ClanCastleID() != 0 {
			t.Errorf("InClan %v, ClanCastleID %d, want true, 0", p.InClan(), p.ClanCastleID())
		}
	})
	if got := pledgeStates(t, srv, objID, gludio, other); got != [3]int32{0, 0, 0} {
		t.Fatalf("before the siege: %v, want [0 0 0]", got)
	}

	gludioSiege(t, srv).Start()
	if got := pledgeStates(t, srv, objID, gludio, other); got != [3]int32{1, 0, 0} {
		t.Fatalf("during the siege: %v, want [1 0 0]", got)
	}
}

// TestPledgeCastleStateOwner: the clan owning Gludio Castle gives 2 before
// its siege (it owns the castle) and while it is fought (the owner side).
func TestPledgeCastleStateOwner(t *testing.T) {
	t.Parallel()
	srv, objID := bootPledgeWorld(t, seedRow{"UPDATE clan_data SET hasCastle = ? WHERE clan_id = ?", []any{gludioCastleID, pledgeClanID}})
	gludio, other := spawnPledgeNPCs(t, srv, objID)

	if got := pledgeStates(t, srv, objID, gludio, other); got != [3]int32{2, 0, 0} {
		t.Fatalf("before the siege: %v, want [2 0 0]", got)
	}
	s := gludioSiege(t, srv)
	if s.Side(pledgeClanID) != siege.SideOwner {
		t.Fatalf("owner clan side %s, want OWNER", s.Side(pledgeClanID))
	}
	s.Start()
	if got := pledgeStates(t, srv, objID, gludio, other); got != [3]int32{2, 0, 0} {
		t.Fatalf("during the siege: %v, want [2 0 0]", got)
	}
}

// TestPledgeCastleStateNoClan: a player in no clan gives 0, even towards
// an NPC of a castle.
func TestPledgeCastleStateNoClan(t *testing.T) {
	t.Parallel()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	srv, objID := bootHelpers(t, gameservertest.WithCastles(castles))
	gludio, other := spawnPledgeNPCs(t, srv, objID)
	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		if p.InClan() {
			t.Error("a clanless player is in a clan")
		}
		if sc.Castle(gludio) == nil {
			t.Error("the Gludio NPC has no castle")
		}
	})
	if got := pledgeStates(t, srv, objID, gludio, other); got != [3]int32{0, 0, 0} {
		t.Fatalf("clanless: %v, want [0 0 0]", got)
	}
}
