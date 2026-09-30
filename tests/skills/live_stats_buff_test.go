package skills

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// userInfoCombat is the combat block of a UserInfo frame (UserInfo.java:127-136).
type userInfoCombat struct {
	pAtk, pAtkSpd, pDef, evasion, accuracy, crit, mAtk, mAtkSpd, pAtkSpd2, mDef int32
}

func decodeUserInfoCombat(t *testing.T, frame []byte) userInfoCombat {
	t.Helper()
	r := userInfoCombatReader(t, frame)
	return userInfoCombat{
		pAtk: r.ReadInt32(), pAtkSpd: r.ReadInt32(), pDef: r.ReadInt32(), evasion: r.ReadInt32(),
		accuracy: r.ReadInt32(), crit: r.ReadInt32(), mAtk: r.ReadInt32(), mAtkSpd: r.ReadInt32(),
		pAtkSpd2: r.ReadInt32(), mDef: r.ReadInt32(),
	}
}

// lastUserInfoCombat decodes the last UserInfo among frames and returns its
// index too.
func lastUserInfoCombat(t *testing.T, frames [][]byte, when string) (userInfoCombat, int) {
	t.Helper()
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i][0] == serverpackets.OpcodeUserInfo {
			return decodeUserInfoCombat(t, frames[i]), i
		}
	}
	t.Fatalf("%s: no UserInfo among %d frames", when, len(frames))
	return userInfoCombat{}, -1
}

// speedStatusUpdate is one StatusUpdate frame carrying an ATK_SPD (18) or
// CAST_SPD (24) attribute.
type speedStatusUpdate struct {
	index    int
	objectID int32
	attrs    map[int32]int32
}

const (
	statusAtkSpd  = 18
	statusCastSpd = 24
)

// speedStatusUpdates returns every StatusUpdate among frames that carries
// an ATK_SPD or CAST_SPD attribute.
func speedStatusUpdates(frames [][]byte) []speedStatusUpdate {
	var out []speedStatusUpdate
	for i, frame := range frames {
		if frame[0] != serverpackets.OpcodeStatusUpdate {
			continue
		}
		r := wireReader(frame[1:])
		su := speedStatusUpdate{index: i, objectID: r.ReadInt32(), attrs: map[int32]int32{}}
		for range r.ReadInt32() {
			typ, value := r.ReadInt32(), r.ReadInt32()
			if typ == statusAtkSpd || typ == statusCastSpd {
				su.attrs[typ] = value
			}
		}
		if len(su.attrs) > 0 {
			out = append(out, su)
		}
	}
	return out
}

// statClass is the shared human-fighter class with the datapack human
// fighter attributes and base combat stats, which the shared class template
// leaves at zero.
func statClass() gameservertest.Option {
	tmpl := gameservertest.ClassTemplate()
	tmpl.STR, tmpl.CON, tmpl.DEX, tmpl.INT, tmpl.WIT, tmpl.MEN = 40, 43, 30, 21, 11, 25
	tmpl.PAtk, tmpl.PDef, tmpl.MAtk, tmpl.MDef = 4, 80, 6, 41
	return gameservertest.WithClassTemplate(tmpl)
}

func selfBuff(id int32, funcs ...modelskill.FuncTemplate) modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "BUFF",
		Effects: []modelskill.EffectTemplate{{
			Name: "Buff", Time: 2, Icon: true, StackType: "live_stats", StackOrder: 1,
			Funcs: funcs,
		}},
	}
}

