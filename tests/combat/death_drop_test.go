package combat

import (
	"context"
	"slices"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Items of the shared catalog a death may or may not drop: a weapon and a
// chest armor to wear, a potion, a scroll that cannot be dropped at all, a
// quest item and adena.
const (
	dropSwordID  int32 = 30
	dropTunicID  int32 = 40
	dropPotionID int32 = 20
	dropScrollID int32 = 736
	dropQuestID  int32 = 9001
)

// everyDeathDrop drops every droppable item of any death that rolls at all.
var everyDeathDrop = player.DeathDropRates{Chance: 100, Equip: 100, EquipWeapon: 100, Item: 100, Limit: 10}

// seedDropVictim creates the account's single selectable character at
// level with karma and pkKills, so a death's drop branch is chosen by the
// stored state.
func seedDropVictim(level, karma, pkKills int) func(*gamesql.CharacterStore, *gamesql.ItemStore) {
	return func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		ch, err := player.NewCharacter(4242, gameservertest.ClassTemplate(), "player1", "Victim", 1, 0, 0, player.SexMale)
		if err != nil {
			panic(err)
		}
		ch.CharLevel = level
		ch.KarmaPoints = karma
		ch.PKKills = pkKills
		ctx := context.Background()
		if err := chars.Create(ctx, ch); err != nil {
			panic(err)
		}
		if err := chars.Save(ctx, ch.SaveState()); err != nil {
			panic(err)
		}
	}
}

// dropVictim is the victim's items: the object ids of the worn sword and
// tunic and of the potion.
type dropVictim struct {
	id                   int32
	sword, tunic, potion int32
}

// stockDropVictim gives the victim id one of each catalog item before it
// enters the world, then has it put on the sword and the tunic.
func stockDropVictim(t *testing.T, srv *gameservertest.Server, c *scriptedClient, id int32) dropVictim {
	t.Helper()
	v := dropVictim{id: id}
	v.sword = srv.GiveItem(t, id, dropSwordID, 1)
	v.tunic = srv.GiveItem(t, id, dropTunicID, 1)
	v.potion = srv.GiveItem(t, id, dropPotionID, 5)
	srv.GiveItem(t, id, dropScrollID, 1)
	srv.GiveItem(t, id, dropQuestID, 1)
	srv.GiveItem(t, id, item.AdenaID, 1000)
	startInWorld(t, c)
	c.Send(encodeUseItem(v.sword, false))
	c.Send(encodeUseItem(v.tunic, false))
	drainUntilQuiet(t, c)
	inv := victimCharacter(t, srv, id).Inventory()
	for _, objectID := range []int32{v.sword, v.tunic} {
		if inst := inv.ItemByObjectID(objectID); inst == nil || !inst.Equipped() {
			t.Fatalf("item %d not worn before the death", objectID)
		}
	}
	return v
}

func victimCharacter(t *testing.T, srv *gameservertest.Server, id int32) *player.Character {
	t.Helper()
	obj, ok := srv.State.Player(id)
	if !ok {
		t.Fatalf("player %d not in world", id)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	return c
}

// setVictimRoll makes every roll of the victim's random source come out
// as roll.
func setVictimRoll(t *testing.T, srv *gameservertest.Server, id int32, roll int) {
	t.Helper()
	c := victimCharacter(t, srv, id)
	done := make(chan struct{})
	if !srv.PlayerQueue(t, id).Post(func() { c.SetRollSource(func(int) int { return roll }); close(done) }) {
		t.Fatal("post: queue closed")
	}
	<-done
}

// killByNPC has a monster kill the victim id on the victim's queue.
func killByNPC(t *testing.T, srv *gameservertest.Server, id int32) {
	t.Helper()
	var killer attackable.Combatant = srv.SpawnHostileNPC(t)
	victim := victimCharacter(t, srv, id)
	done := make(chan struct{})
	if !srv.PlayerQueue(t, id).Post(func() { victim.Kill(killer); close(done) }) {
		t.Fatal("post: queue closed")
	}
	<-done
	srv.Settle(t)
}

// droppedNames returns the items YOU_DROPPED_S1 names among frames, in
// order.
func droppedNames(frames [][]byte) []int32 {
	var out []int32
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(f[1:])
		if r.ReadInt32() != serverpackets.SystemMessageYouDroppedS1 || r.ReadInt32() != 1 || r.ReadInt32() != serverpackets.SystemMessageParamItemName {
			continue
		}
		out = append(out, r.ReadInt32())
	}
	return out
}

