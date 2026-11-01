package castlemanor

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestApprovalMatchesReferenceProbe replays "approve <crops> <warehouse>
// <treasury> => treasury <n> next <lists> sent <messages> mode <mode> delay
// <ms>": a modifiable manor approved at 06:00. The next period sells 10 Dark
// Coda seeds at 5 and buys the crops start/price, alternating Dark Coda and
// Red Coda. The owner's warehouse contents vary in the probe but change
// nothing there: the approval charges no castle, keeps the next period as
// set and tells no one, whatever the warehouse holds.
func TestApprovalMatchesReferenceProbe(t *testing.T) {
	t.Parallel()
	for _, l := range manorGolden(t, "approve") {
		f := strings.Fields(l[0])
		treasury := int64(must(strconv.Atoi(f[2])))
		store := &memStore{
			production: []ProductionRow{{CastleID: gludio, SeedID: codaSeed, Amount: 10, StartAmount: 10, Price: 5, NextPeriod: true}},
		}
		for i, c := range strings.Split(f[0], "+") {
			v := strings.Split(c, "/")
			crop := codaCrop
			if i%2 == 1 {
				crop = redCrop
			}
			n := int32(must(strconv.Atoi(v[0])))
			store.procure = append(store.procure, ProcureRow{
				CastleID: gludio, CropID: crop, StartAmount: n, Amount: n,
				Price: int32(must(strconv.Atoi(v[1]))), RewardType: 1, NextPeriod: true,
			})
		}
		h := start(t, at(5, 0), treasury, store, 0)
		if s := h.m.Status(); s != StatusModifiable {
			t.Fatalf("manor at 05:00 is %s, want %s", s, StatusModifiable)
		}
		h.clock.Advance(time.Hour)

		next := ""
		for _, s := range h.m.SeedProduction(gludio, true) {
			next += fmt.Sprintf("%d:%d,", s.ID, s.Amount())
		}
		for _, c := range h.m.CropProcure(gludio, true) {
			next += fmt.Sprintf("%d:%d,", c.ID, c.Amount())
		}
		if next == "" {
			next = "empty"
		}
		sent := "-"
		if effects := h.effects.take(); len(effects) > 0 {
			sent = strings.Join(effects, ",")
		}
		gl, _ := h.castles.Get(gludio)
		got := fmt.Sprintf("treasury %d next %s sent %s mode %s delay %d", gl.Treasury(), next, sent, h.m.Status(), h.m.NextModeChange().Sub(h.clock.Now()).Milliseconds())
		if got != l[1] {
			t.Errorf("approval of %s, warehouse %s, treasury %d:\n got %s\nwant %s", f[0], f[1], treasury, got, l[1])
		}
		if _, _, saves := h.store.state(); saves != 0 {
			t.Errorf("approval of %s saved the manor %d times, want 0", f[0], saves)
		}
	}
}
