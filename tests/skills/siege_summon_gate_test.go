package skills

import (
	"context"
	"database/sql"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The siege-summon gate world: the caster leads gateClanID; gateRivalClanID,
// led by an offline character, attacks Gludio Castle when the caster's clan
// defends it.
const (
	gateClanID        int32 = 0x7f000201
	gateRivalClanID   int32 = 0x7f000202
	gateRivalLeaderID int32 = 0x7f000203
	gludioCastleID          = 1
)

// siegeGateSetup is where the caster stands and what it is when it asks
// for the siege-summon skill.
type siegeGateSetup struct {
	// defends registers the caster's clan to defend Gludio, a rival
	// attacking; otherwise the caster's clan attacks.
	defends bool
	// offBattlefield puts Gludio's battlefield away from the caster.
	offBattlefield bool
	// inCastle puts the caster in Gludio's castle grounds.
	inCastle bool
	// dusk signs the caster up for Dusk while Dawn holds the Seal of
	// Strife in seal validation.
	dusk bool
}

// gateBox is a zone volume over the caster's spawn when it holds it, else
// far east of it.
func gateBox(t *testing.T, holds bool) zone.Form {
	t.Helper()
	minX, maxX := -1_000, 1_000
	if !holds {
		minX, maxX = 50_000, 51_000
	}
	form, err := zone.NewCuboid(minX, maxX, -1_000, 1_000, -10_000, 10_000)
	if err != nil {
		t.Fatalf("zone form: %v", err)
	}
	return form
}

// bootSiegeGate boots the shipped castles with Gludio's siege under way and
// the caster in the world as setup describes, knowing the gate skills.
func bootSiegeGate(t *testing.T, setup siegeGateSetup) (*gameservertest.Server, int32) {
	t.Helper()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	zones := zone.NewIndex()
	field := zone.NewSiege(1, gateBox(t, !setup.offBattlefield))
	field.ResidenceID = gludioCastleID
	zones.Add(field)
	if setup.inCastle {
		grounds := zone.NewCastle(2, gateBox(t, true))
		grounds.ResidenceID = gludioCastleID
		zones.Add(grounds)
	}
	side, rivalSide := "ATTACKER", "DEFENDER"
	if setup.defends {
		side, rivalSide = "DEFENDER", "ATTACKER"
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Sieger", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, gateSkills())),
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithZones(zones),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, s := range []struct {
				q    string
				args []any
			}{
				{"UPDATE characters SET clanid = ? WHERE char_name = 'Sieger'", []any{gateClanID}},
				{"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) SELECT ?, 'Siegers', 5, obj_Id FROM characters WHERE char_name = 'Sieger'", []any{gateClanID}},
				{"INSERT INTO characters (account_name, obj_Id, char_name, level, clanid) VALUES ('rival', ?, 'Red', 40, ?)", []any{gateRivalLeaderID, gateRivalClanID}},
				{"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (?, 'Rivals', 5, ?)", []any{gateRivalClanID, gateRivalLeaderID}},
				{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, ?)", []any{gludioCastleID, gateClanID, side}},
				{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, ?)", []any{gludioCastleID, gateRivalClanID, rivalSide}},
			} {
				if _, err := db.ExecContext(context.Background(), s.q, s.args...); err != nil {
					t.Fatalf("seed siege clans: %v", err)
				}
			}
		}),
		gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) {
			if !setup.dusk {
				return
			}
			ctx := context.Background()
			row, found, err := store.LoadStatus(ctx)
			if err != nil || !found {
				t.Fatalf("load seven signs status: found=%v err=%v", found, err)
			}
			row.Period = sevensigns.SealValidation
			row.SealOwners[2] = sevensigns.Dawn
			if err := store.SaveStatus(ctx, row); err != nil {
				t.Fatalf("seed seven signs status: %v", err)
			}
		}),
		// No period change fires during the test.
		gameservertest.WithSevenSignsTimer(func(time.Duration, func()) *time.Timer { return nil }),
	)
	objID := srv.SoleObjectID(t)
	if setup.dusk {
		if err := srv.SevenSigns.SetPlayerInfo(context.Background(), objID, sevensigns.Dusk, sevensigns.Strife); err != nil {
			t.Fatalf("sign up for dusk: %v", err)
		}
	}
	for _, def := range gateSkills() {
		seedKnownSkill(t, srv, objID, int(def.ID), 1)
	}
	gludio, ok := srv.Sieges.Get(gludioCastleID)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	// The siege starts before the caster comes in, so its start does not
	// banish the caster off the battlefield.
	gludio.Start()
	if !gludio.InProgress() {
		t.Fatal("Gludio siege not under way")
	}
	enterBesieged(t, srv.Client)
	return srv, objID
}

