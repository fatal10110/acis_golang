package conditions

import (
	"runtime"
	"sync"
	"weak"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// A parsed condition is immutable once loaded, so its compiled tree is built
// on the first test and kept beside it, keyed by the parsed value's address.
// The key is weak: a template dropped by a reload takes its compiled tree
// with it.
var (
	clauseCache   compiledCache[modelskill.ConditionClause]
	nodeCache     compiledCache[modelskill.Condition]
	itemUseCache  compiledCache[item.UseCondition]
	itemNodeCache compiledCache[item.Condition]
)

// CompiledClause is Compile(c.Root), built once per clause. A nil c holds
// no condition.
func CompiledClause(c *modelskill.ConditionClause) (Condition, error) {
	return clauseCache.get(c, func(c *modelskill.ConditionClause) (Condition, error) { return Compile(c.Root) })
}

// CompiledNode is Compile(*c), built once per node. A nil c holds no
// condition.
func CompiledNode(c *modelskill.Condition) (Condition, error) {
	return nodeCache.get(c, func(c *modelskill.Condition) (Condition, error) { return Compile(*c) })
}

// CompiledItemUse is an item condition block's root compiled, built once per
// block. A nil c holds no condition.
func CompiledItemUse(c *item.UseCondition) (Condition, error) {
	return itemUseCache.get(c, func(c *item.UseCondition) (Condition, error) { return Compile(ItemCondition(c.Root)) })
}

// CompiledItemNode is an item condition node compiled, built once per node.
// A nil c holds no condition.
func CompiledItemNode(c *item.Condition) (Condition, error) {
	return itemNodeCache.get(c, func(c *item.Condition) (Condition, error) { return Compile(ItemCondition(*c)) })
}

type compiledResult struct {
	cond Condition
	err  error
}

// compiledCache maps weak.Pointer[T] to the compiledResult of the T it
// points at. It is safe for concurrent use: two first tests of one value
// may both compile it, to equal trees, and the first one stored is kept.
type compiledCache[T any] struct{ m sync.Map }

func (c *compiledCache[T]) get(node *T, compile func(*T) (Condition, error)) (Condition, error) {
	if node == nil {
		return nil, nil
	}
	key := weak.Make(node)
	if v, ok := c.m.Load(key); ok {
		r := v.(compiledResult)
		return r.cond, r.err
	}
	cond, err := compile(node)
	if _, loaded := c.m.LoadOrStore(key, compiledResult{cond: cond, err: err}); !loaded {
		runtime.AddCleanup(node, func(key weak.Pointer[T]) { c.m.Delete(key) }, key)
	}
	return cond, err
}
