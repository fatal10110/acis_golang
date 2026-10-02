package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	partyOtherSkillID  int32 = 1427
	partyMemberSkillID int32 = 1255
	areaStrikeSkillID  int32 = 1230
)

func targetedSkill(id int32, target modelskill.Target, offensive bool) modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive,
		Target: target, Offensive: offensive, SkillType: "BUFF",
		CastRange: 600, Radius: 200, HitTime: 500, ReuseDelay: 60_000,
		StaticHitTime: true, StaticReuse: true,
	}
}

// selectSelf clicks the caster itself and drains the click's reply.
func selectSelf(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient) {
	t.Helper()
	id := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, id)
	c.Send(encodeAction(id, int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, c)
}

// bootCasterWithBystander boots the caster knowing def plus a second,
// unflagged player standing in view, selected by the caster.
func bootCasterWithBystander(t *testing.T, def modelskill.Definition) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := bootTargetConditionCaster(t, def)
	caster := srv.Client
	bystanderID := srv.SeedCharacterFor(t, "player2", "Bystander", 5, 0).ID
	bystander := srv.DialClient(t, "player2", 1)
	startInWorld(t, caster)
	startInWorldAmongPlayers(t, bystander)
	drainUntilQuiet(t, caster)
	drainUntilQuiet(t, bystander)

	x, y, z := srv.PlayerPosition(t, bystanderID)
	caster.Send(encodeAction(bystanderID, int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, caster)
	drainUntilQuiet(t, bystander)
	return srv, caster, bystanderID
}

// TestPartyOtherCastRejections drives a PARTY_OTHER skill at each failing
// target: the caster itself, a monster, and a player outside the caster's
// party. Each answers with its own message and no ActionFailed.
func TestPartyOtherCastRejections(t *testing.T) {
	t.Parallel()
	def := targetedSkill(partyOtherSkillID, modelskill.TargetPartyOther, false)

	t.Run("self target", func(t *testing.T) {
		srv := bootTargetConditionCaster(t, def)
		c := srv.Client
		startInWorld(t, c)
		selectSelf(t, srv, c)

		c.Send(encodeRequestMagicSkillUse(partyOtherSkillID, false, false))
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotUseOnYourself)
		assertNoActionFailedUntilQuiet(t, c, "party-other self target")
	})

	t.Run("walking self target stops first", func(t *testing.T) {
		srv := bootTargetConditionCaster(t, def)
		c := srv.Client
		startInWorld(t, c)
		selectSelf(t, srv, c)

		c.Send(encodeMoveBackwardToLocation(200, 70, 30))
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToLocation, "walk")

		c.Send(encodeRequestMagicSkillUse(partyOtherSkillID, false, false))
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeStopMove, "cast stop")
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotUseOnYourself)
		assertNoActionFailedUntilQuiet(t, c, "walking party-other self target")
	})

	t.Run("monster target", func(t *testing.T) {
		srv := bootTargetConditionCaster(t, def)
		c := srv.Client
		startInWorld(t, c)
		mob := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
		drainUntilQuiet(t, c)
		targetHostile(t, c, mob.ObjectID())
		drainUntilQuiet(t, c)

		c.Send(encodeRequestMagicSkillUse(partyOtherSkillID, false, false))
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageInvalidTarget)
		assertNoActionFailedUntilQuiet(t, c, "party-other monster target")
	})

	// The bystander is in no party with the caster.
	t.Run("player outside the party", func(t *testing.T) {
		_, c, _ := bootCasterWithBystander(t, def)

		c.Send(encodeRequestMagicSkillUse(partyOtherSkillID, false, false))
		assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1CannotBeUsed, partyOtherSkillID, 1)
		assertNoActionFailedUntilQuiet(t, c, "party-other non-member")
	})
}

// TestPartyMemberCastRejections drives a PARTY_MEMBER skill at a monster and
// at a player outside the caster's party: both refuse the skill by name.
func TestPartyMemberCastRejections(t *testing.T) {
	t.Parallel()
	def := targetedSkill(partyMemberSkillID, modelskill.TargetPartyMember, false)

	t.Run("monster target", func(t *testing.T) {
		srv := bootTargetConditionCaster(t, def)
		c := srv.Client
		startInWorld(t, c)
		mob := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
		drainUntilQuiet(t, c)
		targetHostile(t, c, mob.ObjectID())
		drainUntilQuiet(t, c)

		c.Send(encodeRequestMagicSkillUse(partyMemberSkillID, false, false))
		assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1CannotBeUsed, partyMemberSkillID, 1)
		assertNoActionFailedUntilQuiet(t, c, "party-member monster target")
	})

	t.Run("player outside the party", func(t *testing.T) {
		_, c, _ := bootCasterWithBystander(t, def)

		c.Send(encodeRequestMagicSkillUse(partyMemberSkillID, false, false))
		assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1CannotBeUsed, partyMemberSkillID, 1)
		assertNoActionFailedUntilQuiet(t, c, "party-member non-member")
	})
}

// TestAreaCastOnUnflaggedPlayer aims an offensive AREA skill at an unflagged
// player: without CTRL it is an invalid target, with CTRL the cast starts.
func TestAreaCastOnUnflaggedPlayer(t *testing.T) {
	t.Parallel()
	def := targetedSkill(areaStrikeSkillID, modelskill.TargetArea, true)
	def.SkillType = "MDAM"

	t.Run("without ctrl", func(t *testing.T) {
		_, c, _ := bootCasterWithBystander(t, def)

		c.Send(encodeRequestMagicSkillUse(areaStrikeSkillID, false, false))
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageInvalidTarget)
		assertNoActionFailedUntilQuiet(t, c, "area unflagged player")
	})

	t.Run("with ctrl", func(t *testing.T) {
		srv, c, bystanderID := bootCasterWithBystander(t, def)

		c.Send(encodeRequestMagicSkillUse(areaStrikeSkillID, true, false))
		readCastStartFrames(t, c, srv.SoleObjectID(t), areaStrikeSkillID, 1, 500, 60_000, bystanderID)
	})
}
