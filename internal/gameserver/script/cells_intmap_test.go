package script

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// The walk order of IntMapCell is the reference's hash map's. Each
// scenario is a list of operations; its want is what the reference's
// java.util.HashMap<Integer, Integer> printed for every "keys" step (JDK 21,
// eclipse-temurin:21-jdk-alpine): "new N" is HashMap.newHashMap(N), "new -"
// new HashMap<>(), "put" puts each key in turn, "del" removes each key.
func TestIntMapCellWalksInReferenceHashMapOrder(t *testing.T) {
	for _, tc := range []struct {
		name, ops, want string
	}{
		{
			name: "six clans, then a seventh",
			ops:  "new 6; put 268435461 268435459 268435470 268435457 268435465 268435466; keys; del 268435470; put 268435471; keys; put 268435480; keys",
			want: "[268435457,268435465,268435466,268435459,268435461,268435470] [268435457,268435465,268435466,268435459,268435461,268435471] [268435457,268435459,268435461,268435480,268435465,268435466,268435471]",
		},
		{
			name: "a ninth key in one bucket doubles a small table",
			ops:  "new -; put 0 16 32 48 64 80 96 112 128; keys; put 144; keys",
			want: "[0,32,64,96,128,16,48,80,112] [0,32,64,96,128,16,48,80,112,144]",
		},
		{
			name: "negative keys, folded high bits, removal and clear",
			ops:  "new -; put 1 17 33 65 -1 -16 2147483647 -2147483648 65536 131072 196608 65537; keys; del 33 -1; put 33; keys; clear; put 5 3 1; keys",
			want: "[-1,2147483647,-2147483648,65537,1,17,33,65,65536,131072,196608,-16] [2147483647,-2147483648,65537,1,17,65,65536,33,131072,196608,-16] [1,3,5]",
		},
		{
			name: "a map sized for no mapping",
			ops:  "new 0; put 7; keys; put 9 11; keys; put 100 200 300 400 500; keys",
			want: "[7] [9,7,11] [400,100,500,7,200,9,11,300]",
		},
		{
			name: "growth past the expected size",
			ops:  "new 6; put 100000 200000 300000 400000 500000 600000 700000; keys; del 100000 700000; keys; put 100000; keys",
			want: "[100000,200000,300000,400000,500000,600000,700000] [200000,300000,400000,500000,600000] [100000,200000,300000,400000,500000,600000]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runIntMapScenario(t, tc.ops); got != tc.want {
				t.Errorf("walks\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

func runIntMapScenario(t *testing.T, ops string) string {
	t.Helper()
	var (
		c   *IntMapCell[int]
		out []string
	)
	for op := range strings.SplitSeq(ops, ";") {
		f := strings.Fields(op)
		keys := make([]int32, 0, len(f))
		for _, s := range f[1:] {
			if s == "-" {
				continue
			}
			k, err := strconv.ParseInt(s, 10, 32)
			if err != nil {
				t.Fatalf("%q: %v", op, err)
			}
			keys = append(keys, int32(k))
		}
		switch f[0] {
		case "new":
			if len(keys) == 0 {
				c = &IntMapCell[int]{}
			} else {
				c = NewIntMapCell[int](int(keys[0]))
			}
		case "put":
			for i, k := range keys {
				c.Set(k, i+1)
			}
		case "del":
			for _, k := range keys {
				c.Delete(k)
			}
		case "clear":
			c.Clear()
		case "keys":
			parts := make([]string, 0, c.Len())
			for _, k := range c.Keys() {
				parts = append(parts, strconv.Itoa(int(k)))
			}
			out = append(out, "["+strings.Join(parts, ",")+"]")
		default:
			t.Fatalf("unknown operation %q", op)
		}
	}
	return strings.Join(out, " ")
}

// A present key keeps its place (17 and 1 share a bucket of the 8-bucket
// table), values and entries walk as the keys do, and a walk is a snapshot
// the script may change the cell under.
func TestIntMapCellReadsAndSnapshots(t *testing.T) {
	c := NewIntMapCell[string](6)
	if _, ok := c.Get(1); ok || c.Has(1) || c.Len() != 0 {
		t.Fatalf("empty cell holds 1 or has a length")
	}
	c.Set(17, "a")
	c.Set(1, "b")
	c.Set(17, "c")
	if v, ok := c.Get(17); !ok || v != "c" || !c.Has(17) {
		t.Fatalf("Get(17) = %q, %v; want c, true", v, ok)
	}
	if got := fmt.Sprint(c.Keys(), c.Values(), c.Entries()); got != "[17 1] [c b] [{17 c} {1 b}]" {
		t.Fatalf("walks = %s", got)
	}
	for _, e := range c.Entries() {
		c.Delete(e.Key)
		c.Set(e.Key+100, e.Value)
	}
	if got := fmt.Sprint(c.Keys(), c.Len()); got != "[117 101] 2" {
		t.Fatalf("after a walk that changed the cell: %s", got)
	}
	if v, ok := c.Delete(5); ok || v != "" {
		t.Fatalf("Delete(5) = %q, %v; want none", v, ok)
	}
	if v, ok := c.Delete(101); !ok || v != "b" {
		t.Fatalf("Delete(101) = %q, %v; want b, true", v, ok)
	}
}
