package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestOffensiveCastOnGuardRequiresCtrlDamage drives RequestMagicSkillUse
// through the live cast-resolution path against a real Guard NPC: an
// offensive ONE-target skill is rejected unless CTRL is pressed and the
// skill is a damage type.
func TestOffensiveCastOnGuardRequiresCtrlDamage(t *testing.T) {
	tests := []struct {
		name       string
		skillType  string
		ctrl       bool
		wantReject bool
	}{
		{name: "PDAM without ctrl", skillType: "PDAM", ctrl: false, wantReject: true},
		{name: "PDAM with ctrl", skillType: "PDAM", ctrl: true, wantReject: false},
		{name: "DEBUFF with ctrl", skillType: "DEBUFF", ctrl: true, wantReject: true},
		{name: "DEBUFF without ctrl", skillType: "DEBUFF", ctrl: false, wantReject: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const skillID int32 = 46
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
					{
						ID: modelskill.ID(skillID), Level: 1, Activation: modelskill.ActivationActive,
						Target: modelskill.TargetOne, Offensive: true, SkillType: tt.skillType,
						CastRange: 900, HitTime: 500, ReuseDelay: 60_000,
						StaticHitTime: true, StaticReuse: true, Power: 1_000_000,
					},
				})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, int(skillID), 1)
			startInWorld(t, c)
			guard := srv.SpawnHostileNPCKindAt(t, "Guard", location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			if !guard.FolkOrGuard() {
				t.Fatalf("Guard.FolkOrGuard() = false, want true")
			}
			drainUntilQuiet(t, c)

			targetHostile(t, c, guard.ObjectID())
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(skillID, tt.ctrl, false))
			if tt.wantReject {
				assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageInvalidTarget)
				assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToPawn, "Guard cast rejection rotation")
				if hp, max := guard.CurrentHP(), guard.MaxHP(); hp != max {
					t.Fatalf("Guard HP after rejected cast = %d, want unchanged %d", hp, max)
				}
				return
			}
			readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, guard.ObjectID())
		})
	}
}

// TestOffensiveCastOnSiegeGuardDoesNotUseFolkOrGuardBranch proves SiegeGuard
// is not a Folk/Guard target: PDAM without CTRL starts the cast.
func TestOffensiveCastOnSiegeGuardDoesNotUseFolkOrGuardBranch(t *testing.T) {
	const skillID int32 = 47
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: modelskill.ID(skillID), Level: 1, Activation: modelskill.ActivationActive,
				Target: modelskill.TargetOne, Offensive: true, SkillType: "PDAM",
				CastRange: 900, HitTime: 500, ReuseDelay: 60_000,
				StaticHitTime: true, StaticReuse: true, Power: 1_000_000,
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(skillID), 1)
	startInWorld(t, c)
	siege := srv.SpawnHostileNPCKindAt(t, "SiegeGuard", location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	if siege.FolkOrGuard() {
		t.Fatalf("SiegeGuard.FolkOrGuard() = true, want false")
	}
	drainUntilQuiet(t, c)

	targetHostile(t, c, siege.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, siege.ObjectID())
}

// TestUnlockableCastRejectionsUseProductionPackets drives the UNLOCKABLE
// cast boundary with real actors. Door coverage stays in the target core: the
// boot fixture does not construct a door.
func TestUnlockableCastRejectionsUseProductionPackets(t *testing.T) {
	const skillID int32 = 2065
	newServer := func(t *testing.T) (*gameservertest.Server, int32) {
		t.Helper()
		srv := gameservertest.Boot(t,
			gameservertest.WithCharacter("Newbie", 5, 0),
			gameservertest.WithWantChars(1),
			gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
				ID: modelskill.ID(skillID), Level: 1, Activation: modelskill.ActivationActive,
				Target: modelskill.TargetUnlockable, SkillType: "DELUXE_KEY_UNLOCK",
				CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
			}})),
		)
		objID := srv.SoleObjectID(t)
		seedKnownSkill(t, srv, objID, int(skillID), 1)
		startInWorld(t, srv.Client)
		return srv, objID
	}

	t.Run("Chest accepted", func(t *testing.T) {
		srv, objID := newServer(t)
		chest := srv.SpawnHostileNPCKindAt(t, "Chest", location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
		drainUntilQuiet(t, srv.Client)
		targetHostile(t, srv.Client, chest.ObjectID())
		drainUntilQuiet(t, srv.Client)
		srv.Client.Send(encodeRequestMagicSkillUse(skillID, false, false))
		readCastStartFrames(t, srv.Client, objID, skillID, 1, 500, 60_000, chest.ObjectID())
	})

	t.Run("Monster rejected", func(t *testing.T) {
		srv, _ := newServer(t)
		monster := srv.SpawnHostileNPCKindAt(t, "Monster", location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
		drainUntilQuiet(t, srv.Client)
		targetHostile(t, srv.Client, monster.ObjectID())
		drainUntilQuiet(t, srv.Client)
		srv.Client.Send(encodeRequestMagicSkillUse(skillID, false, false))
		assertStaticSystemMessage(t, srv.Client.Read(), serverpackets.SystemMessageInvalidTarget)
		assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeMoveToPawn, "unlockable rejection rotation")
		if frame := srv.Client.ReadWithTimeout(300 * time.Millisecond); frame != nil {
			t.Fatalf("unlockable rejection extra opcode = %#x, want none", frame[0])
		}
	})

	t.Run("no target", func(t *testing.T) {
		srv, _ := newServer(t)
		srv.Client.Send(encodeRequestMagicSkillUse(skillID, false, false))
		assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeActionFailed, "untargeted unlock")
		if frame := srv.Client.ReadWithTimeout(300 * time.Millisecond); frame != nil {
			t.Fatalf("untargeted unlock extra opcode = %#x, want none", frame[0])
		}
	})
}
