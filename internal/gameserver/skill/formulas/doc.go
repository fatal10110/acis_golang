// Package formulas computes the dice-roll and damage outcomes combat
// resolves once a creature's attack/defense stats (see skill/funcs) are
// known: hit/miss, critical/blow success, shield block, physical/magic
// damage, and attack timing.
//
// Every function here takes its inputs as already-resolved numbers (attack
// power, defense, elemental/positional/race/pvp multipliers, …) rather than
// a creature and target — deriving those numbers from a live creature's
// gear, buffs and target is the calculation chain's job (see skill/funcs
// and skill.Calculator), not this package's. This keeps the fidelity-
// critical arithmetic testable in isolation against oracle-generated
// values, and defers wiring it to real actors until that actor exists.
package formulas
