package skills

import (
	"encoding/binary"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: an effect hook's appearance refresh is
// Creature.updateAbnormalEffect(), which for a player is broadcastUserInfo():
// UserInfo to the player, CharInfo to every known player (Player.java:
// 4984-4987, 2244-2266). The buff-icon refresh (AbnormalStatusUpdate) is sent
// once, when the effect queue run ends (EffectList.java:465-497). The base
// start/exit hooks set and clear the template's abnormal visual
// (AbstractEffect.java:247-262) for every kind that does not replace them.

// Abnormal visual bits (enums/skills/AbnormalEffect.java).
const (
	abnormalBleeding      = 0x000001
	abnormalPoison        = 0x000002
	abnormalBigHead       = 0x002000
	abnormalFlame         = 0x004000
	abnormalChangeTexture = 0x008000
	abnormalFloatRoot     = 0x020000
	abnormalDanceStun     = 0x040000
	abnormalStealth       = 0x100000
	abnormalImprison1     = 0x200000
	abnormalImprison2     = 0x400000
)

const appearanceSkillID = 4100

// appearanceHolder is the live player surface an effect lands on.
type appearanceHolder interface {
	effect.Actor
	EffectList() *effect.List
	Queue() *sim.Queue
}

// appearancePair is a Target and a Watcher in the world together.
type appearancePair struct {
	srv                 *gameservertest.Server
	tc, wc              *testsupport.ScriptedClient
	targetID, watcherID int32
	target              appearanceHolder
}

func bootAppearancePair(t *testing.T) *appearancePair {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Target", 5, 0),
		gameservertest.WithWantChars(1),
	)
	tc, targetID := srv.Client, srv.SoleObjectID(t)
	watcher := srv.SeedCharacterFor(t, "watcher", "Watcher", 5, 0)
	wc := srv.DialClient(t, "watcher", 1)
	startInWorldAmongPlayers(t, wc)
	startInWorldAmongPlayers(t, tc)
	drainUntilQuiet(t, wc)
	drainUntilQuiet(t, tc)

	obj, ok := srv.State.Player(targetID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", targetID)
	}
	target, ok := obj.(appearanceHolder)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an effect holder", targetID, obj)
	}
	return &appearancePair{srv: srv, tc: tc, wc: wc, targetID: targetID, watcherID: watcher.ID, target: target}
}

// land adds a real effect built from tmpl to the Target on its own queue,
// self-applied the way a landed skill's effect is.
func (p *appearancePair) land(t *testing.T, meta effect.Skill, tmpl modelskill.EffectTemplate) *effect.Effect {
	t.Helper()
	e, err := effect.New(meta, tmpl)
	if err != nil {
		t.Fatalf("effect.New(%s): %v", tmpl.Name, err)
	}
	e.Effector, e.Effected = p.target, p.target
	p.onQueue(t, func() { p.target.EffectList().Add(e) })
	return e
}

// remove drops e from the Target's effect list on its own queue.
func (p *appearancePair) remove(t *testing.T, e *effect.Effect) {
	t.Helper()
	p.onQueue(t, func() { p.target.EffectList().Remove(e) })
}

func (p *appearancePair) onQueue(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !p.target.Queue().Post(func() { fn(); close(done) }) {
		t.Fatal("post to target queue: queue closed")
	}
	<-done
}

func (p *appearancePair) holds(e *effect.Effect) bool {
	for _, held := range p.target.EffectList().All() {
		if held == e {
			return true
		}
	}
	return false
}

// expire runs the one-second effect sweep until e has left the Target's
// effect list.
func (p *appearancePair) expire(t *testing.T, e *effect.Effect, what string) {
	t.Helper()
	for i := 0; p.holds(e); i++ {
		if i == 10 {
			t.Fatalf("%s not observed within 10 effect ticks", what)
		}
		p.srv.Advance(t, 1100*time.Millisecond)
		p.srv.TickEffects()
	}
}

// readQuiet returns every frame c receives until it stays quiet.
func readQuiet(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 100 reads")
	return nil
}

