package skills

import (
	"slices"
	"testing"
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestOffensiveCastOnFolkRequiresCtrlDamage drives RequestMagicSkillUse
// against a spawned civilian NPC: like a town guard, it accepts an
// offensive ONE-target skill only with CTRL pressed and only a damage skill
// (TargetOne.java:78-86); anything else is INVALID_TARGET.
func TestOffensiveCastOnFolkRequiresCtrlDamage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		skillType  string
		ctrl       bool
		wantReject bool
	}{
		{name: "PDAM without ctrl", skillType: "PDAM", ctrl: false, wantReject: true},
		{name: "PDAM with ctrl", skillType: "PDAM", ctrl: true, wantReject: false},
		{name: "DEBUFF with ctrl", skillType: "DEBUFF", ctrl: true, wantReject: true},
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
						StaticHitTime: true, StaticReuse: true, Power: 10,
					},
				})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, int(skillID), 1)
			startInWorld(t, c)
			folk := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Merchant", 30001), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			if !folk.Folk() || !folk.FolkOrGuard() {
				t.Fatalf("spawned Merchant Folk()=%v FolkOrGuard()=%v, want both true", folk.Folk(), folk.FolkOrGuard())
			}
			drainUntilQuiet(t, c)

			targetHostile(t, c, folk.ObjectID())
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(skillID, tt.ctrl, false))
			if tt.wantReject {
				assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageInvalidTarget)
				if extra := c.ReadWithTimeout(300 * time.Millisecond); extra != nil {
					t.Fatalf("Folk cast rejection extra opcode = %#x, want none", extra[0])
				}
				return
			}
			readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, folk.ObjectID())
		})
	}
}

// TestFolkAuraAffectsPlayablesOnly pins the civilian NPC's place in AURA
// target resolution (TargetAura.java:34-45): a Folk caster's aura takes in
// a nearby player, while a player's offensive aura leaves a nearby Folk out
// and still takes in a monster, the Folk not being an attackable NPC.
func TestFolkAuraAffectsPlayablesOnly(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	x, y, z := srv.PlayerPosition(t, objID)
	folk := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 30100), location.Location{X: x + 40, Y: y, Z: z})
	monster := srv.SpawnHostileNPCAt(t, location.Location{X: x - 40, Y: y, Z: z})
	p, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player not in the world")
	}
	player, ok := p.(skilltarget.Actor)
	if !ok {
		t.Fatalf("player %T is not a skill actor", p)
	}
	handler, ok := skilltarget.NewRegistry(skilltarget.WorldKnown{State: srv.State}).Handler(modelskill.TargetAura)
	if !ok {
		t.Fatal("no AURA target handler")
	}
	def := modelskill.Definition{
		ID: 1000, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetAura, Radius: 200, Offensive: true,
	}
	ids := func(actors []skilltarget.Actor) []int32 {
		var out []int32
		for _, a := range actors {
			out = append(out, a.ObjectID())
		}
		slices.Sort(out)
		return out
	}

	if got := ids(handler.Targets(folk, folk, &def)); !slices.Equal(got, []int32{objID}) {
		t.Fatalf("Folk caster aura = %v, want only the player %d", got, objID)
	}
	if got := ids(handler.Targets(player, player, &def)); !slices.Equal(got, []int32{monster.ObjectID()}) {
		t.Fatalf("player aura = %v, want only the monster %d (Folk %d left out)", got, monster.ObjectID(), folk.ObjectID())
	}
}
