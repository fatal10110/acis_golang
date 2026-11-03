//go:build simdebug

package sim

// simdebugBuild reports that queue drains carry the simdebug owner
// bookkeeping, whose per-drain allocations the production budgets exclude.
const simdebugBuild = true
