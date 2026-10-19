package maker

import (
	"fmt"
	"math"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// memoInt reads the 32-bit integer the npcmaker's memo declares under
// key. A missing, non-numeric or out-of-range value panics: the hook stops
// there.
func memoInt(g spawn.Group, key string) int {
	v := g.Maker().AIParams[key]
	n, ok := parseInt32(v)
	if !ok {
		panic(fmt.Sprintf("maker: memo %s = %q is not an integer", key, v))
	}
	return int(n)
}

// parseInt32 parses s as a decimal 32-bit integer.
func parseInt32(s string) (int32, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < math.MinInt32 || n > math.MaxInt32 {
		return 0, false
	}
	return int32(n), true
}

// memoIntOr is memoInt with def for a key the memo does not declare.
func memoIntOr(g spawn.Group, key string, def int) int {
	if _, ok := g.Maker().AIParams[key]; !ok {
		return def
	}
	return memoInt(g, key)
}

// intMemo is an integer a maker writes in its memo under key: reading it
// before the first write panics.
type intMemo struct {
	cell script.MapCell[string, int]
	key  string
}

func (c *intMemo) get() int {
	v, ok := c.cell.Get(c.key)
	if !ok {
		panic(fmt.Sprintf("maker: memo %s read before the maker wrote it", c.key))
	}
	return v
}

func (c *intMemo) set(v int) { c.cell.Set(c.key, v) }