func framesWithOpcode(frames [][]byte, opcode byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == opcode {
			out = append(out, f)
		}
	}
	return out
}

func opcodeIndex(frames [][]byte, opcode byte) int {
	for i, f := range frames {
		if f[0] == opcode {
			return i
		}
	}
	return -1
}

// userInfoAbnormal reads UserInfo's abnormal-effect field. Everything after
// it is fixed width (UserInfo.java:189-212): 73 bytes.
func userInfoAbnormal(t *testing.T, frame []byte) int {
	t.Helper()
	if frame[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("frame opcode %#x, want UserInfo", frame[0])
	}
	return int(binary.LittleEndian.Uint32(frame[len(frame)-77:]))
}

// charInfoAbnormal reads CharInfo's object id and abnormal-effect field.
// Everything after the field is fixed width (CharInfo.java:144-163): 60
// bytes.
func charInfoAbnormal(t *testing.T, frame []byte) (objectID int32, abnormal int) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("frame opcode %#x, want CharInfo", frame[0])
	}
	objectID = int32(binary.LittleEndian.Uint32(frame[17:21]))
	return objectID, int(binary.LittleEndian.Uint32(frame[len(frame)-64:]))
}

// refreshOrder is where the hook's UserInfo lands against the effect list's
// icon refresh. A start runs its hook inside the queue run, ahead of the
// run's closing icon refresh (EffectList.java:465-497). An ending effect
// leaves the list first, which runs the queue and its icon refresh, and
// only then runs its exit hook (AbstractEffect.java:308-321).
type refreshOrder bool

const (
	userInfoFirst refreshOrder = true
	iconsFirst    refreshOrder = false
)

// assertAppearanceRefresh checks one effect add or removal on the Target:
// one UserInfo carrying wantMask and one AbnormalStatusUpdate (the effect
// list's icon refresh) to the Target, in order, and one CharInfo of the
// Target carrying wantMask to the Watcher.
func (p *appearancePair) assertAppearanceRefresh(t *testing.T, what string, wantMask int, order refreshOrder) {
	t.Helper()
	own := readQuiet(t, p.tc)
	infos := framesWithOpcode(own, serverpackets.OpcodeUserInfo)
	if len(infos) != 1 {
		t.Fatalf("%s: Target got %d UserInfo frames, want 1 (opcodes % x)", what, len(infos), opcodeList(own))
	}
	if got := userInfoAbnormal(t, infos[0]); got != wantMask {
		t.Fatalf("%s: UserInfo abnormal = %#x, want %#x", what, got, wantMask)
	}
	icons := framesWithOpcode(own, serverpackets.OpcodeAbnormalStatusUpdate)
	if len(icons) != 1 {
		t.Fatalf("%s: Target got %d AbnormalStatusUpdate frames, want 1 (opcodes % x)", what, len(icons), opcodeList(own))
	}
	if ui, asu := opcodeIndex(own, serverpackets.OpcodeUserInfo), opcodeIndex(own, serverpackets.OpcodeAbnormalStatusUpdate); (ui < asu) != bool(order) {
		t.Fatalf("%s: UserInfo at %d, AbnormalStatusUpdate at %d, want UserInfo first = %v (opcodes % x)", what, ui, asu, bool(order), opcodeList(own))
	}

	seen := readQuiet(t, p.wc)
	var chars [][]byte
	for _, f := range framesWithOpcode(seen, serverpackets.OpcodeCharInfo) {
		if id, _ := charInfoAbnormal(t, f); id == p.targetID {
			chars = append(chars, f)
		}
	}
	if len(chars) != 1 {
		t.Fatalf("%s: Watcher got %d CharInfo frames of the Target, want 1 (opcodes % x)", what, len(chars), opcodeList(seen))
	}
	if _, got := charInfoAbnormal(t, chars[0]); got != wantMask {
		t.Fatalf("%s: CharInfo abnormal = %#x, want %#x", what, got, wantMask)
	}
	if framesWithOpcode(seen, serverpackets.OpcodeAbnormalStatusUpdate) != nil {
		t.Fatalf("%s: Watcher got an AbnormalStatusUpdate (opcodes % x)", what, opcodeList(seen))
	}
}

