package olympiad

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The strip scenario's skills and items: Alpha's clan holds Clan Vitality,
// Alpha is under Might and uses soulshots on its own; the match buffs are
// Wind Walk 2 for all and Haste 1 for a fighter.
const (
	stripClanID       = 600
	stripClanVitality = 370
	stripMight        = 1068
	stripSoulshot     = 1835
	matchWindWalk     = 1204
	matchHaste        = 1086
	humanMystic       = 10
)

// stripMatch boots a match of Alpha, a human fighter leading a level 5 clan
// that holds Clan Vitality, against Bravo, a human mystic, with the shipped
// skills.
func stripMatch(t *testing.T, adjust func(*olympiad.Config)) *match {
	t.Helper()
	mystic := gameservertest.ClassTemplate()
	mystic.ID = humanMystic
	opts := append(skillOptions(t),
		gameservertest.WithClassTemplates(mystic),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, q := range []struct {
				query string
				args  []any
			}{
				{`UPDATE characters SET clanid = ?, power_grade = 0 WHERE char_name = 'Alpha'`, []any{stripClanID}},
				{`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id)
					SELECT ?, 'Strip', 5, 1000, obj_Id FROM characters WHERE char_name = 'Alpha'`, []any{stripClanID}},
				{`INSERT INTO clan_skills (clan_id, skill_id, skill_level) VALUES (?, ?, 1)`, []any{stripClanID, stripClanVitality}},
			} {
				if _, err := db.Exec(q.query, q.args...); err != nil {
					t.Fatalf("%s: %v", q.query, err)
				}
			}
		}),
	)
	return bootMatchPrepared(t, adjust, func(srv *gameservertest.Server) {
		if _, err := srv.DB.Exec(`UPDATE characters SET classid = ?, base_class = ? WHERE char_name = 'Bravo'`, humanMystic, humanMystic); err != nil {
			t.Fatalf("make Bravo a mystic: %v", err)
		}
	}, opts...)
}

// hasEffect reports whether p is under an effect of skill id.
func (m *match) hasEffect(t *testing.T, p *competitor, id int) bool {
	t.Helper()
	var ok bool
	m.onCharacter(t, p, func(pc *player.Character) { _, ok = pc.EffectList().ActiveBySkillID(id) })
	return ok
}

// modeAt is the index of the first ExOlympiadMode switching to mode among
// frames, -1 when none.
func modeAt(frames [][]byte, mode int) int {
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeExtended {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadUint16() == serverpackets.OpcodeExOlympiadMode && int(r.ReadUint8()) == mode {
			return i
		}
	}
	return -1
}

// skillListsFrom are the skills of each SkillList among frames from index
// from on.
func skillListsFrom(t *testing.T, frames [][]byte, from int) []map[int32]int32 {
	t.Helper()
	var out []map[int32]int32
	for _, f := range frames[from:] {
		if f[0] == serverpackets.OpcodeSkillList {
			out = append(out, skillListLevels(t, f))
		}
	}
	return out
}

