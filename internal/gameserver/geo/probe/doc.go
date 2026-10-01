// Package probe generates and evaluates geodata queries (height, canMove,
// line-of-sight, path) against the geo engine, rendering each as a
// datadiff.Record so its answers can be diffed against a captured dump of
// expected answers.
package probe
