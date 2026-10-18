package skills

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: PlayerCast.canCast, case RESURRECT: a caster standing in the
// zone of a castle siege in progress (CastleManager.getActiveSiege) is
// refused CANNOT_BE_RESURRECTED_DURING_SIEGE (1053) without a clan; by the
// side its clan holds (Siege.getSide), TOWER_DESTROYED_NO_RESURRECTION
// (717) as the owner or a defender once no life control tower of the
// castle stands (Castle.getAliveLifeTowerCount), NO_RESURRECTION_WITHOUT_
// BASE_CAMP (716) as an attacker whose clan holds no headquarters
// (Clan.getFlag), and 1053 on any other side (none, or a pending
// defender). A refusal is the message alone (PlayerAI.thinkCast returns).
// The cast is unaffected off a siege in progress. Scrolls cast through the
// same check.
//
// PlayerStatus.getRegenHp: a member of a clan attacking the siege in
// progress where it stands regenerates HP 1.5 times as fast within 200
// (3D) of its clan's headquarters.

const (
	siegeResSkillID      int32 = 9321
	siegeResKnightsLead  int32 = 0x7f000501
	siegeResRivalsLead   int32 = 0x7f000502
	siegeResTowerHP            = 1000
	siegeResHQRegenRange       = 200
)

// siegeResSkill is a resurrection of a dead player.
func siegeResSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(siegeResSkillID), Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetCorpsePlayer, SkillType: "RESURRECT", Power: 100,
		CastRange: 400, HitTime: 500, StaticHitTime: true,
	}
}

// siegeResSetup is the Caster's standing when it casts.
type siegeResSetup struct {
	// side is the siege_clans side of Knights, the Caster's clan, or
	// "OWNER" for the castle's owner and "" for no side.
	side string
	// noClan leaves the Caster out of Knights.
	noClan bool
	// offBattlefield puts Gludio's battlefield away from the players.
	offBattlefield bool
	// noSiege leaves Gludio's siege unfought.
	noSiege bool
}

// siegeResScene is the Caster and the Stranger, a Knights member, by
// Gludio's single life control tower, Gludio's siege under way as set up.
type siegeResScene struct {
	srv                  *gameservertest.Server
	caster, stranger     *testsupport.ScriptedClient
	casterID, strangerID int32
	gludio               *siege.Siege
	tower                *npc.Folk
	held                 map[int32]int32
}