// TestOlympiadMatchStripsBuffsAndRestores pins what a match does to its
// competitors besides moving them (AbstractOlympiadGame.removals,
// buffPlayer, cleanEffects, playerStatusBack): on entry Alpha loses its
// effects, its clan's skills, its party and its automatic soulshots, each
// cancelled with its notice, then gets its skill list; at the countdown's
// 20 the fighter gets Wind Walk and Haste, the mystic Wind Walk only; at
// the cleanup Alpha's client leaves Olympiad mode, its match buffs end and
// its clan's skills come back in its next skill list.
func TestOlympiadMatchStripsBuffsAndRestores(t *testing.T) {
	t.Parallel()
	m := stripMatch(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 5_000 })
	a, b := m.ones[0], m.ones[1]
	id := m.srv.SeedCharacterFor(t, "charlie", "Charlie", 40, 0).ID
	mate := &competitor{c: m.srv.DialClient(t, "charlie", 1), id: id, name: "Charlie"}
	startInWorld(t, mate.c)
	a.c.Send(encodeJoinParty("Charlie", 0))
	m.srv.ReadQueued(t, mate.c)
	mate.c.Send(encodeAnswerJoinParty(1))
	m.srv.ReadQueued(t, mate.c)
	m.read(t)
	might, ok := shippedSkills.defs.Definition(modelskill.Ref{ID: stripMight, Level: 1})
	if !ok {
		t.Fatal("no Might 1 among the shipped skills")
	}
	m.onCharacter(t, a, func(pc *player.Character) {
		skillhandler.LandEffects(pc, pc, might)
		pc.SetAutoSoulShot(stripSoulshot, true)
	})
	if !m.hasEffect(t, a, stripMight) {
		t.Fatal("Alpha is not under Might before the match")
	}
	if got := m.srv.PlayerSkillLevel(t, a.id, stripClanVitality); got != 1 {
		t.Fatalf("Alpha's Clan Vitality before the match = %d, want 1", got)
	}
	m.ones = append(m.ones, mate)
	for _, p := range m.ones {
		p.frames = nil
	}

	m.runUntil(t, "Alpha in the stadium", 5*time.Minute, func() bool { return modeAt(a.frames, 1) >= 0 })
	m.srv.Advance(t, 50*time.Millisecond)
	m.read(t)
	entry := modeAt(a.frames, 1)
	if m.hasEffect(t, a, stripMight) {
		t.Error("Alpha kept Might in the stadium")
	}
	if got := m.srv.PlayerSkillLevel(t, a.id, stripClanVitality); got != 0 {
		t.Errorf("Alpha's Clan Vitality in the stadium = %d, want none", got)
	}
	if !slices.ContainsFunc(a.frames[entry:], func(f []byte) bool { return f[0] == serverpackets.OpcodePartySmallWindowDeleteAll }) {
		t.Error("Alpha's party window was not closed as it entered the stadium")
	}
	if !slices.ContainsFunc(mate.frames, func(f []byte) bool { return f[0] == serverpackets.OpcodePartySmallWindowDeleteAll }) {
		t.Error("Charlie's party window was not closed as Alpha was expelled")
	}
	shot := slices.IndexFunc(a.frames[entry:], func(f []byte) bool {
		r := wire.NewReader(f[1:])
		return f[0] == serverpackets.OpcodeExtended && r.ReadUint16() == serverpackets.OpcodeExAutoSoulShot && r.ReadInt32() == stripSoulshot && r.ReadInt32() == 0
	})
	if shot < 0 {
		t.Fatal("Alpha's automatic soulshot was not switched off")
	}
	if msgs := sysMsgs(t, a.frames[entry+shot+1:entry+shot+2]); !slices.Equal(msgs[0], []string{"1434", "1835"}) {
		t.Errorf("after the soulshot switched off Alpha was told %v, want 1434 naming 1835", msgs)
	}
	lists := skillListsFrom(t, a.frames, entry)
	if len(lists) == 0 {
		t.Fatal("Alpha got no skill list as it entered the stadium")
	}
	if _, ok := lists[len(lists)-1][stripClanVitality]; ok {
		t.Error("Alpha's skill list in the stadium still shows Clan Vitality")
	}

	m.fighting(t)
	for _, tc := range []struct {
		p     *competitor
		haste bool
	}{{a, true}, {b, false}} {
		if !m.hasEffect(t, tc.p, matchWindWalk) {
			t.Errorf("%s fights without Wind Walk", tc.p.name)
		}
		if got := m.hasEffect(t, tc.p, matchHaste); got != tc.haste {
			t.Errorf("%s under Haste = %v, want %v", tc.p.name, got, tc.haste)
		}
	}

	m.over(t)
	back := modeAt(a.frames, 0)
	if back < 0 {
		t.Fatal("Alpha's client did not leave Olympiad mode")
	}
	lists = skillListsFrom(t, a.frames, back)
	if len(lists) == 0 {
		t.Fatal("Alpha got no skill list after the match")
	}
	if level := lists[0][stripClanVitality]; level != 1 {
		t.Errorf("Alpha's skill list after the match shows Clan Vitality at %d, want 1", level)
	}
	if m.hasEffect(t, a, matchWindWalk) {
		t.Error("Alpha kept the match's Wind Walk after it")
	}
}

// TestOlympiadMatchWonOnSkillDamage pins damage dealt by a skill counting
// toward the match (the damage hook of a cast's result): Alpha strikes
// Bravo once with Wind Strike, the fight runs out of time with both
// standing, and Alpha wins it on damage.
func TestOlympiadMatchWonOnSkillDamage(t *testing.T) {
	t.Parallel()
	const windStrike = 1177
	// Bodies hardy enough to take a Wind Strike standing.
	hardy := gameservertest.ClassTemplate()
	hardy.HPTable, hardy.CPTable = []float64{8000}, []float64{8000}
	opts := append(skillOptions(t), gameservertest.WithMagicFailures(false), gameservertest.WithClassTemplate(hardy))
	m := bootMatchPrepared(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 10_000 }, func(srv *gameservertest.Server) {
		if _, err := srv.DB.Exec(`INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index)
			SELECT obj_Id, ?, 1, 0 FROM characters WHERE char_name = 'Alpha'`, windStrike); err != nil {
			t.Fatalf("teach Alpha Wind Strike: %v", err)
		}
	}, opts...)
	a, b := m.ones[0], m.ones[1]

	m.shown(t)
	a.c.Send(encodeAttackRequest(b.id))
	m.read(t)
	a.c.Send(encodeMagicSkillUse(windStrike))
	m.runEvery(t, "Alpha's Wind Strike", 50*time.Millisecond, 10*time.Second, a.dealt)
	m.over(t)
	if m.srv.PlayerDead(t, b.id) {
		t.Fatal("Bravo died; the fight should end on time")
	}
	for _, f := range a.frames {
		if f[0] == serverpackets.OpcodeAttack {
			t.Fatal("Alpha attacked Bravo; only its Wind Strike should count")
		}
	}

	if got := matchFights(t, m); len(got) != 1 || got[0].winner != 1 {
		t.Fatalf("olympiad_fights = %+v, want one won by side 1", got)
	}
	if n, _ := m.srv.Olympiad.Noble(a.id); n.CompWon != 1 {
		t.Errorf("Alpha record = %+v, want one match won", n)
	}
}

func encodeJoinParty(name string, loot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	w.WriteString(name)
	w.WriteInt32(loot)
	return w.Bytes()
}

func encodeAnswerJoinParty(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	w.WriteInt32(response)
	return w.Bytes()
}

func encodeMagicSkillUse(skillID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}
