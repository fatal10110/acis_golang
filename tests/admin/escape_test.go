package admin

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestGMEscapeCastsOneSecond pins /unstuck (user command 52) for a game
// master (Escape.useUserCommand's isGM branch): no sound and no notice, only
// the cast of Escape: 1 second (2100) on itself. The skill row is the
// shipped one (skills/2100-2199.xml: target SELF, hitTime 1000, static,
// RECALL).
func TestGMEscapeCastsOneSecond(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: 2100, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "RECALL", HitTime: 1000, StaticHitTime: true, MagicLevel: 1,
	}}), gamesql.NewCharacterSkillStore(db))
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithSkills(skills))
	enterWorld(t, srv.Client)

	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUserCommand)
	w.WriteInt32(52)
	srv.Client.Send(w.Bytes())
	first := srv.Client.ReadWithTimeout(5 * time.Second)
	if first == nil {
		t.Fatal("GM /unstuck sent nothing")
	}
	for first[0] != serverpackets.OpcodeMagicSkillUse {
		if first[0] == serverpackets.OpcodePlaySound || first[0] == serverpackets.OpcodeSystemMessage {
			t.Fatalf("GM /unstuck sent %#x before its cast, want no sound or notice", first[0])
		}
		if first = srv.Client.ReadWithTimeout(5 * time.Second); first == nil {
			t.Fatal("GM /unstuck cast nothing")
		}
	}
	r := wire.NewReader(first[1:])
	caster, target, skill, level, hit := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if caster != gmID || target != gmID || skill != 2100 || level != 1 || hit != 1000 {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d/%d hit %d, want %d on itself, 2100/1, 1000", caster, target, skill, level, hit, gmID)
	}
}
