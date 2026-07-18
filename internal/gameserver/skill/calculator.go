// Package skill contains skill rules and effects.
package skill

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"

// Calculator dynamically computes stat funcs. It is an alias kept at the
// package boundary for callers that already import skill.
type Calculator = basefunc.Calculator
