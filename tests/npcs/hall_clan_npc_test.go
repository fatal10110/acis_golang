package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Npc.getClanId (Npc.java 1032-1041) is the clan the NPC's spawn
// gave it (ASpawn.doSpawn's clanId, which only the flag war hall sieges
// pass, FlagWar.java 557), else the owner of its residence.
// Monster.isAttackableBy (Monster.java 295-301) refuses a playable whose
// clan is that clan, so NpcInfo shows such a monster to its own clan as not
// attackable (isAttackableWithoutForceBy). AbstractNpcInfo's crest is the
// owner of the castle the NPC serves, else the clan getClanId names.

const (
	// flagWarWarriorID is a fixture Monster of the flag war warriors' kind
	// (35428, Oel Mahum Berserker).
	flagWarWarriorID = 35428
	// hallServantID is Dietrich, an NPC Devastated Castle lists, here a
	// fixture Monster.
	hallServantID = 35408
	hallClanQuest = "Q963_HallClan"
)

func hallClanTemplates() *npc.Table {
	tmpl := func(id int) *npc.Template {
		return &npc.Template{
			ID: id, TemplateID: id, Type: "Monster", Level: 60, HPMax: 1000,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		}
	}
	return npc.NewTable([]*npc.Template{tmpl(flagWarWarriorID), tmpl(hallServantID)})
}

// TestNPCSpawnedForAClanStandsForIt spawns flag war warriors for the
// Talker's clan Knights and for Rivals, and Devastated Castle servants with
// no clan of their own while Knights owns the hall. A monster standing for
// Knights, by its spawn or by the hall's owner, refuses the Talker's
// attacks and shows it no attack cursor; a monster spawned for Rivals stands
// for Rivals even in Knights' hall. Each shows the clan it stands for as its
// crest.
func TestNPCSpawnedForAClanStandsForIt(t *testing.T) {
	t.Parallel()
	w := bootHallNPCs(t, false, nil,
		gameservertest.WithNPCs(hallClanTemplates()),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithShowNPCCrest())
	if !w.siege.SetOwner(hallKnightsID) {
		t.Fatal("Knights could not take Devastated Castle")
	}
	drainUntilQuiet(t, w.srv.Client)
	for i, tt := range []struct {
		name   string
		npcID  int32
		clanID int32
		want   int32
	}{
		{"warrior of the Talker's clan", flagWarWarriorID, hallKnightsID, hallKnightsID},
		{"warrior of another clan", flagWarWarriorID, hallRivalsID, hallRivalsID},
		{"hall servant of no clan", hallServantID, 0, hallKnightsID},
		{"hall servant of another clan", hallServantID, hallRivalsID, hallRivalsID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			live, err := w.srv.NpcSpawns.SpawnForClan(tt.npcID, w.at.X+100+40*i, w.at.Y, w.at.Z, 0, tt.clanID)
			if err != nil {
				t.Fatalf("spawn %d for clan %#x: %v", tt.npcID, tt.clanID, err)
			}
			monster, ok := live.(*npc.Hostile)
			if !ok {
				t.Fatalf("spawned %T, want a hostile NPC", live)
			}
			frames := drainFrames(t, w.srv.Client)
			ownClan := tt.want == hallKnightsID
			if got := monster.Instance.NPCClanID(); got != tt.want {
				t.Fatalf("NPC clan = %#x, want %#x", got, tt.want)
			}
			if got := monster.AttackableBy(w.talker); got == ownClan {
				t.Fatalf("attackable by the Talker = %v, want %v", got, !ownClan)
			}
			wantFlag := int32(1)
			if ownClan {
				wantFlag = 0
			}
			if got := npcInfoAttackable(t, frames, monster.ObjectID()); got != wantFlag {
				t.Fatalf("NpcInfo attackable = %d, want %d", got, wantFlag)
			}
			if got := npcCrestFields(t, npcInfoOf(t, frames, monster.ObjectID())); got[0] != tt.want {
				t.Fatalf("NpcInfo clan = %#x, want %#x", got[0], tt.want)
			}
		})
	}
}

// TestScriptSpawnForClanAndHallOf spawns, from a script, a flag war warrior
// for Rivals and a Devastated Castle servant for no clan while Knights owns
// the hall: the script reads the warrior as standing for Rivals and serving
// no hall, and the servant as serving Devastated Castle and standing for
// its owner.
func TestScriptSpawnForClanAndHallOf(t *testing.T) {
	t.Parallel()
	w := bootHallNPCs(t, false, nil,
		gameservertest.WithNPCs(hallClanTemplates()),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithScripts([]script.Listing{{Path: "quest." + hallClanQuest}}, script.Catalog{
			"quest." + hallClanQuest: func() script.Script { return script.Script{Title: "Hall Clan", QuestID: 963} },
		}))
	if !w.siege.SetOwner(hallKnightsID) {
		t.Fatal("Knights could not take Devastated Castle")
	}
	ran := w.srv.RunScript(t, w.srv.SoleObjectID(t), hallClanQuest, func(s *script.Script, _ *script.Player) {
		at := script.Loc{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z}
		warrior := s.Spawner().SpawnForClan(flagWarWarriorID, at, hallRivalsID)
		if warrior == nil || warrior.ClanID() != hallRivalsID || s.HallOf(warrior) != nil {
			t.Errorf("warrior = %v: clan %#x, hall %v", warrior, warrior.ClanID(), s.HallOf(warrior))
		}
		at.X += 50
		servant := s.Spawner().SpawnForClan(hallServantID, at, 0)
		if servant == nil || servant.ClanID() != hallKnightsID {
			t.Errorf("servant = %v: clan %#x, want Knights", servant, servant.ClanID())
		}
		if h := s.HallOf(servant); h == nil || h.ID() != devastatedHall || h.OwnerID() != hallKnightsID {
			t.Errorf("servant's hall = %v", h)
		}
		if s.Spawner().SpawnForClan(1, at, hallRivalsID) != nil {
			t.Error("an NPC of no template spawned")
		}
	})
	if !ran {
		t.Fatal("the script did not run")
	}
}
