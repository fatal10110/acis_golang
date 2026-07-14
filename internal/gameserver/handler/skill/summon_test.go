package skill

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type creatureSummonCaster struct {
	calls int
	skill modelskill.Definition
	item  any
}

func (c *creatureSummonCaster) SummonCreature(skill modelskill.Definition, item any) {
	c.calls++
	c.skill = skill
	c.item = item
}

func TestSummonCreatureDelegatesToCasterRuntime(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &creatureSummonCaster{}
	item := struct{ objectID int32 }{objectID: 7}

	if !registry.Use(Cast{
		Caster: caster,
		Skill:  modelskill.Definition{ID: 111, SkillType: "SUMMON_CREATURE"},
		Item:   item,
	}) {
		t.Fatal("Use() returned false for SUMMON_CREATURE")
	}
	if caster.calls != 1 {
		t.Fatalf("SummonCreature calls = %d, want 1", caster.calls)
	}
	if caster.skill.ID != 111 || caster.item != item {
		t.Fatalf("SummonCreature received skill=%+v item=%+v", caster.skill, caster.item)
	}
}

type summonFriendActor struct {
	mounted, olympiad, observer, noSummonFriend bool
	dead, operating, rooted, inCombat, festival bool

	x, y, z int

	items    map[int]int
	consumed map[int]int

	teleported       bool
	tx, ty, tz, trad int

	requestOK     bool
	requests      int
	requestCaster any
	requestSkill  modelskill.Definition
	clearRequests int

	confirms       int
	confirmCaster  any
	confirmSkill   modelskill.Definition
	confirmTimeout time.Duration

	party []any
}

func newSummonFriendActor() *summonFriendActor {
	return &summonFriendActor{items: make(map[int]int), consumed: make(map[int]int), requestOK: true}
}

func (a *summonFriendActor) Mounted() bool             { return a.mounted }
func (a *summonFriendActor) OlympiadMode() bool        { return a.olympiad }
func (a *summonFriendActor) ObserverMode() bool        { return a.observer }
func (a *summonFriendActor) NoSummonFriendZone() bool  { return a.noSummonFriend }
func (a *summonFriendActor) AlikeDead() bool           { return a.dead }
func (a *summonFriendActor) Operating() bool           { return a.operating }
func (a *summonFriendActor) Rooted() bool              { return a.rooted }
func (a *summonFriendActor) InCombat() bool            { return a.inCombat }
func (a *summonFriendActor) FestivalParticipant() bool { return a.festival }
func (a *summonFriendActor) Position() (int, int, int) { return a.x, a.y, a.z }
func (a *summonFriendActor) ItemCount(itemID int) int  { return a.items[itemID] }

func (a *summonFriendActor) ConsumeItem(itemID, count int) bool {
	if a.items[itemID] < count {
		return false
	}
	a.items[itemID] -= count
	a.consumed[itemID] += count
	return true
}

func (a *summonFriendActor) TeleportTo(x, y, z, radius int) {
	a.teleported = true
	a.tx, a.ty, a.tz, a.trad = x, y, z, radius
}

func (a *summonFriendActor) TeleportRequest(caster any, skill modelskill.Definition) bool {
	a.requests++
	a.requestCaster = caster
	a.requestSkill = skill
	return a.requestOK
}

func (a *summonFriendActor) ClearTeleportRequest() { a.clearRequests++ }

func (a *summonFriendActor) ConfirmSummon(caster any, skill modelskill.Definition, timeout time.Duration) {
	a.confirms++
	a.confirmCaster = caster
	a.confirmSkill = skill
	a.confirmTimeout = timeout
}

func (a *summonFriendActor) PartyMembers() []any { return a.party }

func TestSummonFriendTeleportsTargetAndConsumesRequiredItem(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newSummonFriendActor()
	caster.x, caster.y, caster.z = 10, 20, 30
	target := newSummonFriendActor()
	target.items[57] = 2

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{ID: 1400, SkillType: "SUMMON_FRIEND", TargetConsumeID: 57, TargetConsumeCount: 2},
		Targets: []any{target},
	})

	if target.requests != 1 || target.requestCaster != caster {
		t.Fatalf("teleport request = count %d caster %v, want one request from caster", target.requests, target.requestCaster)
	}
	if !target.teleported || target.tx != 10 || target.ty != 20 || target.tz != 30 || target.trad != 20 {
		t.Fatalf("target teleport = %v to (%d,%d,%d,%d), want caster position with radius 20", target.teleported, target.tx, target.ty, target.tz, target.trad)
	}
	if target.consumed[57] != 2 || target.items[57] != 0 {
		t.Fatalf("target item consumption = consumed %d remaining %d, want consumed 2 remaining 0", target.consumed[57], target.items[57])
	}
	if target.clearRequests != 1 {
		t.Fatalf("clear teleport requests = %d, want 1", target.clearRequests)
	}
}

