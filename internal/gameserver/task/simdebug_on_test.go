//go:build simdebug

package task

// simdebugBuild reports that queue drains carry the simdebug owner
// bookkeeping, whose per-task allocations the production budgets exclude.
const simdebugBuild = true