func opcodeList(frames [][]byte) []byte {
	ops := make([]byte, len(frames))
	for i, f := range frames {
		ops[i] = f[0]
	}
	return ops
}

// TestPlayerStunRefreshesAppearanceNotIcons pins #2842: a stun landing on a
// player and wearing off each send UserInfo to the player and CharInfo to an
// observer from the hook, and only the effect list's single icon refresh
// sends an AbnormalStatusUpdate: after the UserInfo on start, before it on
// expiry.
func TestPlayerStunRefreshesAppearanceNotIcons(t *testing.T) {
	t.Parallel()
	p := bootAppearancePair(t)

	stun := p.land(t, effect.Skill{ID: appearanceSkillID, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: "Stun", Count: 1, Time: 1, Icon: true})
	p.assertAppearanceRefresh(t, "stun start", 0, userInfoFirst)

	p.expire(t, stun, "stun wearing off")
	p.assertAppearanceRefresh(t, "stun exit", 0, iconsFirst)
}

// TestPlayerBigHeadRefreshesAppearanceOnce pins #2842's second half: BigHead
// starting and stopping on a player sends UserInfo/CharInfo once per call
// with the bit set and then cleared, and no extra AbnormalStatusUpdate.
func TestPlayerBigHeadRefreshesAppearanceOnce(t *testing.T) {
	t.Parallel()
	p := bootAppearancePair(t)

	bigHead := p.land(t, effect.Skill{ID: appearanceSkillID, Level: 1},
		modelskill.EffectTemplate{Name: "BigHead", Count: 1, Time: 60, Icon: true})
	p.assertAppearanceRefresh(t, "big head start", abnormalBigHead, userInfoFirst)

	p.remove(t, bigHead)
	p.assertAppearanceRefresh(t, "big head stop", 0, iconsFirst)
}

// TestPlayerTemplateAbnormalVisual pins #2841 on shipped templates: a
// DamOverTime with abnormal="poison" (4243 Venomous Poison) and a SilentMove
// with abnormal="stealth" (366 Dance of Shadows) set their visual in the
// player's UserInfo and its observers' CharInfo on start and clear it when
// the effect ends. The shipped counts and periods are shortened so the
// effect wears off within the test clock; the abnormal attribute is the
// loaded one.
func TestPlayerTemplateAbnormalVisual(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		id    modelskill.ID
		kind  string
		count int
		want  int
	}{
		{"poison DamOverTime", 4243, "DamOverTime", 2, abnormalPoison},
		{"stealth SilentMove", 366, "SilentMove", 1, abnormalStealth},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := shippedSkill(t, tc.id, 1)
			tmpl := def.Effects[0]
			if tmpl.Name != tc.kind || tmpl.AbnormalEffect != tc.want {
				t.Fatalf("shipped %d effect = %s abnormal %#x, want %s %#x", tc.id, tmpl.Name, tmpl.AbnormalEffect, tc.kind, tc.want)
			}
			tmpl.Count, tmpl.Time, tmpl.Value = tc.count, 1, 1
			// Dance of Shadows' runSpd func would hit #2854 (a run-speed
			// change on a watched player deadlocks its queue); the visual
			// does not depend on it.
			tmpl.Funcs = nil
			p := bootAppearancePair(t)

			e := p.land(t, effect.SkillFromDefinition(def), tmpl)
			p.assertAppearanceRefresh(t, "start", tc.want, userInfoFirst)

			p.expire(t, e, tc.name+" wearing off")
			p.assertAppearanceRefresh(t, "exit", 0, iconsFirst)
		})
	}
}

