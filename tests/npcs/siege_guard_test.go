package npcs

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The besieged world: the Talker leads Knights, attacking Gludio Castle;
// the Keeper leads Rivals, defending it.
const (
	besiegedKnightsID int32 = 0x7f310101
	besiegedRivalsID  int32 = 0x7f310102
	besiegedKeeperID  int32 = 0x7f310001
	gludioCastleID          = 1
	// gludioSorcererID is a Court Sorcerer, a siege guard Gludio Castle
	// lists among its NPCs.
	gludioSorcererID = 35080
)

// besiegedWorld is the Talker and the Keeper on Gludio's battlefield.
type besiegedWorld struct {
	srv                *gameservertest.Server
	talker, keeper     *testsupport.ScriptedClient
	talkerID, keeperID int32
	at                 location.Location
	gludio             *siege.Siege
}

// battlefieldBox is a zone volume over the shared spawn.
func battlefieldBox(t *testing.T) zone.Form {
	t.Helper()
	form, err := zone.NewCuboid(-1_000, 1_000, -1_000, 1_000, -10_000, 10_000)
	if err != nil {
		t.Fatalf("zone form: %v", err)
	}
	return form
}

// shippedCastles loads the shipped castles.xml.
func shippedCastles(t *testing.T) gameservertest.Option {
	t.Helper()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	return gameservertest.WithCastles(castles)
}

// bootBesiegedWorld boots the shipped castles with Gludio's battlefield
// over the shared spawn and both clans registered on its siege, and brings
// the Talker, holding adena, and the Keeper into the world, starting
// Gludio's siege first when started is set so its start banishes nobody.
func bootBesiegedWorld(t *testing.T, started bool, adena int32, extra ...gameservertest.Option) *besiegedWorld {
	t.Helper()
	return bootBesiegedWorldPrepared(t, started, adena, nil, extra...)
}

// bootBesiegedWorldPrepared is bootBesiegedWorld running prepare, when
// set, on the booted server before anyone comes into the world.
func bootBesiegedWorldPrepared(t *testing.T, started bool, adena int32, prepare func(srv *gameservertest.Server, talkerID int32), extra ...gameservertest.Option) *besiegedWorld {
	t.Helper()
	zones := zone.NewIndex()
	field := zone.NewSiege(1, battlefieldBox(t))
	field.ResidenceID = gludioCastleID
	zones.Add(field)
	// Water east of the battlefield shows a still NPC entering it by its
	// ServerObjectInfo.
	zones.Add(zone.NewWater(2, zoneBox(t, 1_200, 2_000)))
	knights, rivals := strconv.Itoa(int(besiegedKnightsID)), strconv.Itoa(int(besiegedRivalsID))
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		shippedCastles(t),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithZones(zones),
		gameservertest.WithSeed(func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
			ch, err := player.NewCharacter(besiegedKeeperID, gameservertest.ClassTemplate(), "player2", "Keeper", playerLevel, 0, 0, player.SexMale)
			if err != nil {
				t.Fatalf("seed Keeper: %v", err)
			}
			if err := chars.Create(context.Background(), ch); err != nil {
				t.Fatalf("seed Keeper: %v", err)
			}
		}),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, stmt := range []string{
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
					SELECT ` + knights + `, 'Knights', 5, obj_Id FROM characters WHERE char_name = 'Talker'`,
				`UPDATE characters SET clanid = ` + knights + ` WHERE char_name = 'Talker'`,
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
					VALUES (` + rivals + `, 'Rivals', 5, ` + strconv.Itoa(int(besiegedKeeperID)) + `)`,
				`UPDATE characters SET clanid = ` + rivals + ` WHERE char_name = 'Keeper'`,
				`INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, ` + knights + `, 'ATTACKER')`,
				`INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, ` + rivals + `, 'DEFENDER')`,
			} {
				if _, err := db.ExecContext(context.Background(), stmt); err != nil {
					t.Fatalf("seed siege clans: %v", err)
				}
			}
		}),
	}, extra...)...)
	w := &besiegedWorld{srv: srv, talker: srv.Client, talkerID: srv.SoleObjectID(t), keeperID: besiegedKeeperID}
	gludio, ok := srv.Sieges.Get(gludioCastleID)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	w.gludio = gludio
	if adena > 0 {
		srv.GiveItem(t, w.talkerID, item.AdenaID, adena)
	}
	if prepare != nil {
		prepare(srv, w.talkerID)
	}
	if started {
		w.startSiege(t)
	}
	startInWorld(t, srv, w.talker)
	w.keeper = srv.DialClient(t, "player2", 1)
	startInWorld(t, srv, w.keeper)
	drainUntilQuiet(t, w.talker)
	x, y, z := srv.PlayerPosition(t, w.talkerID)
	w.at = location.Location{X: x, Y: y, Z: z}
	for _, id := range []int32{w.talkerID, w.keeperID} {
		if x, y, z := srv.PlayerPosition(t, id); !field.ContainsPoint(x, y, z) {
			t.Fatalf("player %d at %d,%d,%d is off the battlefield", id, x, y, z)
		}
	}
	return w
}

