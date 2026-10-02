package skill

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect/effecttest"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// fakeActor supplies the Actor surface every cast participant carries, so a
// test double only has to spell out the capability the case under test
// actually exercises. Prefer a real actor (see newDisablerHostile) when the
// case is about the behavior rather than about one narrow surface; a double
// that models death or identity itself declares its own Dead or ObjectID,
// which shadows the one embedded here. Kept per
// docs/agents/test-strategy.md: it stands in for no production type of its
// own, only an object id the narrow doubles below share.
type fakeActor struct {
	effecttest.Actor
	objectID int32
}

func (f fakeActor) ObjectID() int32 { return f.objectID }

func (fakeActor) Dead() bool { return false }

// placedFakeActor is a fakeActor with a world placement, for the call sites
// that pass a bare actor rather than a positioned double.
type placedFakeActor struct {
	fakeActor
	world.Presence
}

// ---- from cancel_test.go ----
type cancelFakeActor struct {
	neutralCreature
	world.Presence
	fakeActor
	dead  bool
	level int
	list  *effect.List
}

func newCancelFakeActor(level int) *cancelFakeActor {
	return &cancelFakeActor{level: level, list: newTestList(nil)}
}

func (a *cancelFakeActor) Dead() bool               { return a.dead }
func (a *cancelFakeActor) Level() int               { return a.level }
func (a *cancelFakeActor) EffectList() *effect.List { return a.list }

func addBuff(t *testing.T, actor *cancelFakeActor, tmpl modelskill.EffectTemplate, meta effect.Skill) *effect.Effect {
	t.Helper()
	e, err := effect.New(meta, tmpl)
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = actor
	actor.list.Add(e)
	return e
}

func hasEffect(list *effect.List, e *effect.Effect) bool {
	for _, cur := range list.All() {
		if cur == e {
			return true
		}
	}
	return false
}

// ---- from continuous_fixtures_test.go ----
// reflect sources wired to a guaranteed-success roll by default.
type continuousFake struct {
	neutralCreature
	world.Presence
	id                int32
	dead, invul       bool
	denyDamage        bool
	playable          bool
	attackableFlag    bool
	cursed            bool
	bss               bool
	list              *effect.List
	successOK         bool
	reflectOK         bool
	successInput      formulas.SkillSuccessInput
	skillReflectInput formulas.SkillReflectInput

	// recordSuccessInput, when set, is called with every SkillSuccessInput
	// invocation's raw arguments, letting tests assert on the resolved
	// caster/shield state without duplicating checkSkillSuccess's logic.
	recordSuccessInput func(caster any, def modelskill.Definition, bss bool, shield formulas.ShieldDefense)

	// aggression-event recording: which optional surface fired, and with
	// what arguments.
	aggressionSource  any
	aggressionPower   int
	currentTarget     world.Tracked
	setTargetCalls    []world.Tracked
	attackTargetCalls []world.Tracked
}

func newContinuousFake(id int32) *continuousFake {
	return &continuousFake{
		id:           id,
		list:         newTestList(nil),
		successOK:    true,
		successInput: formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: 100},
		reflectOK:    true,
	}
}

func (f *continuousFake) ObjectID() int32                { return f.id }
func (*continuousFake) Kind() actor.Kind                 { return actor.KindNPC }
func (*continuousFake) CharacterName() string            { return "Target" }
func (f *continuousFake) Dead() bool                     { return f.dead }
func (f *continuousFake) Invul() bool                    { return f.invul }
func (f *continuousFake) CanGiveDamage() bool            { return !f.denyDamage }
func (f *continuousFake) Playable() bool                 { return f.playable }
func (f *continuousFake) Attackable() bool               { return f.attackableFlag }
func (f *continuousFake) CursedWeaponEquipped() bool     { return f.cursed }
func (f *continuousFake) EffectList() *effect.List       { return f.list }
func (f *continuousFake) BlessedSpiritshotCharged() bool { return f.bss }

func (f *continuousFake) SkillSuccessInput(caster creature.FormulaActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if f.recordSuccessInput != nil {
		f.recordSuccessInput(caster, def, bss, shield)
	}
	return f.successInput, f.successOK
}

func (f *continuousFake) SkillReflectInput(modelskill.Definition) formulas.SkillReflectInput {
	return f.skillReflectInput
}

func (f *continuousFake) NotifyAggression(source attackable.Combatant, power int) {
	f.aggressionSource = source
	f.aggressionPower = power
}

func (f *continuousFake) CurrentTarget() world.Tracked { return f.currentTarget }

