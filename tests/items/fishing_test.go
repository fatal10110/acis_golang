package items

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/fish"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Fishing fixtures, after the datapack's Albatross Rod, Green Colored Lure,
// D-grade Fishing Shot and Small Green Nimble Fish, and the Fishing,
// Pumping, Reeling and Fishing Expertise skills.
const (
	fishRodD     int32 = 6530
	fishLure     int32 = 6520 // normal group, medium grade
	fishShotD    int32 = 6536
	fishShotC    int32 = 6537
	fishCaughtID int32 = 6411

	fishingSkill  = 1312
	pumpingSkill  = 1313
	reelingSkill  = 1314
	expertiseSkil = 1315
)

// fishingWaterLevel is the fishing zone's surface, where a line floats 10
// above; fishingGeo puts the ground below it.
const fishingWaterLevel = 0

// fishingGeo is the passable test geodata with the ground 100 below any
// probe, so fishing water anywhere has its bottom under the surface.
type fishingGeo struct{ gameservertest.Geo }

func (fishingGeo) Height(_, _, z int) int16 { return int16(z - 100) }

func fishingCatalog() *item.Table {
	return item.NewTable(append(gameservertest.ItemTemplates().All(),
		&item.Template{
			ID: fishRodD, Name: "Albatross Rod", Kind: item.KindWeapon, Slot: item.SlotLRHand, Duration: -1,
			Crystal: item.CrystalD, Destroyable: true, DefaultAction: item.ActionEquip,
			Weapon: &item.WeaponDetail{Type: item.WeaponFishingRod},
		},
		&item.Template{
			ID: fishLure, Name: "Green Colored Lure", Kind: item.KindEtcItem, Slot: item.SlotLHand, Duration: -1,
			Stackable: true, Destroyable: true, DefaultAction: item.ActionEquip,
			EtcItem: &item.EtcItemDetail{Type: item.EtcItemLure},
		},
		&item.Template{
			ID: fishShotD, Name: "Fishing Shot: D-grade", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
			Destroyable: true, Crystal: item.CrystalD, DefaultAction: item.ActionFishingShot,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "FishShots"},
			AttachedSkills: []item.SkillRef{{ID: 2182, Level: 1}},
		},
		&item.Template{
			ID: fishShotC, Name: "Fishing Shot: C-grade", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
			Destroyable: true, Crystal: item.CrystalC, DefaultAction: item.ActionFishingShot,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "FishShots"},
			AttachedSkills: []item.SkillRef{{ID: 2183, Level: 1}},
		},
		&item.Template{
			ID: fishCaughtID, Name: "Small Green Nimble Fish", Kind: item.KindEtcItem, Duration: -1,
			Destroyable: true, EtcItem: &item.EtcItemDetail{},
		},
	))
}

// fishingSkills knows the three fishing skills at level 1, with the
// datapack's level-1 power of 24, no hit time and no reuse.
func fishingSkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	active := func(id modelskill.ID, typ string, power float32) modelskill.Definition {
		return modelskill.Definition{ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, SkillType: typ, Power: power}
	}
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: 248, Level: 3},
		{ID: 294, Level: 1},
		active(fishingSkill, "FISHING", 0),
		active(pumpingSkill, "PUMPING", 24),
		active(reelingSkill, "REELING", 24),
		{ID: expertiseSkil, Level: 1, Activation: modelskill.ActivationPassive},
	}), gamesql.NewCharacterSkillStore(db))
}

// fishingDice answers every fishing roll: 0 for the bait distance, bite for
// the bite (guts 500 beats 0, never 500 or more), check for each percentile
// roll (50: the fish level stays, a green lure draws a nimble fish, the fish
// rests, no attempt is resisted, no catch is a monster).
type fishingDice struct{ check, bite atomic.Int32 }

func (d *fishingDice) roll(n int) int {
	switch n {
	case 100:
		return int(d.check.Load())
	case 1000:
		return int(d.bite.Load())
	}
	return 0
}

