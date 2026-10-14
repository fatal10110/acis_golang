package pets

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A servitor's lifetime ticks once a second.
const servitorLifeTick = time.Second

// lifetimeServitorSkill is the fixture servitor summon with a short
// lifetime: 35200 ms, losing 500 a second idle, so the remaining time is
// shown on every tick (each tick loses more than 35200/352 = 100). Its one
// upkeep checkpoint (one step) falls at half the lifetime, 17600, and costs
// one Gemstone D.
func lifetimeServitorSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: killableServitorSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON", NpcID: killableServitorNPCID,
		SummonTotalLifeTime: 35_200, SummonTimeLostIdle: 500, SummonTimeLostActive: 1000,
		ItemConsumeIDOT: int(gameservertest.GemstoneDID), ItemConsumeCountOT: 1, ItemConsumeSteps: 1,
		StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
	}
}

// bootLifetimeServitorOwner boots an owner who knows def as its servitor
// summon and carries seeds. The servitor lifetime is counted in ticks of the
// driven clock.
func bootLifetimeServitorOwner(t *testing.T, def modelskill.Definition, seeds ...seedItem) *servitorOwner {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{def}), gamesql.NewCharacterSkillStore(db))
	cat := &npc.Template{
		ID: killableServitorNPCID, TemplateID: killableServitorNPCID, Type: "Servitor", Name: "Kat the Cat", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20, CorpseTime: servitorCorpseTime,
	}
	srv := bootPets(t, gameservertest.WithNPCs(npc.NewTable([]*npc.Template{cat})), gameservertest.WithSkills(skills))
	if !srv.DrivesClock() {
		t.Skip("counting one-second lifetime ticks needs the driven clock")
	}
	ownerID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, int(def.ID), int(def.Level)); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	seeded := map[int32][]int32{}
	for _, seed := range seeds {
		seeded[seed.TemplateID] = append(seeded[seed.TemplateID], srv.GiveItem(t, ownerID, seed.TemplateID, seed.Count))
	}
	startInWorld(t, srv.Client)
	return &servitorOwner{srv: srv, client: srv.Client, id: ownerID, seeded: seeded}
}

// summonLifetimeServitor summons the servitor and lets its first lifetime
// tick come due right after: the summon cast takes the clock a little way
// into the first second.
func (o *servitorOwner) summonLifetimeServitor(t *testing.T) *summon.Actor {
	t.Helper()
	servitor := o.summonServitor(t)
	if got := servitor.Lifetime().TimeRemaining; got != 35_200 {
		t.Fatalf("summoned servitor lifetime = %d, want the full 35200", got)
	}
	return servitor
}

// tickServitor lets n lifetime ticks pass and returns the owner's frames.
func (o *servitorOwner) tickServitor(t *testing.T, n int) [][]byte {
	t.Helper()
	o.srv.Advance(t, time.Duration(n)*servitorLifeTick)
	return drainFrames(t, o.client)
}

// remainTimes returns the (total, remaining) pairs of the SetSummonRemainTime
// frames among frames, in order.
func remainTimes(frames [][]byte) [][2]int32 {
	var got [][2]int32
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSetSummonRemainTime {
			r := wire.NewReader(f[1:])
			got = append(got, [2]int32{r.ReadInt32(), r.ReadInt32()})
		}
	}
	return got
}

// opcodeIndex is the index of the first frame carrying opcode, or -1.
func opcodeIndex(frames [][]byte, opcode byte) int {
	for i, f := range frames {
		if f[0] == opcode {
			return i
		}
	}
	return -1
}

// buffServitor lands a lasting buff with an icon on servitor from its own
// queue, then drains the owner's frames it caused.
func (o *servitorOwner) buffServitor(t *testing.T, servitor *summon.Actor) {
	t.Helper()
	buff, err := effect.New(effect.Skill{ID: 1040, Level: 1}, modelskill.EffectTemplate{Name: "Buff", Time: 600, Icon: true})
	if err != nil {
		t.Fatalf("effect.New(Buff): %v", err)
	}
	buff.Effector, buff.Effected = servitor, servitor
	runOn(t, servitor.Queue(), func() { servitor.EffectList().Add(buff) })
	if !servitor.EffectList().HasHeld() {
		t.Fatal("buff did not land on the servitor")
	}
	drainFrames(t, o.client)
}

