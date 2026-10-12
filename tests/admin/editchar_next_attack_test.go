package admin

import (
	"context"
	"slices"
	"strings"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// nextAttackCastSkill is a self skill whose hit time holds a cast open.
const nextAttackCastSkill = 1030

// bootNextAttackAdmin boots a GM and a second character, Player, knowing
// nextAttackCastSkill, both in the world, with two monsters in Player's
// reach. It returns the server, Player's client and object id, and the
// monsters' object ids.
func bootNextAttackAdmin(t *testing.T) (srv *gameservertest.Server, user *testsupport.ScriptedClient, userID, first, second int32) {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: nextAttackCastSkill, Level: 1, Name: "SlowCast", Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", StaticHitTime: true, HitTime: 5000, StaticReuse: true, Magic: true,
	}}), gamesql.NewCharacterSkillStore(db))
	srv, _ = bootAdmin(t, adminLevel,
		gameservertest.WithSkills(skills),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "charinfo.htm")),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a swing or cast open needs the driven clock")
	}
	enterWorld(t, srv.Client)
	ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)
	if _, err := srv.DB.ExecContext(context.Background(), "INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index) VALUES (?, ?, 1, 0)", ch.ID, nextAttackCastSkill); err != nil {
		t.Fatalf("seed skill: %v", err)
	}
	user = srv.DialClient(t, "player2", 1)
	enterWorld(t, user)
	first = srv.SpawnHostileNPCAt(t, location.Location{X: spawnX + 30, Y: spawnY, Z: spawnZ}).ObjectID()
	second = srv.SpawnHostileNPCAt(t, location.Location{X: spawnX + 30, Y: spawnY + 20, Z: spawnZ}).ObjectID()
	drain(t, user)
	drain(t, srv.Client)
	return srv, user, ch.ID, first, second
}

// clickAttack has user select target, then attack it, and returns the
// frames that drew. No time passes on the driven clock.
func clickAttack(t *testing.T, user *testsupport.ScriptedClient, target int32) [][]byte {
	t.Helper()
	settle(t, user)
	user.Send(encodeAction(target))
	settle(t, user)
	user.Send(encodeAction(target))
	return settle(t, user)
}

// charInfoAI returns the "current <> next" intention line //debug shows for
// Player.
func charInfoAI(t *testing.T, srv *gameservertest.Server) string {
	t.Helper()
	pages := only(exchange(t, srv.Client, encodeBuildCmd("debug Player")), serverpackets.OpcodeNpcHtmlMessage)
	if len(pages) != 1 {
		t.Fatalf("//debug pages = %d, want one", len(pages))
	}
	page := htmlBody(t, pages[0])
	for _, line := range strings.Split(page, "\n") {
		if strings.Contains(line, " <> ") {
			return strings.TrimSuffix(strings.TrimSpace(line), "<br>")
		}
	}
	t.Fatalf("//debug page has no intention line:\n%s", page)
	return ""
}

// TestAdminCharacterPageNextAttack pins %nextai% (AdminEditChar
// gatherPlayerInfo, getNextIntention().getType()) naming an attack queued
// as the next intention: one clicked mid-swing (PlayableAI.tryToAttack,
// PlayableAI.java:243-250) shows ATTACK behind the swing's ATTACK, and one
// clicked mid-cast shows ATTACK behind the CAST.
func TestAdminCharacterPageNextAttack(t *testing.T) {
	t.Parallel()

	t.Run("mid-swing", func(t *testing.T) {
		t.Parallel()
		srv, user, _, first, second := bootNextAttackAdmin(t)
		if frames := clickAttack(t, user, first); !slices.Contains(opcodes(frames), serverpackets.OpcodeAttack) {
			t.Fatalf("attack frames = %x, want the opening swing", testsupport.FrameOpcodes(frames))
		}
		if got := charInfoAI(t, srv); got != "ATTACK <> IDLE" {
			t.Fatalf("intentions mid-swing = %q, want ATTACK <> IDLE", got)
		}
		if frames := clickAttack(t, user, second); !slices.Contains(opcodes(frames), serverpackets.OpcodeActionFailed) {
			t.Fatalf("mid-swing attack frames = %x, want it queued with ActionFailed", testsupport.FrameOpcodes(frames))
		}
		if got := charInfoAI(t, srv); got != "ATTACK <> ATTACK" {
			t.Fatalf("intentions after a mid-swing attack = %q, want ATTACK <> ATTACK", got)
		}
	})

	t.Run("mid-cast", func(t *testing.T) {
		t.Parallel()
		srv, user, userID, first, _ := bootNextAttackAdmin(t)
		user.Send(encodeMagicSkillUse(nextAttackCastSkill))
		settle(t, user)
		if !srv.PlayerCastingNow(t, userID) {
			t.Fatal("Player's cast is not in flight")
		}
		if got := charInfoAI(t, srv); got != "CAST <> IDLE" {
			t.Fatalf("intentions mid-cast = %q, want CAST <> IDLE", got)
		}
		clickAttack(t, user, first)
		if !srv.PlayerCastingNow(t, userID) {
			t.Fatal("the mid-cast attack click ended the cast")
		}
		if got := charInfoAI(t, srv); got != "CAST <> ATTACK" {
			t.Fatalf("intentions after a mid-cast attack = %q, want CAST <> ATTACK", got)
		}
	})
}
