package ai

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// raidSelfBuffID is a good self buff the character knows. A self buff is
// seen by the NPCs around its caster without landing on any of them, so it
// reaches a raid private's see-spell hook alone, not its attacked hooks.
const raidSelfBuffID = modelskill.ID(4101)

// raidCurseDefinition is the raid curse the private casts.
func raidCurseDefinition() modelskill.Definition {
	return modelskill.Definition{
		ID: raidCurseID, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, Offensive: true, Magic: true, SkillType: "MDAM",
		CastRange: 600, HitTime: 1000, StaticHitTime: true, StaticReuse: true, Power: 1,
	}
}

// raidSpellBoot boots the character knowing the self buff, with the self
// buff and the raid curse in the skill table.
func raidSpellBoot(t *testing.T) raidBoot {
	t.Helper()
	db := sqltest.SharedDB(t)
	defs := []modelskill.Definition{
		{
			ID: raidSelfBuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true,
		},
		raidCurseDefinition(),
	}
	table := modelskill.NewTable(append([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}}, defs...))
	return raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(defs...),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(raidSelfBuffID), 1); err != nil {
				t.Fatalf("seed the self buff: %v", err)
			}
		},
	}
}

// castSelfBuff has the character cast its self buff.
func (w *warriorWorld) castSelfBuff() {
	p := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	p.WriteInt32(int32(raidSelfBuffID))
	p.WriteInt32(0)
	p.WriteUint8(0)
	w.srv.Client.Send(p.Bytes())
}

// TestRaidPrivateCursesOverLevelledCaster ports the see-spell hook's raid
// curse: a level 1 raid private that sees the level 20 character, more than
// 8 levels above it and not cursed, cast a spell wants to cast its raid
// curse on the character with weight 1000000.
func TestRaidPrivateCursesOverLevelledCaster(t *testing.T) {
	t.Parallel()
	w, _, private := raidPartyWith(t, raidSpellBoot(t), 1, 1)
	w.srv.InstallNPCCastSeam(t, private, modelskill.NewTable([]modelskill.Definition{raidCurseDefinition()}))
	if d, ok := desire(private, actorai.IntentionCast); ok {
		t.Fatalf("raid private wants %+v before the spell", d)
	}

	w.castSelfBuff()
	w.srv.AdvanceUntil(t, "the raid curse desire", func() bool {
		_, ok := desire(private, actorai.IntentionCast)
		return ok
	})
	d, _ := desire(private, actorai.IntentionCast)
	if d.Skill.ID != raidCurseID || d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("raid private cast desire = %+v, want its raid curse on the character, weight 1000000", d)
	}
}

// TestRaidPrivateJumpsToCasterItHunts ports the see-spell hook's
// path-failure branch: a hurt raid private whose top desire target is the
// character, after more than 10 failed paths, is teleported onto the
// character when it sees the character cast a spell. The character is at
// most 8 levels above the private, so no curse is wanted.
func TestRaidPrivateJumpsToCasterItHunts(t *testing.T) {
	t.Parallel()
	w, _, private := raidPartyWith(t, raidSpellBoot(t), 20, 20)

	w.hit(t, private, 100)
	w.ticks(t, 2)
	if top := private.AI().TopDesireTarget(); top == nil || top.ObjectID() != w.player.ObjectID() {
		t.Fatalf("raid private top desire target = %v, want the character", top)
	}
	w.onQueue(t, private, func() {
		script.NPCOf(private).InstantTeleportTo(script.Loc{X: w.at.X + 500, Y: w.at.Y, Z: w.at.Z})
		for range 11 {
			private.AddGeoPathFailCount()
		}
	})
	w.srv.ReadQueued(t, w.srv.Client)
	if private.HP() == float64(private.MaxHP()) {
		t.Fatal("setup: the raid private is not hurt")
	}

	w.castSelfBuff()
	w.srv.AdvanceUntil(t, "the raid private's jump onto the character", func() bool {
		x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID())
		return positionOf(private) == location.Location{X: x, Y: y, Z: z}
	})
	if d, ok := desire(private, actorai.IntentionCast); ok {
		t.Fatalf("raid private wants %+v, want no curse on a caster at most 8 levels above it", d)
	}
}

// TestRaidPrivateHitInPeaceGoesHome ports the party-attacked hook's peace
// zone branch: a raid private hit inside a peace zone is teleported to its
// spawn location and drops its attack desires and hate, the ones the hit
// just added included. The private is a live NPC of the population, which
// enters the WithZones zones as production's does.
func TestRaidPrivateHitInPeaceGoesHome(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(200, 3_000, -1_000, 1_000, -1_000, 1_000)
	if err != nil {
		t.Fatalf("zone form: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewPeace(1, form))
	w, _, private := raidPartyWith(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithZones(zones)}}, 20, 20)

	home := script.NPCOf(private).SpawnLocation()
	w.teleport(t, private, 600, 0)
	if positionOf(private) == (location.Location{X: home.X, Y: home.Y, Z: home.Z}) {
		t.Fatal("setup: the raid private still stands on its spawn location")
	}
	if !private.InsideZone(zone.FlagPeace) {
		t.Fatal("setup: the raid private is not in the peace zone")
	}

	w.hit(t, private, 10)
	w.srv.ReadQueued(t, w.srv.Client)
	if got, want := positionOf(private), (location.Location{X: home.X, Y: home.Y, Z: home.Z}); got != want {
		t.Fatalf("raid private at %v after a hit in peace, want its spawn location %v", got, want)
	}
	if d, ok := desire(private, actorai.IntentionAttack); ok {
		t.Fatalf("raid private hit in peace still wants to attack: %+v", d)
	}
	if got := private.AggroHate(w.player); got != 0 {
		t.Fatalf("raid private hate after a hit in peace = %v, want its hate cleaned", got)
	}
}
