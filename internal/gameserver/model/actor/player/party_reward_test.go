package player

import (
	"slices"
	"testing"
)

// The expected values below come from a standalone Java 21 probe that
// copies, operation for operation, the reference's party kill arithmetic:
// Monster.calculateExpAndSp and the party branch of
// Monster.calculateRewards (partyMul, the (long)/(int) narrowing) for the
// pool, and Party.distributeXpAndSp (cutoff methods, BONUS_EXP_SP, party
// rates, Math.round of the squared-level share, servitor penalty in float)
// for the shares. Run in eclipse-temurin:21-jdk-alpine; none of the values
// were derived from the Go code under test.

func TestPartyKillPool(t *testing.T) {
	tests := []struct {
		name                     string
		expReward, spReward      float64
		partyDamage, totalDamage float64
		levelDiff                int
		wantExp                  int64
		wantSp                   int
	}{
		{"party dealt all the damage", 5000, 250, 1000, 1000, 0, 5000, 250},
		// The share is applied twice: 5000*0.6 = 3000, then *0.6 again.
		{"party dealt 60%", 5000, 250, 600, 1000, 0, 1800, 90},
		{"party level past the falloff", 5000, 250, 600, 1000, 8, 1041, 51},
		{"uneven share under the falloff", 12345, 678, 137, 400, 3, 1448, 79},
		{"no party damage", 5000, 250, 0, 1000, 0, 0, 0},
		{"int32 ceiling reward", 2147483647, 250, 999, 1000, 0, 2143190826, 248},
		{"fractional damage one past the falloff", 73123, 4567, 333.5, 1000.25, 6, 6774, 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp, sp := PartyKillPool(tt.expReward, tt.spReward, tt.partyDamage, tt.totalDamage, tt.levelDiff)
			if exp != tt.wantExp || sp != tt.wantSp {
				t.Fatalf("PartyKillPool = (%d, %d), want (%d, %d)", exp, sp, tt.wantExp, tt.wantSp)
			}
		})
	}
}

func TestPartyXPRulesShares(t *testing.T) {
	level20 := PartyXPRules{Cutoff: PartyXPCutoffLevel, CutoffLevel: 20, CutoffPercent: 3, RateXP: 1, RateSP: 1}
	with := func(r PartyXPRules, f func(*PartyXPRules)) PartyXPRules { f(&r); return r }
	members := func(levels []int, penalties ...float32) []PartyRewardMember {
		out := make([]PartyRewardMember, len(levels))
		for i, l := range levels {
			out[i].Level = l
			if i < len(penalties) {
				out[i].ServitorPenalty = penalties[i]
			}
		}
		return out
	}
	twelve := make([]int, 12)
	for i := range twelve {
		twelve[i] = 70 - i
	}

	tests := []struct {
		name    string
		rules   PartyXPRules
		xp      int64
		sp      int
		top     int
		members []PartyRewardMember
		want    []PartyShare
	}{
		{
			"level: two members", level20, 5000, 250, 40, members([]int{40, 30}),
			[]PartyShare{{4160, 208, true}, {2340, 117, true}},
		},
		{
			"level: exactly at and one past the cutoff", level20, 5000, 250, 40, members([]int{40, 20, 19}),
			[]PartyShare{{5200, 260, true}, {1300, 65, true}, {0, 0, false}},
		},
		{
			"level: full party with servitors, one cut", level20, 7777, 333, 60,
			members([]int{60, 55, 52, 48, 44, 41, 40, 39, 61}, 0, 0.1, 0, 0, 0, 0.05),
			[]PartyShare{{2273, 97, true}, {1719, 73, true}, {1707, 73, true}, {1455, 62, true}, {1222, 52, true}, {1008, 43, true}, {1010, 43, true}, {0, 0, false}, {2349, 100, true}},
		},
		{
			"level: party rates", with(level20, func(r *PartyXPRules) { r.RateXP, r.RateSP = 2.5, 1.5 }), 5000, 250, 40, members([]int{40, 38, 35}),
			[]PartyShare{{6512, 195, true}, {5877, 176, true}, {4986, 149, true}},
		},
		{
			"percentage: low members cut", with(level20, func(r *PartyXPRules) { r.Cutoff = PartyXPCutoffPercentage }), 5000, 250, 70, members([]int{70, 10, 12}),
			[]PartyShare{{5000, 250, true}, {0, 0, false}, {0, 0, false}},
		},
		{
			"percentage: one member over the line", with(level20, func(r *PartyXPRules) { r.Cutoff = PartyXPCutoffPercentage }), 5000, 250, 70, members([]int{70, 13, 12}),
			[]PartyShare{{6283, 314, true}, {217, 10, true}, {0, 0, false}},
		},
		{
			"auto: three members", with(level20, func(r *PartyXPRules) { r.Cutoff = PartyXPCutoffAuto }), 5000, 250, 70, members([]int{70, 30, 40}),
			[]PartyShare{{4601, 229, true}, {845, 42, true}, {1502, 75, true}},
		},
		{
			"auto: lone member with a servitor", with(level20, func(r *PartyXPRules) { r.Cutoff = PartyXPCutoffAuto }), 9999, 999, 50, members([]int{50}, 0.3),
			[]PartyShare{{6999, 699, true}},
		},
		{
			"none: everyone shares", with(level20, func(r *PartyXPRules) { r.Cutoff = PartyXPCutoffNone }), 5000, 250, 70, members([]int{70, 1}),
			[]PartyShare{{6499, 324, true}, {1, 0, true}},
		},
		{
			"unknown method: nobody qualifies", with(level20, func(r *PartyXPRules) { r.Cutoff = PartyXPCutoffUnknown }), 5000, 250, 70, members([]int{70, 60}),
			[]PartyShare{{0, 0, false}, {0, 0, false}},
		},
		{
			"command channel: bonus stops at nine", level20, 123456, 7890, 70, members(twelve),
			[]PartyShare{{20661, 1320, true}, {20075, 1282, true}, {19498, 1246, true}, {18928, 1209, true}, {18368, 1173, true}, {17815, 1138, true}, {17271, 1103, true}, {16736, 1069, true}, {16209, 1035, true}, {15690, 1002, true}, {15180, 970, true}, {14678, 938, true}},
		},
		{
			"tiny pool rounds each share", level20, 3, 1, 40, members([]int{40, 39, 38}),
			[]PartyShare{{1, 0, true}, {1, 0, true}, {1, 0, true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rules.Shares(tt.xp, tt.sp, tt.top, tt.members); !slices.Equal(got, tt.want) {
				t.Fatalf("Shares = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParsePartyXPCutoff(t *testing.T) {
	for in, want := range map[string]PartyXPCutoff{
		"level": PartyXPCutoffLevel, "LEVEL": PartyXPCutoffLevel, "Percentage": PartyXPCutoffPercentage,
		"auto": PartyXPCutoffAuto, "none": PartyXPCutoffNone, "": PartyXPCutoffUnknown, "levels": PartyXPCutoffUnknown,
	} {
		if got := ParsePartyXPCutoff(in); got != want {
			t.Errorf("ParsePartyXPCutoff(%q) = %d, want %d", in, got, want)
		}
	}
}