type fishingRig struct {
	srv                            *gameservertest.Server
	c                              *testsupport.ScriptedClient
	objID, rod, lure, shotD, shotC int32
	dice                           *fishingDice
	// castAt is the client clock when the last cast was requested.
	castAt time.Time
}

// bootFishing boots a player at (10, 20, 30) facing east, a fishing zone
// starting 200 ahead of it, holding a D-grade rod with five lures on it
// and knowing the fishing skills.
func bootFishing(t *testing.T, opts ...gameservertest.Option) *fishingRig {
	t.Helper()
	return bootFishingWith(t, true, opts...)
}

// bootFishingWith is bootFishing, the lures left off the rod unless
// equipLure.
func bootFishingWith(t *testing.T, equipLure bool, opts ...gameservertest.Option) *fishingRig {
	t.Helper()
	form, err := zone.NewCuboid(200, 600, -300, 300, -500, fishingWaterLevel)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewFishing(1, form))
	dice := &fishingDice{}
	dice.check.Store(50)
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithItemTemplates(fishingCatalog()),
		gameservertest.WithSkills(fishingSkills(t)),
		gameservertest.WithFish(fish.NewTable([]fish.Fish{
			fish.New(fishCaughtID, 1, 100, 4, 1, 1, 500, 5000, 20000, 24000),
		})),
		gameservertest.WithFishingRoll(dice.roll),
		gameservertest.WithZones(zones),
		gameservertest.WithGeo(fishingGeo{}),
		gameservertest.WithCharacter("Fisher", 20, 0),
		gameservertest.WithWantChars(1),
	}, opts...)...)
	r := &fishingRig{srv: srv, c: srv.Client, objID: srv.SoleObjectID(t), dice: dice}
	for _, id := range []int{fishingSkill, pumpingSkill, reelingSkill, expertiseSkil} {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), r.objID, 0, id, 1); err != nil {
			t.Fatalf("grant skill %d: %v", id, err)
		}
	}
	r.rod = srv.GiveItem(t, r.objID, fishRodD, 1)
	r.lure = srv.GiveItem(t, r.objID, fishLure, 5)
	r.shotD = srv.GiveItem(t, r.objID, fishShotD, 3)
	r.shotC = srv.GiveItem(t, r.objID, fishShotC, 1)
	startInWorld(t, r.c)
	r.c.Send(encodeUseItem(r.rod, false))
	drainUntilQuiet(t, r.c)
	if equipLure {
		r.c.Send(encodeUseItem(r.lure, false))
		drainUntilQuiet(t, r.c)
	}
	return r
}

// fishingEvent describes a frame of the fishing flow, "" for any other.
func fishingEvent(t *testing.T, f []byte) string {
	t.Helper()
	r := wire.NewReader(f[1:])
	switch f[0] {
	case serverpackets.OpcodeSystemMessage:
		id := r.ReadInt32()
		s := fmt.Sprintf("msg %d", id)
		for range r.ReadInt32() {
			switch typ := r.ReadInt32(); typ {
			case serverpackets.SystemMessageParamText:
				s += " " + r.ReadString()
			case serverpackets.SystemMessageParamSkillName:
				s += fmt.Sprintf(" %d/%d", r.ReadInt32(), r.ReadInt32())
			default:
				s += fmt.Sprintf(" %d", r.ReadInt32())
			}
		}
		return s
	case serverpackets.OpcodePlaySound:
		typ := r.ReadInt32()
		return fmt.Sprintf("sound %d %s", typ, r.ReadString())
	case serverpackets.OpcodeActionFailed:
		return "action failed"
	case serverpackets.OpcodeMagicSkillUse:
		r.ReadInt32()
		r.ReadInt32()
		return fmt.Sprintf("skill use %d", r.ReadInt32())
	case serverpackets.OpcodeExtended:
		sub := r.ReadUint16()
		id := r.ReadInt32()
		switch sub {
		case serverpackets.OpcodeExFishingStart:
			typ, x, y, z := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
			return fmt.Sprintf("start %d type %d at %d,%d,%d night %d ranking %d", id, typ, x, y, z, r.ReadUint8(), r.ReadUint8())
		case serverpackets.OpcodeExFishingEnd:
			return fmt.Sprintf("end %d win %d", id, r.ReadUint8())
		case serverpackets.OpcodeExFishingStartCombat:
			tm, hp := r.ReadInt32(), r.ReadInt32()
			return fmt.Sprintf("combat %d time %d hp %d mode %d lure %d deceptive %d", id, tm, hp, r.ReadUint8(), r.ReadUint8(), r.ReadUint8())
		case serverpackets.OpcodeExFishingHpRegen:
			tm, hp := r.ReadInt32(), r.ReadInt32()
			mode, good, anim := r.ReadUint8(), r.ReadUint8(), r.ReadUint8()
			penalty, deceptive := r.ReadInt32(), r.ReadUint8()
			if good == 0 && anim == 0 {
				return fmt.Sprintf("tick time %d hp %d mode %d penalty %d deceptive %d", tm, hp, mode, penalty, deceptive)
			}
			// An action's gauge carries the fight's time left, which
			// depends on how many seconds the reads let pass: left out.
			return fmt.Sprintf("gauge hp %d mode %d good %d anim %d penalty %d deceptive %d", hp, mode, good, anim, penalty, deceptive)
		}
	}
	return ""
}

