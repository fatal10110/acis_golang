package network

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestInboundOpcodesMatchReferenceHandler replays testdata/opcodes.golden,
// the output of testdata/oracle/OpcodeProbe.java: the packet class the
// reference GamePacketHandler builds for each first opcode in each client
// state, and for each second opcode behind 0xd0. The opcodes the state gate
// lets through, and the second opcodes the extended table dispatches, must be
// exactly those sets.
func TestInboundOpcodesMatchReferenceHandler(t *testing.T) {
	data, err := os.ReadFile("testdata/opcodes.golden")
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]State{"CONNECTED": StateConnected, "AUTHED": StateAuthed, "ENTERING": StateEntering, "IN_GAME": StateInGame}
	first := map[State][]int{}
	second := map[State][]int{}
	var noWarehouse string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Fatalf("malformed golden line %q", line)
		}
		if fields[0] == "IN_GAME-no-warehouse" {
			noWarehouse = fields[1] + " " + fields[2]
			continue
		}
		state, ok := states[fields[0]]
		if !ok {
			t.Fatalf("unknown state in golden line %q", line)
		}
		if id, isSecond := strings.CutPrefix(fields[1], "d0:"); isSecond {
			second[state] = append(second[state], parseHexOpcode(t, id))
			continue
		}
		first[state] = append(first[state], parseHexOpcode(t, fields[1]))
	}
	if noWarehouse != "31 none" {
		t.Fatalf("golden does not record the warehouse deposit off: %q", noWarehouse)
	}

	for name, state := range states {
		var got []int
		for op := range 256 {
			if Allowed(state, byte(op)) && op != 0xd0 {
				got = append(got, op)
			}
		}
		if want := first[state]; !slices.Equal(got, want) {
			t.Errorf("%s: first opcodes the state gate allows differ from the reference\n  only here: %s\n  only in the reference: %s", name, hexList(minus(got, want), 2), hexList(minus(want, got), 2))
		}
	}

	var gotEx []int
	for id := range gameExInbound {
		gotEx = append(gotEx, int(id))
	}
	slices.Sort(gotEx)
	if want := second[StateInGame]; !slices.Equal(gotEx, want) {
		t.Errorf("in-game second opcodes differ from the reference\n  only here: %s\n  only in the reference: %s", hexList(minus(gotEx, want), 4), hexList(minus(want, gotEx), 4))
	}
	if want := second[StateEntering]; !slices.Equal(want, []int{0x08}) {
		t.Errorf("reference entering second opcodes = %v, want only the manor list 0x08", want)
	}
	if !Allowed(StateEntering, 0xd0) || !Allowed(StateInGame, 0xd0) || Allowed(StateConnected, 0xd0) || Allowed(StateAuthed, 0xd0) {
		t.Error("0xd0 must be allowed while entering and in game, and in no earlier state")
	}
}

func parseHexOpcode(t *testing.T, s string) int {
	t.Helper()
	v, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	return int(v)
}

// minus returns the members of a that b lacks; both are ascending.
func minus(a, b []int) []int {
	var out []int
	for _, v := range a {
		if _, found := slices.BinarySearch(b, v); !found {
			out = append(out, v)
		}
	}
	return out
}

func hexList(ids []int, width int) string {
	if len(ids) == 0 {
		return "none"
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%0*x", width, id)
	}
	return strings.Join(parts, " ")
}
