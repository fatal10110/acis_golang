package skills

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// siegeSideScene is the Caster, leading Knights, and the Stranger on
// Gludio's battlefield while its siege runs.
type siegeSideScene struct {
	srv                  *gameservertest.Server
	caster, stranger     *testsupport.ScriptedClient
	casterID, strangerID int32
	// held maps each item template the Caster carries in to its object id.
	held map[int32]int32
}

// bootSiegeSideScene boots the shipped castles with Gludio's battlefield
// over the shared spawn, Knights registered on knightsSide and the
// Stranger's Rivals on rivalsSide ("" leaves the Stranger without a clan),
// starts Gludio's siege, then brings the Caster, knowing def, and the
// Stranger in. more are further skills the server defines, held the items
// (template id to count) the Caster carries in, extra further boot options.
func bootSiegeSideScene(t *testing.T, def modelskill.Definition, knightsSide, rivalsSide string, more []modelskill.Definition, held map[int32]int32, extra ...gameservertest.Option) *siegeSideScene {
	t.Helper()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	zones := zone.NewIndex()
	field := zone.NewSiege(1, gateBox(t, true))
	field.ResidenceID = gludioCastleID
	zones.Add(field)
	knights, rivals := strconv.Itoa(int(socialKnightsID)), strconv.Itoa(int(socialRivalsID))
	stmts := []string{
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
			SELECT ` + knights + `, 'Knights', 5, obj_Id FROM characters WHERE char_name = 'Caster'`,
		`UPDATE characters SET clanid = ` + knights + ` WHERE char_name = 'Caster'`,
		`INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, ` + knights + `, '` + knightsSide + `')`,
	}
	if rivalsSide != "" {
		stmts = append(stmts,
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
				VALUES (`+rivals+`, 'Rivals', 5, `+strconv.Itoa(int(socialStrangerID))+`)`,
			`UPDATE characters SET clanid = `+rivals+` WHERE char_name = 'Stranger'`,
			`INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, `+rivals+`, '`+rivalsSide+`')`,
		)
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Caster", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, append([]modelskill.Definition{def}, more...))),
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithZones(zones),
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
	s := &siegeSideScene{srv: srv, caster: srv.Client, casterID: srv.SoleObjectID(t), strangerID: socialStrangerID}
	seedKnownSkill(t, srv, s.casterID, int(def.ID), int(def.Level))
	s.held = map[int32]int32{}
	for id, n := range held {
		s.held[id] = srv.GiveItem(t, s.casterID, id, n)
	}
	gludio, ok := srv.Sieges.Get(gludioCastleID)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	// The siege starts before anyone comes in, so its start banishes
	// nobody off the battlefield.
	gludio.Start()
	if !gludio.InProgress() {
		t.Fatal("Gludio siege not under way")
	}
	enterBesieged(t, s.caster)
	s.stranger = srv.DialClient(t, "player2", 1)
	enterBesieged(t, s.stranger)
	drainUntilQuiet(t, s.caster)
	for _, id := range []int32{s.casterID, s.strangerID} {
		x, y, z := srv.PlayerPosition(t, id)
		if !field.ContainsPoint(x, y, z) {
			t.Fatalf("player %d at %d,%d,%d is off the battlefield", id, x, y, z)
		}
	}
	x, y, z := srv.PlayerPosition(t, s.strangerID)
	s.caster.Send(encodeAction(s.strangerID, int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, s.caster)
	return s
}

// lastAutoAttackable reads the auto-attackable flag of the last
// RelationChanged in frames describing objectID.
func lastAutoAttackable(t *testing.T, frames [][]byte, objectID int32) int32 {
	t.Helper()
	flag, found := int32(0), false
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeRelationChanged {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != objectID {
			continue
		}
		r.ReadInt32()
		flag, found = r.ReadInt32(), true
	}
	if !found {
		t.Fatalf("no RelationChanged of %d in %x", objectID, opcodes(frames))
	}
	return flag
}

// TestSiegeSameSideCasts has the Caster face the Stranger, both on
// Gludio's battlefield while its siege runs. Two players whose clans are
// not on opposite sides of that siege (Playable.isInSameActiveSiegeSide,
// Playable.java:748-761) are treated as clan mates, their PvP flags aside:
// neither may be attacked without force, so once the Stranger is flagged
// the Caster still sees its auto-attackable flag off (Playable.java:521-
// 522), and an offensive cast needs CTRL and must be a damage skill on the
// main target, though the battlefield is a PvP zone (Playable.java:
// 410-414). A player without a clan is on the opposite side of nobody.
// Players on opposite sides fight freely there.
func TestSiegeSameSideCasts(t *testing.T) {
	t.Parallel()
	damage := targetedSkill(socialSkillID, modelskill.TargetOne, true)
	damage.SkillType = "MDAM"
	for _, tt := range []struct {
		name                string
		knightsSide, rivals string
		sameSide            bool
	}{
		{name: "both attack", knightsSide: "ATTACKER", rivals: "ATTACKER", sameSide: true},
		{name: "stranger without a clan", knightsSide: "ATTACKER", sameSide: true},
		{name: "opposite sides", knightsSide: "ATTACKER", rivals: "DEFENDER"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := bootSiegeSideScene(t, damage, tt.knightsSide, tt.rivals, nil, nil)
			obj, ok := s.srv.State.Player(s.strangerID)
			if !ok {
				t.Fatal("Stranger missing from the world")
			}
			obj.(interface{ UpdatePvPFlag(task.PvPFlagState) }).UpdatePvPFlag(task.PvPFlagOn)
			want := int32(1)
			if tt.sameSide {
				want = 0
			}
			if got := lastAutoAttackable(t, collectUntilQuiet(t, s.caster), s.strangerID); got != want {
				t.Fatalf("flagged Stranger's auto-attackable flag = %d, want %d", got, want)
			}
			drainUntilQuiet(t, s.stranger)
			s.cast(t, damage, false, !tt.sameSide)
			if tt.sameSide {
				s.cast(t, damage, true, true)
			}
		})
	}
}

// cast has the Caster cast def at the Stranger, with or without CTRL:
// started reports whether the cast starts, otherwise it must be refused as
// an invalid target.
func (s *siegeSideScene) cast(t *testing.T, def modelskill.Definition, ctrl, started bool) {
	t.Helper()
	s.caster.Send(encodeRequestMagicSkillUse(int32(def.ID), ctrl, false))
	if started {
		readCastStartFrames(t, s.caster, s.casterID, int32(def.ID), 1, 500, 60_000, s.strangerID)
		return
	}
	assertStaticSystemMessage(t, s.caster.Read(), serverpackets.SystemMessageInvalidTarget)
	assertNoActionFailedUntilQuiet(t, s.caster, "refused siege-side cast")
}