// siegeResCastle is Gludio with one life control tower east of the shared
// spawn.
func siegeResCastle(t *testing.T) *castledata.Table {
	t.Helper()
	tower, err := castledata.NewControlTower("life1", "LIFE_CONTROL", 3000, 0, 0, siegeResTowerHP, 100, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	c, err := castledata.NewCastle(castledata.CastleAttrs{ID: gludioCastleID, Alias: "gludio_castle", Name: "Gludio"}, nil,
		[]castledata.ControlTower{tower}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err := castledata.NewTable([]*castledata.Castle{c})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// siegeResTowerTemplates are the working forms of the control towers.
func siegeResTowerTemplates() *npc.Table {
	working := func(id int, kind, name string) *npc.Template {
		return &npc.Template{
			ID: id, TemplateID: id, Type: kind, Name: name, Level: 20, HPMax: 342, MPMax: 213,
			WalkSpeed: 49.5, CollisionRadius: 13, CollisionHeight: 54, CanBeAttacked: true, CanMove: true,
		}
	}
	return npc.NewTable([]*npc.Template{
		working(npc.LifeTowerWorkingNpcID, "LifeTower", "Life Control Tower"),
		working(npc.FlameTowerWorkingNpcID, "FlameTower", "Flame Control Tower"),
	})
}

func bootSiegeResScene(t *testing.T, setup siegeResSetup, held map[int32]int32, extra ...gameservertest.Option) *siegeResScene {
	t.Helper()
	zones := zone.NewIndex()
	field := zone.NewSiege(1, gateBox(t, !setup.offBattlefield))
	field.ResidenceID = gludioCastleID
	zones.Add(field)
	knights, rivals := strconv.Itoa(int(socialKnightsID)), strconv.Itoa(int(socialRivalsID))
	stmts := []string{
		`INSERT INTO characters (account_name, obj_Id, char_name, level, clanid) VALUES ('kl', ` + strconv.Itoa(int(siegeResKnightsLead)) + `, 'Lord', 40, ` + knights + `)`,
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (` + knights + `, 'Knights', 5, ` + strconv.Itoa(int(siegeResKnightsLead)) + `)`,
		`INSERT INTO characters (account_name, obj_Id, char_name, level, clanid) VALUES ('rl', ` + strconv.Itoa(int(siegeResRivalsLead)) + `, 'Red', 40, ` + rivals + `)`,
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (` + rivals + `, 'Rivals', 5, ` + strconv.Itoa(int(siegeResRivalsLead)) + `)`,
		`UPDATE characters SET clanid = ` + knights + ` WHERE char_name = 'Stranger'`,
	}
	if !setup.noClan {
		stmts = append(stmts, `UPDATE characters SET clanid = `+knights+` WHERE char_name = 'Caster'`)
	}
	// Rivals take the other side, attacking unless Knights do, so the
	// siege has an attacker to start with.
	rivalsSide := "ATTACKER"
	switch setup.side {
	case "OWNER":
		stmts = append(stmts, `UPDATE clan_data SET hasCastle = 1 WHERE clan_id = `+knights)
	case "DEFENDER", "PENDING":
		stmts = append(stmts, `INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, `+knights+`, '`+setup.side+`')`)
	case "ATTACKER":
		stmts = append(stmts, `INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, `+knights+`, 'ATTACKER')`)
		rivalsSide = "DEFENDER"
	}
	stmts = append(stmts, `INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, `+rivals+`, '`+rivalsSide+`')`)
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Caster", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, append([]modelskill.Definition{siegeResSkill()}, resurrectionScrollSkill()))),
		gameservertest.WithCastles(siegeResCastle(t)),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithZones(zones),
		gameservertest.WithNPCs(siegeResTowerTemplates()),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithSeed(func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
			tmpl, ok := gameservertest.Templates(t).Get(0)
			if !ok {
				t.Fatal("missing test class template")
			}
			ch, err := player.NewCharacter(socialStrangerID, tmpl, "player2", "Stranger", 1, 0, 0, player.SexMale)
			if err != nil {
				t.Fatalf("seed Stranger: %v", err)
			}
			ch.CharLevel = 20
			if err := chars.Create(context.Background(), ch); err != nil {
				t.Fatalf("seed Stranger: %v", err)
			}
		}),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, stmt := range stmts {
				if _, err := db.ExecContext(context.Background(), stmt); err != nil {
					t.Fatalf("seed siege clans: %v", err)
				}
			}
		}),
	}, extra...)...)
	s := &siegeResScene{srv: srv, caster: srv.Client, casterID: srv.SoleObjectID(t), strangerID: socialStrangerID, held: map[int32]int32{}}
	seedKnownSkill(t, srv, s.casterID, int(siegeResSkillID), 1)
	for id, n := range held {
		s.held[id] = srv.GiveItem(t, s.casterID, id, n)
	}
	gludio, ok := srv.Sieges.Get(gludioCastleID)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	s.gludio = gludio
	towers := gludio.Castle().Towers()
	if len(towers) != 1 {
		t.Fatalf("towers placed = %d, want 1", len(towers))
	}
	s.tower, ok = towers[0].(*npc.Folk)
	if !ok {
		t.Fatalf("placed tower is %T", towers[0])
	}
	if !setup.noSiege {
		// The siege starts before anyone comes in, so its start banishes
		// nobody off the battlefield.
		gludio.Start()
		if !gludio.InProgress() {
			t.Fatal("Gludio siege not under way")
		}
	}
	enterBesieged(t, s.caster)
	s.stranger = srv.DialClient(t, "player2", 1)
	enterBesieged(t, s.stranger)
	drainUntilQuiet(t, s.caster)
	return s
}

// headquarters stands a headquarters of Knights dx east of the Caster.
func (s *siegeResScene) headquarters(t *testing.T, dx int) *npc.Folk {
	t.Helper()
	x, y, z := s.srv.PlayerPosition(t, s.casterID)
	flag := s.srv.SpawnFolkNPCAt(t, npc.SiegeFlagTemplate("Knights", false), location.Location{X: x + dx, Y: y, Z: z})
	if !hqClan(t, s.srv, socialKnightsID).SetHeadquarters(flag) {
		t.Fatal("Knights already hold a headquarters")
	}
	drainUntilQuiet(t, s.caster)
	return flag
}