func (f *continuousFake) SetTarget(target world.Tracked) {
	f.setTargetCalls = append(f.setTargetCalls, target)
}

func (f *continuousFake) AttackTarget(target world.Tracked) {
	f.attackTargetCalls = append(f.attackTargetCalls, target)
}

func buffEffect() []modelskill.EffectTemplate {
	return []modelskill.EffectTemplate{{Name: "Buff", Time: 600}}
}

type continuousDefinitions map[modelskill.Ref]modelskill.Definition

func (d continuousDefinitions) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	def, ok := d[ref]
	return def, ok
}

func (d continuousDefinitions) MaxLevel(id modelskill.ID) int {
	max := 0
	for ref := range d {
		if ref.ID == id && ref.Level > max {
			max = ref.Level
		}
	}
	return max
}

// ---- from cubic_test.go ----
// fakeCubicSummoner records which cubics a cast admitted and whether each
// was granted by another player. *player.Character is the real summoner and
// the cubic tests that only need the admitted list or the servitor request
// use it, but the given-by-other flag it keeps is read by nothing it
// exports, so the tests pinning that flag keep this double. Kept per
// docs/agents/test-strategy.md.
type fakeCubicSummoner struct {
	neutralCreature
	world.Presence
	fakeActor
	added        map[cubic.ID]bool
	givenByOther map[cubic.ID]bool
	nextAdded    bool
}

// newFakeCubicSummoner returns a summoner with its own world object id, so a
// mass cast can tell the caster from the other recipients.
func newFakeCubicSummoner(nextAdded bool) *fakeCubicSummoner {
	return &fakeCubicSummoner{
		fakeActor:    fakeActor{objectID: nextFakeObjectID()},
		added:        map[cubic.ID]bool{},
		givenByOther: map[cubic.ID]bool{},
		nextAdded:    nextAdded,
	}
}

func (f *fakeCubicSummoner) AddOrRefreshCubic(id cubic.ID, givenByOther bool) (touched, added bool) {
	f.added[id] = true
	f.givenByOther[id] = givenByOther
	return true, f.nextAdded
}

func (*fakeCubicSummoner) SummonServitor(modelskill.Definition) {}

// ---- from disablers_test.go ----
// disablerFake is a Combatant (for the hate-table skill types) that also
// satisfies every optional interface disablersHandler probes for, wired to
// a guaranteed-success SkillSuccessInput by default (IgnoreResists with a
// 100 base chance always beats a [0,100) roll).
type disablerFake struct {
	world.Presence
	neutralCreature
	id                     int32
	dead, invul, paralyzed bool
	list                   *effect.List
	successOK              bool
	attackableFlag         bool
	raidRelated            bool
	undeadFlag             bool
	aggro                  *attackable.ThreatTable
	hate                   *attackable.HateTable
	shield                 formulas.ShieldDefense
	level                  int
	reflects               bool
	name                   string
	// failRoll makes the skill's own landing roll against d always fail.
	failRoll bool

	// shieldRolls counts ShieldDefense calls; templateLandings records the
	// blessed-spiritshot and shield inputs of every per-template landing
	// roll made against d.
	shieldRolls      int
	templateLandings []templateLanding

	// lastBss and lastShield record the most recent SkillSuccessInput call's
	// resolved caster/target state, for tests asserting checkSkillSuccess
	// threaded them through correctly.
	lastBss    bool
	lastShield formulas.ShieldDefense

	// aggressionSource and aggressionPower record the most recent
	// NotifyAggression call, for tests asserting AGGDAMAGE's aggro
	// notification.
	aggressionSource any
	aggressionPower  int
}

func newDisablerFake(id int32) *disablerFake {
	d := &disablerFake{id: id, list: newTestList(nil), successOK: true}
	d.aggro = attackable.NewThreatTable(d, time.Now)
	d.hate = attackable.NewHateTable(d)
	return d
}

func (d *disablerFake) ObjectID() int32          { return d.id }
func (d *disablerFake) SiegeGuard() bool         { return false }
func (d *disablerFake) AlikeDead() bool          { return d.dead }
func (d *disablerFake) Dead() bool               { return d.dead }
func (d *disablerFake) Invul() bool              { return d.invul }
func (d *disablerFake) Paralyzed() bool          { return d.paralyzed }
func (d *disablerFake) EffectList() *effect.List { return d.list }

