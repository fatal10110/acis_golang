package clan

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: EffectClanGate.onStart (EffectClanGate.java:20-31) shows the
// magic circle and, on a player in a clan, sends every other online member
// COURT_MAGICIAN_CREATED_PORTAL (clan.broadcastToMembersExcept). aCis
// revision in the outer repo.

// clanGateID is the shipped Clan Gate skill (skills 3600-3699.xml).
const clanGateID = 3632

// clanGateSkill is the shipped Clan Gate with its ten-second cast and its
// hour of reuse cut to nothing, so a test casts it at once.
func clanGateSkill(t *testing.T) gameservertest.Option {
	t.Helper()
	defs, _ := shippedSkillData(t)
	def, ok := defs.Get(clanGateID, 1)
	if !ok {
		t.Fatal("no shipped Clan Gate")
	}
	def.HitTime, def.ReuseDelay = 0, 0
	db := sqltest.SharedDB(t)
	return gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{def}), gamesql.NewCharacterSkillStore(db)))
}

func encodeRequestMagicSkillUse(skillID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

// TestClanGateTellsTheOtherMembers has the founder, leading a clan the
// recruit joined, cast Clan Gate: the recruit is told the portal opened,
// the founder is not.
func TestClanGateTellsTheOtherMembers(t *testing.T) {
	w := bootClanWorld(t, 40, 0, 0, clanGateSkill(t), gameservertest.WithClanSeed(func(db *sql.DB) {
		seedClanRows(t, clanSeed{level: 5}, db)
		if _, err := db.ExecContext(context.Background(),
			`INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index) SELECT obj_Id, ?, 1, 0 FROM characters WHERE char_name = 'Founder'`,
			clanGateID); err != nil {
			t.Fatalf("seed Clan Gate: %v", err)
		}
	}))
	w.recruit(t)
	drainFrames(t, w.member)

	w.leader.Send(encodeRequestMagicSkillUse(clanGateID))
	leader := drainFrames(t, w.leader)
	if _, ok := firstOpcode(leader, serverpackets.OpcodeMagicSkillLaunched); !ok {
		t.Fatalf("Clan Gate cast answer = %x, want the cast launched", opcodes(leader))
	}
	if ids := messages(t, leader); slices.Contains(ids, serverpackets.SystemMessageCourtMagicianCreatedPortal) {
		t.Fatalf("caster's messages = %v, want no COURT_MAGICIAN_CREATED_PORTAL", ids)
	}
	if ids := messages(t, drainFrames(t, w.member)); !slices.Equal(ids, []int{serverpackets.SystemMessageCourtMagicianCreatedPortal}) {
		t.Fatalf("member's messages = %v, want COURT_MAGICIAN_CREATED_PORTAL", ids)
	}
}
