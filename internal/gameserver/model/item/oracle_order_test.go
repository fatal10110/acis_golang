package item

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// goldenRows parses "<id>x<count>,..." into drops that always roll their
// count.
func goldenRows(t *testing.T, line int, text string) []Drop {
	t.Helper()
	var drops []Drop
	for part := range strings.SplitSeq(text, ",") {
		id, count, ok := strings.Cut(part, "x")
		itemID, err1 := strconv.Atoi(id)
		qty, err2 := strconv.Atoi(count)
		if !ok || err1 != nil || err2 != nil {
			t.Fatalf("golden line %d: bad row %q", line, part)
		}
		drops = append(drops, Drop{ItemID: int32(itemID), Min: int32(qty), Max: int32(qty), Chance: 100})
	}
	return drops
}

// goldenList renders items as the golden's "<id>x<count>,..." text.
func goldenList[T any](items []T, pair func(T) (int32, int32)) string {
	if len(items) == 0 {
		return "-"
	}
	parts := make([]string, len(items))
	for i, it := range items {
		id, count := pair(it)
		parts[i] = fmt.Sprintf("%dx%d", id, count)
	}
	return strings.Join(parts, ",")
}

// TestDropOrderMatchesReferenceProbe replays testdata/order.golden, the
// output of testdata/oracle/OrderProbe.java: the order of a spoil category's
// merged result for a drop rate, and the order a sweep reads the items of a
// kill's spoil categories in.
func TestDropOrderMatchesReferenceProbe(t *testing.T) {
	data, err := os.ReadFile("testdata/order.golden")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for i, text := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line := i + 1
		fields := strings.Fields(text)
		counts[fields[0]]++
		switch fields[0] {
		case "merge":
			if len(fields) != 5 {
				t.Fatalf("malformed golden line %d %q", line, text)
			}
			rate, err := strconv.Atoi(fields[2])
			if err != nil {
				t.Fatalf("golden line %d: %v", line, err)
			}
			cat := DropCategory{Kind: DropSpoil, Chance: 100, Drops: goldenRows(t, line, fields[3])}
			got := goldenList(cat.rollOrdered(1, float64(rate)), func(d rolledDrop) (int32, int32) { return d.ItemID, d.Count })
			if got != fields[4] {
				t.Errorf("merge %s (rate %d): order = %s, want %s", fields[1], rate, got, fields[4])
			}
		case "sweep":
			if len(fields) != 4 {
				t.Fatalf("malformed golden line %d %q", line, text)
			}
			var categories []DropCategory
			for rows := range strings.SplitSeq(fields[2], ";") {
				categories = append(categories, DropCategory{Kind: DropSpoil, Chance: 100, Drops: goldenRows(t, line, rows)})
			}
			var pool SpoilPool
			pool.Mark(1)
			RollKillReward(categories, &pool, 1, false, Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}, false)
			got := goldenList(pool.Sweep(), func(s SpoilItem) (int32, int32) { return s.ItemID, s.Count })
			if got != fields[3] {
				t.Errorf("sweep %s: order = %s, want %s", fields[1], got, fields[3])
			}
		default:
			t.Fatalf("golden line %d: unknown kind %q", line, fields[0])
		}
	}
	if counts["merge"] == 0 || counts["sweep"] == 0 {
		t.Fatalf("golden holds %v, want merge and sweep lines", counts)
	}
}
