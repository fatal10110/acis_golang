package relation

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestListOrderMatchesReferenceProbe replays testdata/order.scenarios on a
// Manager and compares every listed friend and block list, in order, with
// testdata/order.golden: the output of testdata/oracle/RelationOrderProbe.java
// over the same scenarios (see testdata/oracle/README.md). The scenarios hold
// ids sharing a bucket of the list's set in either order, rows loaded at
// boot, the store's growth on a crowded bin (on an insert and on an update),
// tree bins of the store and of the set, their splits, and a server's worth
// of random relations.
func TestListOrderMatchesReferenceProbe(t *testing.T) {
	scenarios, err := os.ReadFile("testdata/order.scenarios")
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile("testdata/order.golden")
	if err != nil {
		t.Fatal(err)
	}
	got := replayScenarios(t, string(scenarios))
	want := strings.Split(strings.TrimSuffix(string(golden), "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("replay listed %d lines, golden %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d:\n got %s\nwant %s", i+1, got[i], want[i])
		}
	}
}

// replayScenarios runs the scenario lines (format in testdata/oracle/README.md)
// and returns the lines a list prints. A scenario's load rows build its
// Manager before its first other line.
func replayScenarios(t *testing.T, text string) []string {
	t.Helper()
	var (
		out  []string
		name string
		rows []Row
		m    *Manager
	)
	manager := func() *Manager {
		if m == nil {
			m = NewManager(rows)
		}
		return m
	}
	for n, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		num := func(i int) int32 {
			v, err := strconv.ParseInt(f[i], 10, 32)
			if err != nil {
				t.Fatalf("line %d: %v", n+1, err)
			}
			return int32(v)
		}
		switch f[0] {
		case "scenario":
			name, rows, m = f[1], nil, nil
			continue
		case "list":
			id := num(1)
			out = append(out,
				fmt.Sprintf("%s friends %d:%s", name, id, joinIDs(manager().FriendIDs(id))),
				fmt.Sprintf("%s blocks %d:%s", name, id, joinIDs(manager().BlockedIDs(id))))
			continue
		}
		a, b, arg := num(1), num(2), 3
		var flags int32
		if f[0] == "load" {
			flags = num(arg)
			arg++
		}
		stepA, stepB, count := int32(0), int32(0), int32(1)
		if len(f) > arg {
			stepA, stepB, count = num(arg), num(arg+1), num(arg+2)
		}
		for i := range count {
			x, y := a+i*stepA, b+i*stepB
			switch f[0] {
			case "load":
				if m != nil {
					t.Fatalf("line %d: load after the scenario's manager was built", n+1)
				}
				rows = append(rows, Row{CharID: x, FriendID: y, Relation: flags})
			case "friend":
				manager().AddFriend(x, y)
			case "unfriend":
				manager().RemoveFriend(x, y)
			case "block":
				manager().Block(x, y)
			case "unblock":
				manager().Unblock(x, y)
			default:
				t.Fatalf("line %d: unknown op %q", n+1, f[0])
			}
		}
	}
	return out
}

func joinIDs(ids []int32) string {
	var sb strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&sb, " %d", id)
	}
	return sb.String()
}