// TestPAtkBuffSendsCasterUserInfoWithLivePAtk casts a Might-shaped self-buff
// (a P.Atk multiplier). Its stat func change is not RUN_SPEED, so the caster
// gets its own UserInfo (Creature.broadcastModifiedStats →
// Player.updateAndBroadcastStatus(1), Creature.java:1255-1262,
// Player.java:3975-3984) and no speed StatusUpdate, and that UserInfo's
// P.Atk field is the buffed live value (UserInfo.java:127). The buff wearing
// off sends the plain value again.
func TestPAtkBuffSendsCasterUserInfoWithLivePAtk(t *testing.T) {
	t.Parallel()
	const (
		skillID = 1068
		mul     = 1.5
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		statClass(),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			selfBuff(skillID, modelskill.FuncTemplate{Op: modelskill.FuncMul, Stat: "pAtk", Value: mul}),
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 1)
	entry := startInWorld(t, c)
	plainEntry, _ := lastUserInfoCombat(t, entry, "entry")

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, objID)
	srv.Advance(t, 700*time.Millisecond)
	landed := collectUntilQuiet(t, c)
	buffed, _ := lastUserInfoCombat(t, landed, "buff landed")
	if su := speedStatusUpdates(landed); len(su) != 0 {
		t.Fatalf("P.Atk buff sent speed StatusUpdates %+v, want none", su)
	}

	srv.Advance(t, 2200*time.Millisecond)
	srv.TickEffects()
	worn := collectUntilQuiet(t, c)
	plain, _ := lastUserInfoCombat(t, worn, "buff wore off")

	if plain.pAtk <= 1 || plain != plainEntry {
		t.Fatalf("plain combat block = %+v, want a live P.Atk above 1 matching the entry UserInfo %+v", plain, plainEntry)
	}
	// The field truncates the live value, so the buffed value lies between
	// the plain int's and the next int's product.
	lo, hi := int32(float64(plain.pAtk)*mul), int32(float64(plain.pAtk+1)*mul)
	if buffed.pAtk < lo || buffed.pAtk > hi {
		t.Fatalf("buffed UserInfo P.Atk = %d, want %d..%d (plain %d * %v)", buffed.pAtk, lo, hi, plain.pAtk, mul)
	}
	buffed.pAtk = plain.pAtk
	if buffed != plain {
		t.Fatalf("buffed combat block = %+v, want only P.Atk changed from %+v", buffed, plain)
	}
}

// TestHasteBuffBroadcastsAttackSpeedStatusUpdate casts a Haste-shaped
// self-buff (a P.Atk. speed multiplier) next to an observer. The caster gets
// its UserInfo first, then a StatusUpdate with the new ATK_SPD reaches the
// caster and the observer (Creature.java:1222-1228 and 1255-1262,
// Player.broadcastPacket); the value is the UserInfo's P.Atk. speed. The
// buff wearing off sends the plain speed the same way.
func TestHasteBuffBroadcastsAttackSpeedStatusUpdate(t *testing.T) {
	t.Parallel()
	const skillID = 1086
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Caster", 5, 0),
		gameservertest.WithWantChars(1),
		statClass(),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			selfBuff(skillID, modelskill.FuncTemplate{Op: modelskill.FuncMul, Stat: "pAtkSpd", Value: 1.33}),
		})),
	)
	caster, casterID := srv.Client, srv.SoleObjectID(t)
	srv.SeedCharacterFor(t, "player2", "Observer", 5, 0)
	observer := srv.DialClient(t, "player2", 1)
	seedKnownSkill(t, srv, casterID, skillID, 1)
	startInWorld(t, caster)
	startInWorldAmongPlayers(t, observer)
	drainUntilQuiet(t, caster)
	drainUntilQuiet(t, observer)

	caster.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, caster, casterID, skillID, 1, 500, 60_000, casterID)
	srv.Advance(t, 700*time.Millisecond)
	buffed := assertSpeedBroadcast(t, collectUntilQuiet(t, caster), collectUntilQuiet(t, observer), casterID, "buff landed")

	srv.Advance(t, 2200*time.Millisecond)
	srv.TickEffects()
	plain := assertSpeedBroadcast(t, collectUntilQuiet(t, caster), collectUntilQuiet(t, observer), casterID, "buff wore off")

	if lo, hi := int32(float64(plain)*1.33), int32(float64(plain+1)*1.33); plain <= 0 || buffed < lo || buffed > hi {
		t.Fatalf("buffed ATK_SPD = %d, want %d..%d (plain %d * 1.33)", buffed, lo, hi, plain)
	}
}