func (d *disablerFake) SkillSuccessInput(caster creature.FormulaActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	d.lastBss = bss
	d.lastShield = shield
	chance := 100.0
	if d.failRoll {
		chance = 0
	}
	return formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: chance, Shield: shield}, d.successOK
}

func (d *disablerFake) CharacterName() string { return d.name }

// ShieldDefense reports d's pre-set shield-block outcome, letting tests
// exercise checkSkillSuccess's shield-block threading.
func (d *disablerFake) ShieldDefense(caster creature.FormulaActor, def modelskill.Definition, isCrit bool) formulas.ShieldDefense {
	d.shieldRolls++
	return d.shield
}

// templateLanding is one per-template landing roll's resolved inputs.
type templateLanding struct {
	bss    bool
	shield formulas.ShieldDefense
}

// EffectSuccessInput records the landing inputs and lands the template
// unless the shield block was perfect.
func (d *disablerFake) EffectSuccessInput(_ creature.FormulaActor, _ modelskill.Definition, _ modelskill.EffectTemplate, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	d.templateLandings = append(d.templateLandings, templateLanding{bss: bss, shield: shield})
	return formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: 100, Shield: shield}, true
}

// SkillReflectInput reports a guaranteed reflect when d.reflects is set
// (ReflectChance 100 always beats a [0,100) roll), and no reflect otherwise.
func (d *disablerFake) SkillReflectInput(def modelskill.Definition) formulas.SkillReflectInput {
	if !d.reflects {
		return formulas.SkillReflectInput{}
	}
	return formulas.SkillReflectInput{CanBeReflected: true, Magic: true, ReflectChance: 100}
}

func (d *disablerFake) Attackable() bool                  { return d.attackableFlag }
func (d *disablerFake) RaidRelated() bool                 { return d.raidRelated }
func (d *disablerFake) Undead() bool                      { return d.undeadFlag }
func (d *disablerFake) ReduceAllAggroHate(amount float64) { d.aggro.ReduceAllHate(amount) }
func (d *disablerFake) StopAggroHate(attacker attackable.Combatant) {
	d.aggro.StopHate(attacker)
}
func (d *disablerFake) StopHateList(attacker attackable.Combatant) { d.hate.StopHate(attacker) }
func (d *disablerFake) ClearAggroTables() {
	d.aggro.Clear()
	d.hate.Clear()
}
func (d *disablerFake) Level() int { return d.level }

func (d *disablerFake) NotifyAggression(source attackable.Combatant, power int) {
	d.aggressionSource = source
	d.aggressionPower = power
}

// bssCasterFake exposes a fixed blessed-spiritshot charge state for tests
// asserting checkSkillSuccess resolves it from the caster.
type bssCasterFake struct {
	neutralCreature
	world.Presence
	fakeActor
	bss bool
}

func (c *bssCasterFake) BlessedSpiritshotCharged() bool { return c.bss }