// killStranger kills the Stranger by the Caster's hand and selects its
// corpse.
func (s *siegeResScene) killStranger(t *testing.T) {
	t.Helper()
	killer, ok := s.srv.State.Player(s.casterID)
	if !ok {
		t.Fatal("Caster missing from the world")
	}
	obj, ok := s.srv.State.Player(s.strangerID)
	if !ok {
		t.Fatal("Stranger missing from the world")
	}
	stranger, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("Stranger is %T", obj)
	}
	done := make(chan struct{})
	s.srv.PlayerQueue(t, s.strangerID).Post(func() {
		defer close(done)
		if !stranger.Kill(killer.(attackable.Combatant)) {
			t.Error("the Stranger did not die")
		}
	})
	<-done
	drainUntilQuiet(t, s.caster)
	drainUntilQuiet(t, s.stranger)
	x, y, z := s.srv.PlayerPosition(t, s.strangerID)
	s.caster.Send(encodeAction(s.strangerID, int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, s.caster)
}

// felled fells the castle's life control tower.
func (s *siegeResScene) felled(t *testing.T) {
	t.Helper()
	s.tower.TakeDamage(s.tower.MaxHP()*2, nil)
	drainUntilQuiet(t, s.caster)
	if got := s.gludio.AliveLifeTowers(); got != 0 {
		t.Fatalf("alive life towers after felling = %d, want 0", got)
	}
}

// TestSiegeResurrectCastGate casts a resurrection on the dead Stranger
// from each standing the siege rule reads: each refusal answers its message
// alone and casts nothing; every other standing casts.
func TestSiegeResurrectCastGate(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		setup  siegeResSetup
		hq     bool
		felled bool
		refuse int
	}{
		{name: "no clan", setup: siegeResSetup{noClan: true}, refuse: serverpackets.SystemMessageCannotBeResurrectedDuringSiege},
		{name: "clan on no side", refuse: serverpackets.SystemMessageCannotBeResurrectedDuringSiege},
		{name: "pending defender", setup: siegeResSetup{side: "PENDING"}, refuse: serverpackets.SystemMessageCannotBeResurrectedDuringSiege},
		{name: "defender with a life tower standing", setup: siegeResSetup{side: "DEFENDER"}},
		{name: "defender once no life tower stands", setup: siegeResSetup{side: "DEFENDER"}, felled: true, refuse: serverpackets.SystemMessageTowerDestroyedNoResurrection},
		{name: "owner with a life tower standing", setup: siegeResSetup{side: "OWNER"}},
		{name: "owner once no life tower stands", setup: siegeResSetup{side: "OWNER"}, felled: true, refuse: serverpackets.SystemMessageTowerDestroyedNoResurrection},
		{name: "attacker without headquarters", setup: siegeResSetup{side: "ATTACKER"}, refuse: serverpackets.SystemMessageNoResurrectionWithoutBaseCamp},
		{name: "attacker with headquarters", setup: siegeResSetup{side: "ATTACKER"}, hq: true},
		{name: "attacker without headquarters off the battlefield", setup: siegeResSetup{side: "ATTACKER", offBattlefield: true}},
		{name: "no clan off the battlefield", setup: siegeResSetup{noClan: true, offBattlefield: true}},
		{name: "attacker without headquarters before the siege", setup: siegeResSetup{side: "ATTACKER", noSiege: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := bootSiegeResScene(t, tt.setup, nil)
			if tt.hq {
				s.headquarters(t, 3000)
			}
			if tt.felled {
				s.felled(t)
			}
			s.killStranger(t)

			s.caster.Send(encodeRequestMagicSkillUse(siegeResSkillID, false, false))
			frames := collectUntilQuiet(t, s.caster)
			if tt.refuse == 0 {
				if !slices.Contains(opcodes(frames), serverpackets.OpcodeMagicSkillUse) {
					t.Fatalf("resurrection answered %x, want it cast", opcodes(frames))
				}
				return
			}
			if !slices.Equal(opcodes(frames), []byte{serverpackets.OpcodeSystemMessage}) {
				t.Fatalf("refused resurrection answered %x, want one SystemMessage", opcodes(frames))
			}
			assertStaticSystemMessage(t, frames[0], tt.refuse)
			if s.srv.PlayerCastingNow(t, s.casterID) {
				t.Fatal("refused resurrection is in flight")
			}
		})
	}
}