func TestSummonFriendConfirmationSkillDefersTeleport(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newSummonFriendActor()
	target := newSummonFriendActor()

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{ID: 1403, SkillType: "SUMMON_FRIEND"},
		Targets: []any{target},
	})

	if target.confirms != 1 || target.confirmCaster != caster || target.confirmTimeout != 30*time.Second {
		t.Fatalf("confirm = count %d caster %v timeout %s, want one 30s confirmation from caster", target.confirms, target.confirmCaster, target.confirmTimeout)
	}
	if target.teleported {
		t.Fatal("confirmation summon should not teleport until the target accepts")
	}
}

func TestSummonFriendRefusesBlockedSummonerOrTarget(t *testing.T) {
	registry := NewDefaultRegistry()

	blockedCaster := newSummonFriendActor()
	blockedCaster.noSummonFriend = true
	target := newSummonFriendActor()
	registry.Use(Cast{
		Caster:  blockedCaster,
		Skill:   modelskill.Definition{SkillType: "SUMMON_FRIEND"},
		Targets: []any{target},
	})
	if target.requests != 0 || target.teleported {
		t.Fatal("blocked summoner should not request or teleport a target")
	}

	caster := newSummonFriendActor()
	deadTarget := newSummonFriendActor()
	deadTarget.dead = true
	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "SUMMON_FRIEND"},
		Targets: []any{deadTarget},
	})
	if deadTarget.requests != 0 || deadTarget.teleported {
		t.Fatal("blocked target should not receive a teleport request")
	}
}

func TestSummonPartyTeleportsPartyMembersWithoutRequest(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newSummonFriendActor()
	caster.x, caster.y, caster.z = 3, 4, 5
	member := newSummonFriendActor()
	member.items[57] = 1
	caster.party = []any{caster, member}

	registry.Use(Cast{
		Caster: caster,
		Skill:  modelskill.Definition{SkillType: "SUMMON_PARTY", TargetConsumeID: 57, TargetConsumeCount: 1},
	})

	if member.requests != 0 {
		t.Fatalf("party summon requests = %d, want 0", member.requests)
	}
	if !member.teleported || member.tx != 3 || member.ty != 4 || member.tz != 5 {
		t.Fatalf("party member teleport = %v to (%d,%d,%d), want caster position", member.teleported, member.tx, member.ty, member.tz)
	}
	if member.consumed[57] != 1 {
		t.Fatalf("party member consumed %d required items, want 1", member.consumed[57])
	}
}

type eraseOwner struct{ vanished int }

func (o *eraseOwner) ServitorVanished() { o.vanished++ }

type erasableSummonFake struct {
	*disablerFake
	owner        any
	siege        bool
	unsummonedBy any
}

func newErasableSummonFake(owner any) *erasableSummonFake {
	return &erasableSummonFake{disablerFake: newDisablerFake(2), owner: owner}
}

func (s *erasableSummonFake) SummonOwner() any   { return s.owner }
func (s *erasableSummonFake) SiegeSummon() bool  { return s.siege }
func (s *erasableSummonFake) UnSummon(owner any) { s.unsummonedBy = owner }

func TestEraseUnsummonsNonSiegeSummonAndNotifiesOwner(t *testing.T) {
	registry := NewDefaultRegistry()
	owner := &eraseOwner{}
	summon := newErasableSummonFake(owner)

	registry.Use(Cast{
		Caster:  newDisablerFake(1),
		Skill:   modelskill.Definition{SkillType: "ERASE"},
		Targets: []any{summon},
	})

	if summon.unsummonedBy != owner {
		t.Fatalf("summon unsummoned by %v, want owner", summon.unsummonedBy)
	}
	if owner.vanished != 1 {
		t.Fatalf("owner vanish notices = %d, want 1", owner.vanished)
	}
}

func TestEraseSkipsSiegeSummon(t *testing.T) {
	registry := NewDefaultRegistry()
	owner := &eraseOwner{}
	summon := newErasableSummonFake(owner)
	summon.siege = true

	registry.Use(Cast{
		Caster:  newDisablerFake(1),
		Skill:   modelskill.Definition{SkillType: "ERASE"},
		Targets: []any{summon},
	})

	if summon.unsummonedBy != nil || owner.vanished != 0 {
		t.Fatalf("siege summon was affected: unsummonedBy=%v notices=%d", summon.unsummonedBy, owner.vanished)
	}
}