// fishingEvents collects the fishing flow frames until the client is quiet,
// leaving out the fight's per-second gauge: each quiet read lets time pass,
// so how many seconds of the fight it spans is not fixed.
func fishingEvents(t *testing.T, c *testsupport.ScriptedClient) []string {
	t.Helper()
	var out []string
	for _, f := range collectUntilQuiet(t, c) {
		if e := fishingEvent(t, f); e != "" && !strings.HasPrefix(e, "tick ") {
			out = append(out, e)
		}
	}
	return out
}

// awaitEvent lets time pass until a fishing event starting with prefix
// arrives, and returns the events up to it and how long after the last
// cast request it came.
func (r *fishingRig) awaitEvent(t *testing.T, prefix string) (events []string, after time.Duration) {
	t.Helper()
	limit := r.c.Now().Add(40 * time.Second)
	for r.c.Now().Before(limit) {
		f := r.c.ReadWithTimeout(20 * time.Millisecond)
		if f == nil {
			continue
		}
		e := fishingEvent(t, f)
		if e == "" {
			continue
		}
		events = append(events, e)
		if strings.HasPrefix(e, prefix) {
			return events, r.c.Now().Sub(r.castAt)
		}
	}
	t.Fatalf("no %q within 40s; got %q", prefix, events)
	return nil, 0
}

func requireEvents(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s:\n got  %q\n want %q", what, got, want)
	}
}

// castStart is the caster's own start of a cast of skillID: MagicSkillUse,
// then USE_S1 naming it.
func castStart(skillID int) []string {
	return []string{fmt.Sprintf("skill use %d", skillID), fmt.Sprintf("msg 46 %d/1", skillID)}
}

// cast sends skillID and lets its cast land.
func (r *fishingRig) cast(t *testing.T, skillID int32) []string {
	t.Helper()
	r.castAt = r.c.Now()
	r.c.Send(encodeRequestMagicSkillUse(skillID))
	r.srv.Advance(t, 100*time.Millisecond)
	return fishingEvents(t, r.c)
}