// groundTemplates returns the template ids of the items on the ground,
// each checked to lie within the drop offset of (x, y).
func groundTemplates(t *testing.T, srv *gameservertest.Server, x, y int) []int32 {
	t.Helper()
	var out []int32
	for _, g := range srv.GroundItems.Snapshots(nil) {
		if dx, dy := g.X-x, g.Y-y; dx < -25 || dx > 25 || dy < -25 || dy > 25 {
			t.Fatalf("item %d dropped at (%d, %d), more than 25 off the victim at (%d, %d)", g.TemplateID, g.X, g.Y, x, y)
		}
		out = append(out, g.TemplateID)
	}
	slices.Sort(out)
	return out
}

// sorted returns ids sorted.
func sorted(ids ...int32) []int32 {
	out := slices.Clone(ids)
	slices.Sort(out)
	return out
}

// TestPKDeathDropsAtKarmaRates has a player killer with 5 PK kills die to
// another player's cast, every roll of its random source coming out 50: the
// death rolls (50 < 70), each worn item comes off, the sword then stays
// (50 >= 10) while the tunic (50 < 60) and the potion (50 < 60) drop. The
// scroll that cannot be dropped, the quest item and adena stay whatever the
// roll. Each drop names its item to the victim and lands within 25 of it,
// and the inventory left after logout no longer holds them.
func TestPKDeathDropsAtKarmaRates(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(seedDropVictim(5, 240, 5)),
		gameservertest.WithSkills(combatPersistence(t, killSkillDefs())),
		gameservertest.WithDeathDrop(player.DeathDropRules{
			Karma:        player.DeathDropRates{Chance: 70, Equip: 60, EquipWeapon: 10, Item: 60, Limit: 10},
			Monster:      everyDeathDrop,
			KarmaPKLimit: 5,
		}),
	)
	c := srv.Client
	victimID := srv.SoleObjectID(t)
	v := stockDropVictim(t, srv, c, victimID)
	setVictimRoll(t, srv, victimID, 50)

	killerChar := srv.SeedCharacterFor(t, "killer", "Killer", 5, 0)
	seedKnownSkill(t, srv, killerChar.ID, 42, 1)
	killer := srv.DialClient(t, "killer", 1)
	startInWorld(t, killer)
	drainUntilQuiet(t, killer)
	drainUntilQuiet(t, c)
	x, y, _ := srv.PlayerPosition(t, victimID)

	killPrimaryClient(t, srv, killer, killerChar.ID, victimID)

	if got, want := sorted(droppedNames(readQuiet(c))...), sorted(dropTunicID, dropPotionID); !slices.Equal(got, want) {
		t.Fatalf("YOU_DROPPED_S1 named %v, want %v", got, want)
	}
	if got, want := groundTemplates(t, srv, x, y), sorted(dropTunicID, dropPotionID); !slices.Equal(got, want) {
		t.Fatalf("ground items = %v, want %v", got, want)
	}
	inv := victimCharacter(t, srv, victimID).Inventory()
	if sword := inv.ItemByObjectID(v.sword); sword == nil || sword.Equipped() {
		t.Fatalf("sword after the death = %+v, want it kept and taken off", sword)
	}

	logoutPersisted(t, srv, c)
	srv.FlushItems(t)
	rows, err := srv.Items.ListByOwner(context.Background(), victimID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	var kept []int32
	for _, inst := range rows {
		kept = append(kept, inst.TemplateID)
		if inst.ObjectID == v.sword && inst.Location != item.LocationInventory {
			t.Fatalf("stored sword location = %v, want the inventory", inst.Location)
		}
	}
	if got, want := sorted(kept...), sorted(item.AdenaID, dropSwordID, dropScrollID, dropQuestID); !slices.Equal(got, want) {
		t.Fatalf("stored inventory = %v, want %v", got, want)
	}
}