// TestServitorLifetimeTickShowsRemainingTimeThenIcons pins one lifetime
// tick of a buffed servitor: the idle servitor loses 500 ms, and its owner
// is shown the remaining lifetime (SetSummonRemainTime) and then the
// servitor's effect icons (PartySpelled), in that order.
func TestServitorLifetimeTickShowsRemainingTimeThenIcons(t *testing.T) {
	t.Parallel()
	o := bootLifetimeServitorOwner(t, lifetimeServitorSkill())
	servitor := o.summonLifetimeServitor(t)
	o.buffServitor(t, servitor)

	frames := o.tickServitor(t, 1)
	if got := servitor.Lifetime().TimeRemaining; got != 34_700 {
		t.Fatalf("lifetime after one tick = %d, want 34700", got)
	}
	if got := remainTimes(frames); len(got) != 1 || got[0] != [2]int32{35_200, 34_700} {
		t.Fatalf("SetSummonRemainTime after one tick = %v, want one (35200, 34700); opcodes %x", got, frameOpcodes(frames))
	}
	remain, icons := opcodeIndex(frames, serverpackets.OpcodeSetSummonRemainTime), opcodeIndex(frames, serverpackets.OpcodePartySpelled)
	if icons < 0 || icons < remain {
		t.Fatalf("tick frames = opcodes %x, want SetSummonRemainTime then PartySpelled", frameOpcodes(frames))
	}
}

// TestUnbuffedServitorLifetimeTickShowsNoIcons: a servitor that never held
// an effect has no icon list to refresh, so its owner is shown the remaining
// lifetime alone.
func TestUnbuffedServitorLifetimeTickShowsNoIcons(t *testing.T) {
	t.Parallel()
	o := bootLifetimeServitorOwner(t, lifetimeServitorSkill())
	o.summonLifetimeServitor(t)

	frames := o.tickServitor(t, 1)
	if got := remainTimes(frames); len(got) != 1 || got[0] != [2]int32{35_200, 34_700} {
		t.Fatalf("SetSummonRemainTime after one tick = %v, want one (35200, 34700); opcodes %x", got, frameOpcodes(frames))
	}
	if opcodeIndex(frames, serverpackets.OpcodePartySpelled) >= 0 {
		t.Fatalf("unbuffed servitor tick = opcodes %x, want no PartySpelled", frameOpcodes(frames))
	}
}

// TestServitorUpkeepPaidAtItsCheckpoint lets a servitor reach its upkeep
// checkpoint: the tick that crosses it takes the Gemstone D from the owner,
// who reads that it disappeared before the remaining time; the servitor
// stays.
func TestServitorUpkeepPaidAtItsCheckpoint(t *testing.T) {
	t.Parallel()
	o := bootLifetimeServitorOwner(t, lifetimeServitorSkill(), seedItem{TemplateID: gameservertest.GemstoneDID, Count: 2})
	servitor := o.summonLifetimeServitor(t)

	// 35 ticks leave 17700, just above the 17600 checkpoint.
	o.tickServitor(t, 35)
	if got := servitor.Lifetime().TimeRemaining; got != 17_700 {
		t.Fatalf("lifetime after 35 ticks = %d, want 17700", got)
	}
	frames := o.tickServitor(t, 1)
	paid := -1
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessageS1Disappeared {
			assertSystemMessageItem(t, f, serverpackets.SystemMessageS1Disappeared, gameservertest.GemstoneDID)
			paid = i
		}
	}
	if remain := opcodeIndex(frames, serverpackets.OpcodeSetSummonRemainTime); paid < 0 || remain < paid {
		t.Fatalf("checkpoint tick = opcodes %x, want the Gemstone D disappearing, then the remaining time", frameOpcodes(frames))
	}
	if _, ok := o.srv.State.Summon(o.id); !ok {
		t.Fatal("servitor left after its upkeep was paid")
	}
	if got := o.itemCount(t, gameservertest.GemstoneDID); got != 1 {
		t.Fatalf("owner Gemstone D after the upkeep = %d, want 1", got)
	}
}

// TestServitorLeavesWhenItsUpkeepIsShort lets a servitor whose owner holds no
// Gemstone D reach its checkpoint: the owner reads that it has not enough
// items, and the servitor leaves; the owner is still shown the remaining
// time after the PetDelete, then the buffed servitor's emptied icon list.
func TestServitorLeavesWhenItsUpkeepIsShort(t *testing.T) {
	t.Parallel()
	o := bootLifetimeServitorOwner(t, lifetimeServitorSkill())
	servitor := o.summonLifetimeServitor(t)
	o.buffServitor(t, servitor)

	o.tickServitor(t, 35)
	frames := o.tickServitor(t, 1)
	short := -1
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessageNotEnoughItems {
			short = i
		}
	}
	if gone := opcodeIndex(frames, serverpackets.OpcodePetDelete); short < 0 || gone < short {
		t.Fatalf("short checkpoint tick = opcodes %x, want NOT_ENOUGH_ITEMS then PetDelete", frameOpcodes(frames))
	}
	assertTrailingRemainTime(t, frames, [2]int32{35_200, 17_200}, true)
	if _, ok := o.srv.State.Summon(o.id); ok {
		t.Fatal("servitor stayed though its upkeep was short")
	}
}