// TestFishingCastBiteFightAndCatch casts a line into the fishing water
// ahead, waits for the bite, pumps the resting fish down with a fishing
// shot spent on the first pump and catches it: Fishing.useSkill,
// FishingStance.start/the look task/the fight task/usePomping/end.
func TestFishingCastBiteFightAndCatch(t *testing.T) {
	t.Parallel()
	r := bootFishing(t)
	id := r.objID

	requireEvents(t, "cast", r.cast(t, fishingSkill), append(castStart(fishingSkill),
		"msg 1461",
		fmt.Sprintf("start %d type 1 at 260,20,10 night 0 ranking 0", id),
		"sound 1 SF_P_01")...)
	r.srv.FlushItems(t)
	if inst := mustFindItem(t, r.srv, id, r.lure); inst.Count != 4 {
		t.Fatalf("lures after the cast = %d, want 4", inst.Count)
	}

	// Emotes are refused while the line is out.
	r.c.Send(encodeRequestSocialActionFishing(2))
	requireEvents(t, "emote while fishing", fishingEvents(t, r.c), "msg 1471")

	// The first look comes 10 s after the cast and gets the bite; the fight
	// gauge then reaches the fisher every second.
	bite, after := r.awaitEvent(t, "combat ")
	if after < 10*time.Second || after > 10500*time.Millisecond {
		t.Fatalf("bite %v after the cast, want 10s", after)
	}
	requireEvents(t, "bite", bite, fmt.Sprintf("combat %d time 24 hp 100 mode 0 lure 1 deceptive 0", id))
	requireEvents(t, "bite notice", fishingEvents(t, r.c), "sound 1 SF_S_01", "msg 1449")
	if tick, _ := r.awaitEvent(t, "tick "); !strings.HasPrefix(tick[len(tick)-1], "tick time 2") || !strings.HasSuffix(tick[len(tick)-1], " hp 100 mode 0 penalty 0 deceptive 0") {
		t.Fatalf("fight second = %q", tick)
	}

	r.c.Send(encodeUseItem(r.shotD, false))
	requireEvents(t, "fishing shot", fishingEvents(t, r.c), "skill use 2182")

	// A charged shot doubles the first pump: 24 * 1.1 * 2 = 52.8.
	requireEvents(t, "charged pump", r.cast(t, pumpingSkill), append(castStart(pumpingSkill),
		"msg 1465 52",
		"gauge hp 48 mode 0 good 1 anim 1 penalty 0 deceptive 0")...)
	requireEvents(t, "second pump", r.cast(t, pumpingSkill), append(castStart(pumpingSkill),
		"msg 1465 26",
		"gauge hp 22 mode 0 good 1 anim 1 penalty 0 deceptive 0")...)
	// Reeling a resting fish fails: it gets the damage back.
	requireEvents(t, "reel at rest", r.cast(t, reelingSkill), append(castStart(reelingSkill),
		"msg 1468 26",
		"gauge hp 48 mode 0 good 2 anim 2 penalty 0 deceptive 0")...)
	r.cast(t, pumpingSkill)
	requireEvents(t, "catch", r.cast(t, pumpingSkill), append(castStart(pumpingSkill),
		"msg 1465 26",
		"gauge hp 0 mode 0 good 1 anim 1 penalty 0 deceptive 0",
		"msg 1469",
		fmt.Sprintf("msg 30 %d", fishCaughtID),
		fmt.Sprintf("end %d win 1", id),
		"msg 1460")...)

	r.srv.Advance(t, 3*time.Second)
	requireEvents(t, "after the catch", fishingEvents(t, r.c))
	r.srv.FlushItems(t)
	if inst := mustFindItemByTemplate(t, r.srv, id, fishCaughtID); inst.Count != 1 {
		t.Fatalf("caught fish count = %d, want 1", inst.Count)
	}
	if inst := mustFindItem(t, r.srv, id, r.shotD); inst.Count != 2 {
		t.Fatalf("fishing shots left = %d, want 2", inst.Count)
	}
	// Pumping is refused once the fight is over.
	requireEvents(t, "pump after the catch", r.cast(t, pumpingSkill), append(castStart(pumpingSkill),
		"msg 1462",
		"action failed")...)
}