// newTestHostile builds a real Monster-kind NPC, for the cases that are
// about what a handler does to an actor rather than about one narrow
// surface of it. pAtk is the one stat a physical-skill damage roll needs
// from its caster; a target can leave it at 0.
func newTestHostile(t testing.TB, id int32, pAtk float64) *npc.Hostile {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 100, disablerHostileGeo{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(idleQueue())
	h, err := npc.NewHostile(&npc.Instance{
		ObjectID: id,
		Kind:     "Monster",
		Template: &npc.Template{
			ID:              int(id),
			Type:            "Monster",
			Level:           1,
			CON:             40,
			MEN:             40,
			HPMax:           1000,
			PAtk:            pAtk,
			PDef:            1,
			MAtk:            1,
			MDef:            1,
			BaseAttackRange: 40,
			CanMove:         true,
		},
	}, live, disablerHostileMove{}, disablerHostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

type disablerHostileGeo struct{}

func (disablerHostileGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (disablerHostileGeo) Height(_, _, _ int) int16          { return 0 }
func (disablerHostileGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (disablerHostileGeo) Walkable(int, int, int) bool { return true }

func (g disablerHostileGeo) CanFly(ox, oy, oz int, _ float64, tx, ty, tz int) bool {
	return g.CanMove(ox, oy, oz, tx, ty, tz)
}

func (g disablerHostileGeo) ValidFlyLocation(ox, oy, oz int, _ float64, tx, ty, tz int) location.Location {
	return g.ValidLocation(ox, oy, oz, tx, ty, tz)
}

func (disablerHostileGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

type disablerHostileMove struct{}

func (disablerHostileMove) MaybeStartOffensiveFollow(attackable.Combatant, int) (bool, error) {
	return false, nil
}
func (disablerHostileMove) MoveHome(location.Location) error { return nil }
func (disablerHostileMove) Stop()                            {}
func (disablerHostileMove) CancelFollow()                    {}

type disablerHostileAttack struct{}

func (disablerHostileAttack) BowCoolingDown() bool                { return false }
func (disablerHostileAttack) AttackingNow() bool                  { return false }
func (disablerHostileAttack) CanAttack(attackable.Combatant) bool { return false }
func (disablerHostileAttack) DoAttack(attackable.Combatant)       {}
func (disablerHostileAttack) Stop()                               {}

type skillTarget struct {
	neutralPlayer
	world.Presence
	fakeActor
	hp, maxHP float64
	mp, maxMP float64
	cp, maxCP float64

	dead         bool
	alikeDead    bool
	invulnerable bool
	cursed       bool

	sp       int
	diedBy   any
	recharge float64

	healAmount        float64
	healMAtk          int
	healScaling       formulas.HealShotScaling
	healEffectiveness float64
	healOK            bool
	// effectsAtHeal is the effect count AddHP last saw.
	effectsAtHeal int

	physicalInput formulas.PhysicalSkillInput
	physicalOK    bool
	magicInput    formulas.MagicDamageInput
	magicOK       bool
	// magicFailures records the switch the last MagicDamageInput call saw.
	magicFailures  *bool
	skillSuccessOK bool
	// skillSuccessChance overrides SkillSuccessInput's BaseChance; nil keeps
	// the default guaranteed-success 100, a pointer to 0 forces a
	// deterministic effect-landing failure regardless of shield/rnd.
	skillSuccessChance *float64
	lastShield         formulas.ShieldDefense
	blowInput          formulas.BlowInput
	blowOK             bool
	manaInput          formulas.ManaDamageInput
	manaOK             bool
	lethalInput        formulas.LethalInput
	lethalOK           bool
	lethalPlayer       bool

	raidRelated  bool
	lethalImmune bool

	lethalOutcomes []formulas.LethalOutcome

	effects *effect.List
	shots   []item.ShotKind
	// shotFlags parallels shots with the charged flag each write carried.
	shotFlags []bool
	charged   map[item.ShotKind]bool

	castBreakDamage []float64
	// hitLog records, in order, an early cast-break roll and the HP loss
	// that follows it, with the effects the target held at that moment.
	hitLog []string

	isPlayer     bool
	name         string
	noticeKind   string
	noticeName   string
	noticeAmount int
	noticeOther  bool

	// reflects makes every reflectable skill bounce off this target.
	reflects bool
	// resistNotices records the S1_RESISTED_YOUR_S2 notices sent to this
	// actor directly rather than through a cast Result.
	resistNotices []Resisted
}

func (t *skillTarget) SkillReflectInput(modelskill.Definition) formulas.SkillReflectInput {
	if !t.reflects {
		return formulas.SkillReflectInput{}
	}
	return formulas.SkillReflectInput{CanBeReflected: true, Magic: true, ReflectChance: 100}
}

func (t *skillTarget) NotifyResistedSkill(name string, id modelskill.ID, level int) {
	t.resistNotices = append(t.resistNotices, Resisted{TargetName: name, SkillID: id, SkillLevel: level})
}

func (t *skillTarget) BreakCastOnDamage(damage float64) {
	t.castBreakDamage = append(t.castBreakDamage, damage)
	t.hitLog = append(t.hitLog, "cast break")
}

func (t *skillTarget) ReduceHPWithoutCastBreak(v float64, _ attackable.Combatant, _ modelskill.Definition) {
	t.hitLog = append(t.hitLog, fmt.Sprintf("hp -%v with %d effects", v, len(t.effects.All())))
	t.hp -= v
}

func (t *skillTarget) EffectList() *effect.List { return t.effects }

func (t *skillTarget) AlikeDead() bool { return t.dead || t.alikeDead }
func (t *skillTarget) Dead() bool      { return t.dead }

func (t *skillTarget) Invulnerable() bool { return t.invulnerable }

func (t *skillTarget) CursedWeaponEquipped() bool { return t.cursed }

func (t *skillTarget) RaidRelated() bool { return t.raidRelated }

func (t *skillTarget) Lethalable() bool { return !t.lethalImmune }

func (t *skillTarget) CanBeHealed() bool {
	return !t.dead && !t.invulnerable && !t.cursed
}

func (t *skillTarget) IsPlayer() bool { return t.isPlayer }

// Kind follows the fake's isPlayer flag, so the player-only handler paths
// resolve exactly when the test asks for a player.
func (t *skillTarget) Kind() actor.Kind {
	if t.isPlayer {
		return actor.KindPlayer
	}
	return actor.KindNPC
}

func (t *skillTarget) CharacterName() string { return t.name }
func (t *skillTarget) NotifyHPRestored(name string, amount int, other bool) {
	t.noticeKind, t.noticeName, t.noticeAmount, t.noticeOther = "hp", name, amount, other
}

func (t *skillTarget) NotifyMPRestored(name string, amount int, other bool) {
	t.noticeKind, t.noticeName, t.noticeAmount, t.noticeOther = "mp", name, amount, other
}

func (t *skillTarget) NotifyCPRestored(name string, amount int, other bool) {
	t.noticeKind, t.noticeName, t.noticeAmount, t.noticeOther = "cp", name, amount, other
}

// HealInput resolves healAmount as the power term; healMAtk and
// healScaling feed the spiritshot terms.
func (t *skillTarget) HealInput(skill modelskill.Definition) (formulas.HealInput, bool) {
	return formulas.HealInput{
		Power:   t.healAmount,
		Static:  skillTypeKey(skill.SkillType) == "HEAL_STATIC",
		MAtk:    t.healMAtk,
		Scaling: t.healScaling,
	}, t.healOK
}

func (t *skillTarget) SpiritshotCharged() bool { return t.charged[item.ShotSpirit] }

func (t *skillTarget) BlessedSpiritshotCharged() bool { return t.charged[item.ShotBlessedSpirit] }

func (t *skillTarget) HealEffectiveness() float64 {
	if t.healEffectiveness == 0 {
		return 100
	}
	return t.healEffectiveness
}

func (t *skillTarget) HP() float64         { return t.hp }
func (t *skillTarget) MaxHPValue() float64 { return t.maxHP }

func (t *skillTarget) SetHP(v float64) { t.hp = v }

func (t *skillTarget) AddHP(v float64) float64 {
	if t.effects != nil {
		t.effectsAtHeal = len(t.effects.All())
	}
	if t.hp+v > t.maxHP {
		v = t.maxHP - t.hp
	}
	if v == 0 {
		return 0
	}
	t.hp += v
	return v
}

func (t *skillTarget) MaxMPValue() float64 { return t.maxMP }
func (t *skillTarget) MPValue() float64    { return t.mp }

func (t *skillTarget) AddMP(v float64) float64 {
	if t.mp+v > t.maxMP {
		v = t.maxMP - t.mp
	}
	if v == 0 {
		return 0
	}
	t.mp += v
	return v
}

func (t *skillTarget) ReduceMP(v float64) float64 {
	if t.mp-v < 0 {
		v = t.mp
	}
	if v == 0 {
		return 0
	}
	t.mp -= v
	return v
}

func (t *skillTarget) RechargeMP(v float64) float64 { return v * t.recharge }
func (t *skillTarget) BroadcastStatus()             {}

func (t *skillTarget) CP() float64         { return t.cp }
func (t *skillTarget) MaxCPValue() float64 { return t.maxCP }
func (t *skillTarget) SetCP(v float64) {
	if v < 0 {
		v = 0
	}
	if v > t.maxCP {
		v = t.maxCP
	}
	t.cp = v
}

func (t *skillTarget) AddExpAndSp(_ int64, sp int) { t.sp += sp }

func (t *skillTarget) Kill(killer attackable.Combatant) bool {
	if t.dead {
		return false
	}
	t.dead = true
	t.diedBy = killer
	return true
}

func (t *skillTarget) ReduceHP(v float64, attacker attackable.Combatant, skill modelskill.Definition) {
	t.hp -= v
}

func (t *skillTarget) SetChargedShot(kind item.ShotKind, charged bool) {
	t.shots = append(t.shots, kind)
	t.shotFlags = append(t.shotFlags, charged)
}

func (t *skillTarget) ChargedShot(kind item.ShotKind) bool { return t.charged[kind] }

func (t *skillTarget) PhysicalSkillInput(caster creature.FormulaActor, skill modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	return t.physicalInput, t.physicalOK
}

func (t *skillTarget) MagicDamageInput(caster creature.FormulaActor, skill modelskill.Definition, magicFailures bool) (formulas.MagicDamageInput, bool) {
	t.magicFailures = &magicFailures
	return t.magicInput, t.magicOK
}

func (t *skillTarget) SkillSuccessInput(_ creature.FormulaActor, _ modelskill.Definition, _ bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	t.lastShield = shield
	chance := 100.0
	if t.skillSuccessChance != nil {
		chance = *t.skillSuccessChance
	}
	return formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: chance, Shield: shield}, t.skillSuccessOK
}

func (t *skillTarget) BlowInput(caster creature.FormulaActor, skill modelskill.Definition) (formulas.BlowInput, bool) {
	return t.blowInput, t.blowOK
}

func (t *skillTarget) ManaDamageInput(caster creature.FormulaActor, skill modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return t.manaInput, t.manaOK
}

func (t *skillTarget) LethalInput(caster creature.FormulaActor, skill modelskill.Definition) (formulas.LethalInput, bool) {
	in := t.lethalInput
	in.Chance1 = skill.LethalChance1
	in.Chance2 = skill.LethalChance2
	in.MagicLevel = skill.MagicLevel
	return in, t.lethalOK
}

func (t *skillTarget) ApplyLethalOutcome(outcome formulas.LethalOutcome, caster attackable.Combatant, skill modelskill.Definition) {
	t.lethalOutcomes = append(t.lethalOutcomes, outcome)
	switch outcome {
	case formulas.LethalFull:
		t.hp = 1
		if t.lethalPlayer {
			t.cp = 1
		}
	case formulas.LethalHalf:
		if t.lethalPlayer {
			t.cp = 1
		} else {
			t.hp -= t.hp / 2
		}
	}
}

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// playerActor marks a skillTarget as a world player, satisfying
// worldPlayerTarget so it is treated like a Player.Character for
// player-gated system messages.
type playerActor struct{ skillTarget }

func (*playerActor) Kind() actor.Kind { return actor.KindPlayer }

// resistedIconTemplate is an effect template whose landing roll always
// resists: skillTarget implements skillSuccessSource (the skill's own
// effect-success roll, controlled by skillSuccessChance) but not
// effectSuccessSource (the per-template roll inside applyEffectsWithLanding),
// so any EffectPowerSet template with an icon is force-counted as resisted
// there regardless of rnd.
var resistedIconTemplate = []modelskill.EffectTemplate{{Name: "Buff", Time: 10, EffectPowerSet: true, EffectPower: 100, Icon: true}}

func chanceOf(v float64) *float64 { return &v }

type manorFakeCaster struct {
	neutralCreature
	world.Presence
	fakeActor
	id    int32
	level int
	items map[int32]int
}

func (c *manorFakeCaster) ObjectID() int32 { return c.id }
func (c *manorFakeCaster) Level() int      { return c.level }
func (c *manorFakeCaster) AddEarnedItem(itemID int32, count int, nextID func() (int32, error)) bool {
	if _, err := nextID(); err != nil {
		return false
	}
	if c.items == nil {
		c.items = make(map[int32]int)
	}
	c.items[itemID] += count
	return true
}

func (fakeActor) Kind() actor.Kind { return actor.KindNPC }

func (*disablerFake) Kind() actor.Kind { return actor.KindNPC }

func (disablerHostileMove) CanMoveTo(location.Location) bool { return true }

func (disablerHostileMove) MoveToLocation(location.Location) (bool, error) { return false, nil }

// newTestList returns a list whose owner runs on its own inline queue, with
// the clock reading the wall time at creation.
func newTestList(owner effect.StatOwner) *effect.List {
	l := effect.NewList(owner)
	l.SetQueue(sim.NewInline(time.Now()).NewQueue("test"))
	return l
}

func idleQueue() *sim.Queue { return sim.NewInline(time.Unix(0, 0)).NewQueue("test") }

// guardedSkillTarget overrides the invulnerable/paralyzed state the damage
// feedback reads.
type guardedSkillTarget struct {
	*skillTarget
	invul, paralyzed bool
}

func (t *guardedSkillTarget) Invul() bool     { return t.invul }
func (t *guardedSkillTarget) Paralyzed() bool { return t.paralyzed }

func damageMessages(t *testing.T, messages []any) []Damage {
	t.Helper()
	var out []Damage
	for _, m := range messages {
		if d, ok := m.(Damage); ok {
			out = append(out, d)
		}
	}
	return out
}

// rolledStun is an effect template carrying its own landing roll, so landing
// it goes through the target's per-template success input.
func rolledStun() []modelskill.EffectTemplate {
	return []modelskill.EffectTemplate{{Name: "Stun", Time: 10, EffectType: "STUN", EffectPower: 80, EffectPowerSet: true}}
}
