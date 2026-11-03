package statbonus

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestBonusTablesMatchReferenceProbe compares every row of every table with
// testdata/statbonus.golden, the output of testdata/oracle/StatBonusProbe.java
// (the reference static initializer copied unchanged; see the README there).
func TestBonusTablesMatchReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/statbonus.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	tables := []*[MaxStatValue]float64{&STRBonus, &CONBonus, &DEXBonus, &INTBonus, &WITBonus, &MENBonus, &BaseEvasionAccuracy}
	names := []string{"STRBonus", "CONBonus", "DEXBonus", "INTBonus", "WITBonus", "MENBonus", "BaseEvasionAccuracy"}
	rows := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 1+len(tables) {
			t.Fatalf("malformed golden line %q", line)
		}
		i, err := strconv.Atoi(fields[0])
		if err != nil {
			t.Fatal(err)
		}
		for c, table := range tables {
			want, err := strconv.ParseFloat(fields[1+c], 64)
			if err != nil {
				t.Fatal(err)
			}
			if got := table[i]; got != want {
				t.Errorf("%s[%d] = %v, want %v", names[c], i, got, want)
			}
		}
		rows++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if rows != MaxStatValue {
		t.Fatalf("golden has %d rows, want %d", rows, MaxStatValue)
	}
}