// TestFishingCastAgainCancels takes a cast line out of the water with a
// second Fishing cast; a line nothing bites comes back on its own once the
// first look's 10 s and the fish's 20 s wait have run out.
func TestFishingCastAgainCancels(t *testing.T) {
	t.Parallel()
	r := bootFishing(t)
	id := r.objID
	r.cast(t, fishingSkill)
	requireEvents(t, "cancel", r.cast(t, fishingSkill), append(castStart(fishingSkill),
		fmt.Sprintf("end %d win 0", id),
		"msg 1460",
		"msg 1458")...)
	r.srv.Advance(t, 15*time.Second)
	requireEvents(t, "after the cancel", fishingEvents(t, r.c))

	r.dice.bite.Store(500)
	r.cast(t, fishingSkill)
	ended, after := r.awaitEvent(t, "end ")
	if after < 30*time.Second || after > 30500*time.Millisecond {
		t.Fatalf("empty line back %v after the cast, want 30s", after)
	}
	requireEvents(t, "wait run out", ended, fmt.Sprintf("end %d win 0", id))
	requireEvents(t, "wait run out notice", fishingEvents(t, r.c), "msg 1460")
	r.srv.FlushItems(t)
	if inst := mustFindItem(t, r.srv, id, r.lure); inst.Count != 3 {
		t.Fatalf("lures after two casts = %d, want 3", inst.Count)
	}
}

// TestFishingCastRefusals pins Fishing.useSkill's refusals, in order, none
// of which uses up a lure.
func TestFishingCastRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		equipLure bool
		opts      []gameservertest.Option
		setup     func(t *testing.T, r *fishingRig)
		want      string
	}{
		{name: "no rod", equipLure: true, setup: func(t *testing.T, r *fishingRig) {
			r.c.Send(encodeUseItem(r.rod, false))
			drainUntilQuiet(t, r.c)
		}, want: "msg 1453"},
		{name: "operating", equipLure: true, setup: func(t *testing.T, r *fishingRig) {
			r.srv.SetPlayerOperating(t, r.objID, true)
		}, want: "msg 1638"},
		{name: "no lure", want: "msg 1454"},
		{name: "no fishing water", equipLure: true, opts: []gameservertest.Option{gameservertest.WithZones(zone.NewIndex())}, want: "msg 1457"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := bootFishingWith(t, tc.equipLure, tc.opts...)
			if tc.setup != nil {
				tc.setup(t, r)
			}
			requireEvents(t, tc.name, r.cast(t, fishingSkill), append(castStart(fishingSkill), tc.want)...)
			r.srv.FlushItems(t)
			if inst := mustFindItem(t, r.srv, r.objID, r.lure); inst.Count != 5 {
				t.Fatalf("lures after a refused cast = %d, want 5", inst.Count)
			}
		})
	}
}

// TestFishShotRefusals pins FishShots: a shot of the wrong grade is
// refused, a second one on a charged rod is silent, and without a rod
// nothing is charged.
func TestFishShotRefusals(t *testing.T) {
	t.Parallel()
	r := bootFishing(t)
	shotC := r.shotC

	r.c.Send(encodeUseItem(shotC, false))
	requireEvents(t, "wrong grade", fishingEvents(t, r.c), "msg 1479", "action failed")
	r.c.Send(encodeUseItem(r.shotD, false))
	requireEvents(t, "charge", fishingEvents(t, r.c), "skill use 2182")
	r.c.Send(encodeUseItem(r.shotD, false))
	requireEvents(t, "already charged", fishingEvents(t, r.c))
	r.c.Send(encodeUseItem(r.rod, false))
	drainUntilQuiet(t, r.c)
	r.c.Send(encodeUseItem(r.shotD, false))
	requireEvents(t, "no rod", fishingEvents(t, r.c), "action failed")
	r.srv.FlushItems(t)
	if inst := mustFindItem(t, r.srv, r.objID, r.shotD); inst.Count != 2 {
		t.Fatalf("fishing shots left = %d, want 2", inst.Count)
	}
	if inst := mustFindItem(t, r.srv, r.objID, shotC); inst.Count != 1 {
		t.Fatalf("wrong-grade shots left = %d, want 1", inst.Count)
	}
}

func encodeRequestSocialActionFishing(actionID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestSocialAction)
	w.WriteInt32(actionID)
	return w.Bytes()
}
