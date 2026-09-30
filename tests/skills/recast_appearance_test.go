package skills

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// appearanceTrace reads the Target's and the Watcher's frames until both go
// quiet and returns them as ordered tokens: the Target's UserInfo abnormal
// mask ("ui:<mask>"), skill-name system messages ("sm:<msg>:<skill>") and
// icon refreshes ("icons"); the Watcher's CharInfo of the Target
// ("ci:<mask>").
func (p *appearancePair) appearanceTrace(t *testing.T) (own, seen []string) {
	t.Helper()
	for _, f := range readQuiet(t, p.tc) {
		switch f[0] {
		case serverpackets.OpcodeUserInfo:
			own = append(own, fmt.Sprintf("ui:%#x", userInfoAbnormal(t, f)))
		case serverpackets.OpcodeSystemMessage:
			r := wire.NewReader(f[1:])
			id, params := r.ReadInt32(), r.ReadInt32()
			if params == 1 && r.ReadInt32() == serverpackets.SystemMessageParamSkillName {
				own = append(own, fmt.Sprintf("sm:%d:%d", id, r.ReadInt32()))
			}
		case serverpackets.OpcodeAbnormalStatusUpdate:
			own = append(own, "icons")
		}
	}
	for _, f := range framesWithOpcode(readQuiet(t, p.wc), serverpackets.OpcodeCharInfo) {
		if id, abnormal := charInfoAbnormal(t, f); id == p.targetID {
			seen = append(seen, fmt.Sprintf("ci:%#x", abnormal))
		}
	}
	return own, seen
}

// TestIdenticalRecastRepeatsRetiredBuffExitRefresh pins #2763. Recasting an
// identical buff that is still running retires the old one inside the
// effect queue run: its exit() reaches scheduleEffect's FINISHING pass,
// which calls onExit but leaves _inUse set, and only queues its list
// removal because the queue runner is busy (AbstractEffect.java:220-227,
// 310-320; EffectList.java:465-497, 624-629). When the old buff heads a
// stack group, the stack-head change then calls setInUse(false) on it, which
// runs onExit again, before the disappeared message and the newcomer's start
// (EffectList.java:755-775; AbstractEffect.java:159-166). Its queued removal
// runs no further exit hook (EffectList.java:499-585).
//
// Each exit refresh is a player's updateAbnormalEffect, broadcastUserInfo:
// UserInfo to the player and CharInfo to its observers (Player.java:
// 4984-4987), both without the retired buff's visual. So a stacked recast
// refreshes the appearance twice with the visual cleared, then once with it
// set; an unstacked one (no stack-head change) once, then once. BigHead
// (4559) owns its exit hook; Dance of Shadows (366) clears its template's
// abnormal="stealth" from the base one. The shipped templates are used,
// with Dance of Shadows' runSpd func dropped: its stat refresh adds its own
// UserInfo/CharInfo (see TestRunSpeedBuffOnWatchedPlayerCompletes).
func TestIdenticalRecastRepeatsRetiredBuffExitRefresh(t *testing.T) {
	t.Parallel()
	gone := serverpackets.SystemMessageEffectS1Disappeared
	felt := serverpackets.SystemMessageYouFeelS1Effect
	for _, tc := range []struct {
		name      string
		id        modelskill.ID
		kind      string
		mask      int
		unstacked bool
		own, seen []string
	}{
		{
			name: "stacked BigHead", id: 4559, kind: "BigHead", mask: abnormalBigHead,
			own:  []string{"ui:0x0", "ui:0x0", fmt.Sprintf("sm:%d:4559", gone), "ui:0x2000", fmt.Sprintf("sm:%d:4559", felt), "icons"},
			seen: []string{"ci:0x0", "ci:0x0", "ci:0x2000"},
		},
		{
			name: "stacked stealth SilentMove", id: 366, kind: "SilentMove", mask: abnormalStealth,
			own:  []string{"ui:0x0", "ui:0x0", fmt.Sprintf("sm:%d:366", gone), "ui:0x100000", fmt.Sprintf("sm:%d:366", felt), "icons"},
			seen: []string{"ci:0x0", "ci:0x0", "ci:0x100000"},
		},
		{
			name: "unstacked BigHead", id: 4559, kind: "BigHead", mask: abnormalBigHead, unstacked: true,
			own:  []string{"ui:0x0", "ui:0x2000", fmt.Sprintf("sm:%d:4559", gone), "icons"},
			seen: []string{"ci:0x0", "ci:0x2000"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := shippedSkill(t, tc.id, 1)
			tmpl := def.Effects[0]
			if tmpl.Name != tc.kind || tmpl.AbnormalEffect != tc.mask || tmpl.StackType == "" {
				t.Fatalf("shipped %d effect = %s abnormal %#x stack %q, want stacked %s %#x", tc.id, tmpl.Name, tmpl.AbnormalEffect, tmpl.StackType, tc.kind, tc.mask)
			}
			tmpl.Funcs = nil
			if tc.unstacked {
				tmpl.StackType = ""
			}
			meta := effect.SkillFromDefinition(def)
			p := bootAppearancePair(t)

			p.land(t, meta, tmpl)
			p.appearanceTrace(t)

			p.land(t, meta, tmpl)
			own, seen := p.appearanceTrace(t)
			if fmt.Sprint(own) != fmt.Sprint(tc.own) {
				t.Fatalf("Target recast trace = %v, want %v", own, tc.own)
			}
			if fmt.Sprint(seen) != fmt.Sprint(tc.seen) {
				t.Fatalf("Watcher recast trace = %v, want %v", seen, tc.seen)
			}
		})
	}
}