// TestSiegeResurrectScrollGate uses a Scroll of Resurrection on the dead
// Stranger, a fellow attacker: the scroll's skill is refused
// NO_RESURRECTION_WITHOUT_BASE_CAMP alone while Knights hold no
// headquarters, the scroll kept; once they hold one it is cast.
func TestSiegeResurrectScrollGate(t *testing.T) {
	t.Parallel()
	for _, hq := range []bool{false, true} {
		t.Run(map[bool]string{false: "without headquarters", true: "with headquarters"}[hq], func(t *testing.T) {
			t.Parallel()
			s := bootSiegeResScene(t, siegeResSetup{side: "ATTACKER"}, map[int32]int32{resurrectionScrollID: 1}, resurrectionScrollTemplates())
			if hq {
				s.headquarters(t, 3000)
			}
			s.killStranger(t)

			s.caster.Send(encodeUseItem(s.held[resurrectionScrollID]))
			frames := collectUntilQuiet(t, s.caster)
			if hq {
				if !slices.Contains(opcodes(frames), serverpackets.OpcodeMagicSkillUse) {
					t.Fatalf("scroll answered %x, want its skill cast", opcodes(frames))
				}
				return
			}
			if !slices.Equal(opcodes(frames), []byte{serverpackets.OpcodeSystemMessage}) {
				t.Fatalf("refused scroll answered %x, want one SystemMessage", opcodes(frames))
			}
			assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageNoResurrectionWithoutBaseCamp)
			if got := s.srv.PlayerItemCount(t, s.casterID, resurrectionScrollID); got != 1 {
				t.Fatalf("scrolls held after the refusal = %d, want 1", got)
			}
		})
	}
}

// TestSiegeHeadquartersHPRegen reads the Caster's HP regeneration with a
// Knights headquarters at each distance: 1.5 times the rate within 200 of
// it while Knights attack the siege under way where the Caster stands,
// unchanged otherwise.
func TestSiegeHeadquartersHPRegen(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		setup siegeResSetup
		dx    int
		mul   float64
	}{
		{name: "attacker by its headquarters", setup: siegeResSetup{side: "ATTACKER"}, dx: siegeResHQRegenRange - 1, mul: 1.5},
		{name: "attacker at the range edge", setup: siegeResSetup{side: "ATTACKER"}, dx: siegeResHQRegenRange, mul: 1},
		{name: "attacker away from its headquarters", setup: siegeResSetup{side: "ATTACKER"}, dx: siegeResHQRegenRange + 50, mul: 1},
		{name: "attacker by its headquarters off the battlefield", setup: siegeResSetup{side: "ATTACKER", offBattlefield: true}, dx: 50, mul: 1},
		{name: "attacker by its headquarters before the siege", setup: siegeResSetup{side: "ATTACKER", noSiege: true}, dx: 50, mul: 1},
		{name: "defender by its clan's headquarters", setup: siegeResSetup{side: "DEFENDER"}, dx: 50, mul: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := bootSiegeResScene(t, tt.setup, nil, siegeRegenClass())
			obj, ok := s.srv.State.Player(s.casterID)
			if !ok {
				t.Fatal("Caster missing from the world")
			}
			caster, ok := network.OnlineCharacter(obj)
			if !ok {
				t.Fatalf("Caster is %T", obj)
			}
			base := caster.HPRegenRate()
			if base <= 0 {
				t.Fatalf("base HP regeneration = %v, want positive", base)
			}
			s.headquarters(t, tt.dx)
			if got, want := caster.HPRegenRate(), base*tt.mul; got != want {
				t.Fatalf("HP regeneration with the headquarters %d away = %v, want %v (base %v)", tt.dx, got, want, base)
			}
		})
	}
}

// siegeRegenClass is the shared class with a flat HP regeneration table,
// which the shared class template leaves empty.
func siegeRegenClass() gameservertest.Option {
	tmpl := gameservertest.ClassTemplate()
	tmpl.HPRegenTable = make([]float64, 80)
	for i := range tmpl.HPRegenTable {
		tmpl.HPRegenTable[i] = 2
	}
	return gameservertest.WithClassTemplate(tmpl)
}