// assertSpeedBroadcast checks one Haste-shaped stat change: the caster's
// UserInfo precedes one ATK_SPD-only StatusUpdate for the caster whose value
// matches the UserInfo's P.Atk. speed, and the observer gets the same
// StatusUpdate. It returns that speed.
func assertSpeedBroadcast(t *testing.T, casterFrames, observerFrames [][]byte, casterID int32, when string) int32 {
	t.Helper()
	info, infoIndex := lastUserInfoCombat(t, casterFrames, when)
	self := speedStatusUpdates(casterFrames)
	if len(self) != 1 || self[0].objectID != casterID || len(self[0].attrs) != 1 || self[0].attrs[statusAtkSpd] != info.pAtkSpd {
		t.Fatalf("%s: caster speed StatusUpdates = %+v, want one ATK_SPD %d for %d", when, self, info.pAtkSpd, casterID)
	}
	if self[0].index < infoIndex {
		t.Fatalf("%s: caster StatusUpdate at %d before its UserInfo at %d, want UserInfo first", when, self[0].index, infoIndex)
	}
	if info.pAtkSpd2 != info.pAtkSpd {
		t.Fatalf("%s: UserInfo P.Atk. speed fields = %d and %d, want equal", when, info.pAtkSpd, info.pAtkSpd2)
	}
	seen := speedStatusUpdates(observerFrames)
	if len(seen) != 1 || seen[0].objectID != casterID || len(seen[0].attrs) != 1 || seen[0].attrs[statusAtkSpd] != info.pAtkSpd {
		t.Fatalf("%s: observer speed StatusUpdates = %+v, want one ATK_SPD %d for %d", when, seen, info.pAtkSpd, casterID)
	}
	return info.pAtkSpd
}

// TestRestoredStatBuffKeepsEntryBurst logs in a character whose saved
// effect carries P.Atk and P.Atk. speed funcs. The reference restores
// effects while loading the character, before its client is attached, so
// their stat func changes send nothing; the entry burst is the plain
// restored-buff burst, with no extra UserInfo or speed StatusUpdate, and its
// UserInfo already shows the buffed P.Atk. speed.
func TestRestoredStatBuffKeepsEntryBurst(t *testing.T) {
	t.Parallel()
	const skillID = 1086
	def := selfBuff(skillID,
		modelskill.FuncTemplate{Op: modelskill.FuncMul, Stat: "pAtk", Value: 1.5},
		modelskill.FuncTemplate{Op: modelskill.FuncMul, Stat: "pAtkSpd", Value: 1.33},
	)
	def.Effects[0].Time = 60
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 1)
	plainEntry, _ := lastUserInfoCombat(t, startInWorld(t, c), "first entry")

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, objID)
	srv.Advance(t, 700*time.Millisecond)
	buffed, _ := lastUserInfoCombat(t, collectUntilQuiet(t, c), "buff landed")
	logout(t, srv, c)

	relogin := srv.DialClient(t, "player1", 1)
	relogin.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, relogin.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, relogin.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	relogin.Send(encodeEnterWorld())
	burst := readEnterWorldBurstWithRestoredBuff(t, relogin)
	for _, frame := range collectUntilQuiet(t, relogin) {
		if frame[0] == serverpackets.OpcodeUserInfo {
			t.Fatal("restored stat buff sent a UserInfo after the entry burst")
		}
		if su := speedStatusUpdates([][]byte{frame}); len(su) != 0 {
			t.Fatalf("restored stat buff sent speed StatusUpdate %+v after the entry burst", su)
		}
	}
	restored, _ := lastUserInfoCombat(t, burst, "restored entry")
	if restored.pAtkSpd != buffed.pAtkSpd || restored.pAtk != buffed.pAtk || restored.pAtkSpd <= plainEntry.pAtkSpd {
		t.Fatalf("restored entry UserInfo = %+v, want the buffed values %+v (plain %+v)", restored, buffed, plainEntry)
	}
}