// TestDeathDropStopsAtLimit has a PK die with every item set to drop but a
// limit of one: exactly one item drops.
func TestDeathDropStopsAtLimit(t *testing.T) {
	t.Parallel()
	limited := everyDeathDrop
	limited.Limit = 1
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(seedDropVictim(5, 240, 5)),
		gameservertest.WithDeathDrop(player.DeathDropRules{Karma: limited, KarmaPKLimit: 5}),
	)
	c := srv.Client
	victimID := srv.SoleObjectID(t)
	stockDropVictim(t, srv, c, victimID)

	killByNPC(t, srv, victimID)

	if got := droppedNames(readQuiet(c)); len(got) != 1 {
		t.Fatalf("YOU_DROPPED_S1 named %v, want exactly one item", got)
	}
	if n := srv.GroundItems.Len(); n != 1 {
		t.Fatalf("ground items = %d, want 1", n)
	}
}

// TestDeathDropBranches pins which deaths drop at all, every rate set to
// drop everything: a karma-free player killed by another player drops
// nothing, nor does a PK short of the PK threshold killed by one; a monster
// kill drops from level 5 up, not below; a game master drops nothing unless
// the server lets it.
func TestDeathDropBranches(t *testing.T) {
	t.Parallel()
	rules := player.DeathDropRules{Karma: everyDeathDrop, Monster: everyDeathDrop, KarmaPKLimit: 5}
	gmRules := rules
	gmRules.GMDrops = true
	all := sorted(dropSwordID, dropTunicID, dropPotionID)
	for _, tt := range []struct {
		name          string
		level, karma  int
		pkKills       int
		byPlayer, gm  bool
		rules         player.DeathDropRules
		wantGround    []int32
		wantSwordWorn bool
	}{
		{name: "karma-free player kill", level: 5, byPlayer: true, rules: rules, wantSwordWorn: true},
		{name: "PK under the threshold, player kill", level: 5, karma: 240, pkKills: 4, byPlayer: true, rules: rules, wantSwordWorn: true},
		{name: "PK at the threshold, player kill", level: 5, karma: 240, pkKills: 5, byPlayer: true, rules: rules, wantGround: all},
		{name: "monster kill at level 4", level: 4, rules: rules, wantSwordWorn: true},
		{name: "monster kill at level 5", level: 5, rules: rules, wantGround: all},
		{name: "game master", level: 5, gm: true, rules: rules, wantSwordWorn: true},
		{name: "game master allowed to drop", level: 5, gm: true, rules: gmRules, wantGround: all},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := []gameservertest.Option{
				gameservertest.WithWantChars(1),
				gameservertest.WithSeed(seedDropVictim(tt.level, tt.karma, tt.pkKills)),
				gameservertest.WithSkills(combatPersistence(t, killSkillDefs())),
				gameservertest.WithDeathDrop(tt.rules),
			}
			if tt.gm {
				adminData, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
				if err != nil {
					t.Fatalf("load admin data: %v", err)
				}
				opts = append(opts, gameservertest.WithAdmin(adminData))
			}
			srv := gameservertest.Boot(t, opts...)
			c := srv.Client
			victimID := srv.SoleObjectID(t)
			if tt.gm {
				if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE obj_Id = ?", victimID); err != nil {
					t.Fatalf("set access level: %v", err)
				}
			}
			v := stockDropVictim(t, srv, c, victimID)
			x, y, _ := srv.PlayerPosition(t, victimID)

			if tt.byPlayer {
				killerChar := srv.SeedCharacterFor(t, "killer", "Killer", 5, 0)
				seedKnownSkill(t, srv, killerChar.ID, 42, 1)
				killer := srv.DialClient(t, "killer", 1)
				startInWorld(t, killer)
				drainUntilQuiet(t, killer)
				drainUntilQuiet(t, c)
				killPrimaryClient(t, srv, killer, killerChar.ID, victimID)
			} else {
				killByNPC(t, srv, victimID)
			}

			if got := sorted(droppedNames(readQuiet(c))...); !slices.Equal(got, tt.wantGround) {
				t.Fatalf("YOU_DROPPED_S1 named %v, want %v", got, tt.wantGround)
			}
			if got := groundTemplates(t, srv, x, y); !slices.Equal(got, tt.wantGround) {
				t.Fatalf("ground items = %v, want %v", got, tt.wantGround)
			}
			if tt.wantSwordWorn {
				if sword := victimCharacter(t, srv, victimID).Inventory().ItemByObjectID(v.sword); sword == nil || !sword.Equipped() {
					t.Fatal("sword came off a death that drops nothing")
				}
			}
		})
	}
}
