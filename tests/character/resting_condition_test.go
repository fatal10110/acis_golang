package character

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

const vitalForceID = 148

var shippedVitalForce struct {
	once sync.Once
	def  modelskill.Definition
	err  error
}

// vitalForceSkills is a skill table holding the shipped Vital Force (148)
// level 1, whose regHp and regMp adds carry <player resting="true"/>.
func vitalForceSkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	dir := datapack.Path(t, "data", "xml", "skills")
	shippedVitalForce.once.Do(func() {
		table, err := xmldata.LoadSkillDefinitions(dir, zerolog.Nop())
		if err != nil {
			shippedVitalForce.err = err
			return
		}
		def, ok := table.Definition(modelskill.Ref{ID: vitalForceID, Level: 1})
		if !ok {
			shippedVitalForce.err = errors.New("level 1 missing")
			return
		}
		shippedVitalForce.def = *def
	})
	if shippedVitalForce.err != nil {
		t.Fatalf("load shipped Vital Force: %v", shippedVitalForce.err)
	}
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{shippedVitalForce.def}), gamesql.NewCharacterSkillStore(db))
}

// TestRestingConditionWaitsForTheSitDownToEnd pins ConditionPlayerState
// RESTING (ConditionPlayerState.java:25-26), which reads Player.isSitting:
// sitDown sets it only when the 2.5 s transition ends (Player.java:1542-1551).
// Vital Force's resting-only regeneration bonus therefore stays off while
// the player is still sitting down and applies once it is seated.
func TestRestingConditionWaitsForTheSitDownToEnd(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		regenTermsTemplate(),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(vitalForceSkills(t)),
	)
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, vitalForceID, 1); err != nil {
		t.Fatalf("grant Vital Force: %v", err)
	}
	client := srv.Client
	enterWorld(t, client)
	drainQuiet(t, client)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player not in the world")
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%T is not an online character", obj)
	}
	if !c.HasSkill(vitalForceID) {
		t.Fatal("Vital Force not known")
	}
	still := readRegenRates(c)

	client.Send(encodeRequestChangeWaitType(false))
	mustReadOpcode(t, client, serverpackets.OpcodeChangeWaitType, "sit")
	if got := readRegenRates(c); !got.near(still) {
		t.Fatalf("regen while sitting down = %+v, want standing %+v without the resting bonus", got, still)
	}

	srv.SettlePosture(t, objID)
	// Seated, the posture multiplier goes from 1.1 to 1.5 and Vital Force
	// adds 1.9 HP and 0.9 MP before it.
	seated := readRegenRates(c)
	noBonus := still.scaled(1.5 / 1.1)
	if seated.hp-noBonus.hp < 1.9 || seated.mp-noBonus.mp < 0.9 {
		t.Fatalf("seated regen = %+v, want Vital Force's bonus on top of %+v", seated, noBonus)
	}
}