// TestServitorLeavesWhenItsLifetimeRunsOut runs a 2000 ms servitor out: a
// lifetime at exactly 0 still holds, and the tick that takes it below 0
// unsummons the servitor, then shows its owner the negative remaining time.
// The servitor never held an effect, so no icon refresh follows.
func TestServitorLeavesWhenItsLifetimeRunsOut(t *testing.T) {
	t.Parallel()
	def := lifetimeServitorSkill()
	def.SummonTotalLifeTime, def.ItemConsumeIDOT, def.ItemConsumeCountOT, def.ItemConsumeSteps = 2000, 0, 0, 0
	o := bootLifetimeServitorOwner(t, def)
	servitor := o.summonServitor(t)

	o.tickServitor(t, 4)
	if got := servitor.Lifetime().TimeRemaining; got != 0 {
		t.Fatalf("lifetime after 4 ticks = %d, want 0", got)
	}
	if _, ok := o.srv.State.Summon(o.id); !ok {
		t.Fatal("servitor left with its lifetime at 0")
	}
	frames := o.tickServitor(t, 1)
	assertTrailingRemainTime(t, frames, [2]int32{2000, -500}, false)
	if _, ok := o.srv.State.Summon(o.id); ok {
		t.Fatal("servitor stayed past its lifetime")
	}
}

// assertTrailingRemainTime checks the frames of a servitor's departing tick:
// PetDelete, then one SetSummonRemainTime of want, then, when icons, one
// PartySpelled listing no icon, else no PartySpelled after PetDelete.
func assertTrailingRemainTime(t *testing.T, frames [][]byte, want [2]int32, icons bool) {
	t.Helper()
	gone, remain := opcodeIndex(frames, serverpackets.OpcodePetDelete), opcodeIndex(frames, serverpackets.OpcodeSetSummonRemainTime)
	if gone < 0 || remain < gone {
		t.Fatalf("departing tick = opcodes %x, want PetDelete then SetSummonRemainTime", frameOpcodes(frames))
	}
	if got := remainTimes(frames); len(got) != 1 || got[0] != want {
		t.Fatalf("departing tick SetSummonRemainTime = %v, want one %v", got, want)
	}
	trailing := ofOpcodes(frames[gone:], serverpackets.OpcodePartySpelled)
	if !icons {
		if len(trailing) != 0 {
			t.Fatalf("departing tick = opcodes %x, want no PartySpelled after PetDelete", frameOpcodes(frames))
		}
		return
	}
	if len(trailing) != 1 || opcodeIndex(frames[gone:], serverpackets.OpcodePartySpelled) < remain-gone {
		t.Fatalf("departing tick = opcodes %x, want one PartySpelled after SetSummonRemainTime", frameOpcodes(frames))
	}
	if n := spelledCount(t, trailing[0]); n != 0 {
		t.Fatalf("trailing PartySpelled lists %d icons, want 0: %x", n, trailing[0])
	}
}

// TestRevivedServitorLifetimeStaysStopped kills a servitor and revives it:
// its lifetime stopped at its death and does not start again.
func TestRevivedServitorLifetimeStaysStopped(t *testing.T) {
	t.Parallel()
	o := bootLifetimeServitorOwner(t, lifetimeServitorSkill())
	servitor := o.summonLifetimeServitor(t)
	o.tickServitor(t, 2)
	queue := o.srv.PlayerQueue(t, o.id)
	runOn(t, queue, func() { servitor.Kill(nil) })
	drainFrames(t, o.client)
	runOn(t, queue, func() { servitor.Revive() })
	if servitor.Dead() {
		t.Fatal("servitor still dead after the revive")
	}
	drainFrames(t, o.client)

	frames := o.tickServitor(t, 3)
	if got := servitor.Lifetime().TimeRemaining; got != 34_200 {
		t.Fatalf("revived servitor lifetime = %d, want 34200 kept from its death", got)
	}
	if got := remainTimes(frames); len(got) != 0 {
		t.Fatalf("revived servitor showed remaining time %v, want none", got)
	}
}

// itemCount flushes pending item writes and sums the owner's saved stacks
// of templateID.
func (o *servitorOwner) itemCount(t *testing.T, templateID int32) int {
	t.Helper()
	o.srv.FlushItems(t)
	rows, err := o.srv.Items.ListByOwner(petCtx(), o.id)
	if err != nil {
		t.Fatalf("list owner items: %v", err)
	}
	count := 0
	for _, row := range rows {
		if row.TemplateID == templateID {
			count += row.Count
		}
	}
	return count
}