// enterBesieged selects slot 0 and enters the world, consuming the reply
// burst up to its closing ActionFailed, whatever the siege adds to it, and
// every trailing frame.
func enterBesieged(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(encodeEnterWorld())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("EnterWorld burst has no ActionFailed within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeActionFailed {
			break
		}
	}
	drainUntilQuiet(t, c)
}

// TestSiegeSummonSkillGate drives RequestMagicSkillUse for a siege-summon
// skill against a running siege (PlayerCast.canAttemptCast): the caster's
// clan must attack the siege under way where the caster stands, else
// S1_CANNOT_BE_USED; an attacker inside the castle grounds is told
// NOT_CALL_PET_FROM_THIS_LOCATION (604), which answers before the Seal of
// Strife; a Dusk member while Dawn holds the Seal of Strife in seal
// validation is told SEAL_OF_STRIFE_FORBIDS_SUMMONING (1925). Each refusal
// is its message then ActionFailed, and the cast never starts; an attacker
// on the battlefield casts.
func TestSiegeSummonSkillGate(t *testing.T) {
	t.Parallel()
	cannotBeUsed := func(t *testing.T, f []byte) {
		assertSystemMessageSkillFrame(t, f, serverpackets.SystemMessageS1CannotBeUsed, gateSiegeSkillID, 1)
	}
	for _, tt := range []struct {
		name   string
		setup  siegeGateSetup
		refuse func(*testing.T, []byte)
	}{
		{name: "attacker on the battlefield"},
		{name: "defender on the battlefield", setup: siegeGateSetup{defends: true}, refuse: cannotBeUsed},
		{name: "attacker off the battlefield", setup: siegeGateSetup{offBattlefield: true}, refuse: cannotBeUsed},
		{
			name: "attacker in the castle grounds", setup: siegeGateSetup{inCastle: true},
			refuse: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageNotCallPetFromThisLocation)
			},
		},
		{
			name: "dusk attacker under a dawn seal of strife", setup: siegeGateSetup{dusk: true},
			refuse: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageSealOfStrifeForbidsSummoning)
			},
		},
		{
			name: "castle grounds answer before the seal", setup: siegeGateSetup{inCastle: true, dusk: true},
			refuse: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageNotCallPetFromThisLocation)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, objID := bootSiegeGate(t, tt.setup)
			c := srv.Client
			mpBefore := srv.PlayerCurrentMP(t, objID)
			c.Send(encodeRequestMagicSkillUse(gateSiegeSkillID, false, false))
			if tt.refuse == nil {
				assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "siege-summon cast")
				if !srv.PlayerCastingNow(t, objID) {
					t.Fatal("siege-summon cast not in flight")
				}
				return
			}
			tt.refuse(t, c.Read())
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "gate ActionFailed")
			if queued := c.ReadQueued(); len(queued) != 0 {
				t.Fatalf("gate rejection extra opcode = %#x, want none", queued[0][0])
			}
			if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
				t.Fatalf("MP after rejected cast = %d, want unchanged %d", got, mpBefore)
			}
			if srv.PlayerCastingNow(t, objID) {
				t.Fatal("rejected cast is in flight")
			}
		})
	}
}