// TestHandlerOwnedHooksIgnoreTemplateAbnormal pins the kinds whose start and
// exit replace the base hooks: Stun ignores abnormal="dancestun" (5012), so
// its refresh carries no dance-stun bit.
func TestHandlerOwnedHooksIgnoreTemplateAbnormal(t *testing.T) {
	t.Parallel()
	def := shippedSkill(t, 5012, 1)
	tmpl := def.Effects[0]
	if tmpl.Name != "Stun" || tmpl.AbnormalEffect != abnormalDanceStun {
		t.Fatalf("shipped 5012 effect = %s abnormal %#x, want Stun dancestun", tmpl.Name, tmpl.AbnormalEffect)
	}
	tmpl.Count, tmpl.Time = 1, 1
	p := bootAppearancePair(t)

	p.land(t, effect.SkillFromDefinition(def), tmpl)
	p.assertAppearanceRefresh(t, "dance stun start", 0, userInfoFirst)
}

// TestShippedAbnormalNamesResolveToClientMasks is the loader oracle: every
// abnormal name the datapack uses resolves to the reference mask, including
// the per-level table reference of 5098 Capture Penalty.
func TestShippedAbnormalNamesResolveToClientMasks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id    modelskill.ID
		level int
		kind  string
		want  int
	}{
		{4243, 1, "DamOverTime", abnormalPoison},
		{3005, 1, "DamOverTime", abnormalBleeding},
		{1244, 1, "DamOverTime", abnormalFlame},
		{366, 1, "SilentMove", abnormalStealth},
		{4223, 1, "Buff", abnormalChangeTexture},
		{5016, 1, "Paralyze", abnormalFloatRoot},
		{5012, 1, "Stun", abnormalDanceStun},
		{4559, 1, "BigHead", abnormalBigHead},
		{5098, 1, "Debuff", abnormalImprison1},
		{5098, 2, "Debuff", abnormalImprison2},
	} {
		def := shippedSkill(t, tc.id, tc.level)
		var found bool
		for _, e := range def.Effects {
			if e.AbnormalEffect == 0 {
				continue
			}
			if found || e.Name != tc.kind || e.AbnormalEffect != tc.want {
				t.Fatalf("skill %d-%d effect %s abnormal = %#x, want one %s with %#x", tc.id, tc.level, e.Name, e.AbnormalEffect, tc.kind, tc.want)
			}
			found = true
		}
		if !found {
			t.Fatalf("skill %d-%d has no effect with an abnormal visual, want %s %#x", tc.id, tc.level, tc.kind, tc.want)
		}
	}
}

// TestRestoredAbnormalVisualIsSilentUntilEnterWorldUserInfo pins the relog
// path: the reference restores saved effects while loading the character,
// before it has a client or observers (GameClient.loadCharFromDisk ->
// Player.restore, RequestGameStart.java:45-49), so their start hooks'
// appearance refresh sends nothing. The EnterWorld burst keeps its one
// restored-icon frame, gains no extra UserInfo, and its UserInfo carries the
// restored visual.
func TestRestoredAbnormalVisualIsSilentUntilEnterWorldUserInfo(t *testing.T) {
	t.Parallel()
	def := modelskill.Definition{
		ID: appearanceSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "BUFF",
		Effects: []modelskill.EffectTemplate{{
			Name: "Buff", Count: 1, Time: 600, Icon: true, AbnormalEffect: abnormalChangeTexture,
		}},
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	holder, ok := obj.(appearanceHolder)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an effect holder", objID, obj)
	}
	e, err := effect.New(effect.SkillFromDefinition(def), def.Effects[0])
	if err != nil {
		t.Fatalf("effect.New: %v", err)
	}
	e.Effector, e.Effected = holder, holder
	done := make(chan struct{})
	if !holder.Queue().Post(func() { holder.EffectList().Add(e); close(done) }) {
		t.Fatal("post to player queue: queue closed")
	}
	<-done
	drainUntilQuiet(t, c)
	logout(t, srv, c)

	relogin := srv.DialClient(t, "player1", 1)
	relogin.Send(encodeRequestGameStart(0))
	if reply := relogin.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	if reply := relogin.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", reply[0])
	}
	relogin.Send(encodeEnterWorld())
	frames := readEnterWorldBurstWithRestoredBuff(t, relogin)
	if got := userInfoAbnormal(t, frames[10]); got != abnormalChangeTexture {
		t.Fatalf("EnterWorld UserInfo abnormal = %#x, want %#x", got, abnormalChangeTexture)
	}
}