// folk is the Talker's side of the world, for the civilian NPC helpers.
func (w *besiegedWorld) folk() *folkWorld {
	return &folkWorld{srv: w.srv, c: w.talker, player: w.talkerID, at: w.at}
}

// startSiege starts Gludio's siege.
func (w *besiegedWorld) startSiege(t *testing.T) {
	t.Helper()
	w.gludio.Start()
	if !w.gludio.InProgress() {
		t.Fatal("Gludio's siege did not start")
	}
}

// character is the online character id.
func (w *besiegedWorld) character(t *testing.T, id int32) *player.Character {
	t.Helper()
	obj, ok := w.srv.State.Player(id)
	if !ok {
		t.Fatalf("player %d not in the world", id)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %d is %T, not an online character", id, obj)
	}
	return c
}

// sorcererTemplate is Gludio's Court Sorcerer as a fixture siege guard.
func sorcererTemplate() *npc.Template {
	return &npc.Template{
		ID: gludioSorcererID, TemplateID: gludioSorcererID, Type: "SiegeGuard", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
}

// TestSiegeGuardAttackableOnlyByAttackersDuringItsSiege pins a siege
// guard's attackability (SiegeGuard.isAttackableBy, SiegeGuard.java:22-44):
// only during its castle's siege, and only by a player whose clan attacks
// it, so NpcInfo shows the attacker the attack cursor and the defender
// none, and a still guard's ServerObjectInfo shows its HP to the attacker
// alone; outside the siege nobody may attack it. During the siege it may
// attack an attacker, never a player whose clan owns or defends the siege
// where it stands (Playable.isAttackableBy, Playable.java:458-469).
func TestSiegeGuardAttackableOnlyByAttackersDuringItsSiege(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		started   bool
		attacker  int32 // the NpcInfo flag the attacker sees
		attacks   bool  // the guard may attack the attacker
		mayDefend bool  // the guard may attack the defender
	}{
		{name: "before its siege", attacks: true, mayDefend: true},
		{name: "during its siege", started: true, attacker: 1, attacks: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootBesiegedWorld(t, tt.started, 0)
			drainUntilQuiet(t, w.keeper)
			guard := w.srv.SpawnHostileNPCTemplateAt(t, sorcererTemplate(), location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z})
			if got := npcInfoAttackable(t, drainFrames(t, w.talker), guard.ObjectID()); got != tt.attacker {
				t.Fatalf("guard NpcInfo attackable for the attacker = %d, want %d", got, tt.attacker)
			}
			if got := npcInfoAttackable(t, drainFrames(t, w.keeper), guard.ObjectID()); got != 0 {
				t.Fatalf("guard NpcInfo attackable for the defender = %d, want 0", got)
			}
			still := sorcererTemplate()
			still.RunSpeed, still.WalkSpeed = 0, 0
			statue := w.srv.SpawnHostileNPCTemplateAt(t, still, location.Location{X: 1_500, Y: w.at.Y, Z: w.at.Z})
			shown := serverObjectInfoView{}
			if tt.attacker == 1 {
				shown = serverObjectInfoView{attackable: 1, hp: int32(statue.CurrentHP()), maxHP: int32(statue.MaxHPValue())}
			}
			if got := lastServerObjectInfo(t, drainFrames(t, w.talker), statue.ObjectID()); got != shown {
				t.Fatalf("still guard ServerObjectInfo for the attacker = %+v, want %+v", got, shown)
			}
			if got := lastServerObjectInfo(t, drainFrames(t, w.keeper), statue.ObjectID()); got != (serverObjectInfoView{}) {
				t.Fatalf("still guard ServerObjectInfo for the defender = %+v, want no flag and no HP", got)
			}
			talker, keeper := w.character(t, w.talkerID), w.character(t, w.keeperID)
			if got := guard.AttackableBy(talker); got != (tt.attacker == 1) {
				t.Fatalf("guard attackable by the attacker = %v, want %v", got, tt.attacker == 1)
			}
			if guard.AttackableBy(keeper) {
				t.Fatal("guard attackable by the defender")
			}
			if got := talker.AttackableBy(guard); got != tt.attacks {
				t.Fatalf("guard may attack the attacker = %v, want %v", got, tt.attacks)
			}
			if got := keeper.AttackableBy(guard); got != tt.mayDefend {
				t.Fatalf("guard may attack the defender = %v, want %v", got, tt.mayDefend)
			}
		})
	}
}
