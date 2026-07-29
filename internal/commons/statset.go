// Package commons holds small typed data structures shared across the
// server that don't belong to any one game system.
package commons

import (
	"errors"
	"fmt"
	"strings"
)

// ErrValueRequired is returned by a mandatory StatSet accessor when key is
// absent or its stored value cannot be coerced to the requested type.
var ErrValueRequired = errors.New("commons: value required")

// StatSet is a typed attribute bag: values are stored as heterogeneous
// key/value pairs and read back through typed accessors that coerce between
// the stored representation and the requested type (e.g. a value stored as
// the string "1;2;3" can be read back via GetIntArray).
//
// The plain accessors (GetInt, GetString, ...) return ErrValueRequired if
// key is absent or the stored value cannot be coerced to the requested
// type, so a caller can decide how to handle a mandatory key it can't read
// instead of getting a zero value masquerading as real data. Callers that
// can tolerate a missing value use the *Default variant instead — but a
// *present* value that fails to parse is still an error there: an optional
// attribute being absent is normal, a mangled number in a data file is
// corruption and must not silently become the default.
//
// That "present but malformed is still an error" rule applies to every
// accessor whose target type has a genuine parse failure mode: the numeric
// accessors (GetByte, GetInt, GetInt32, GetInt64, GetFloat64, GetFloat32 and
// their array/Default variants) reject a present string that doesn't parse
// as that number. GetBool, GetString, GetStringArray and their Default
// variants have no such failure mode by design: a string coerces to bool
// by a deliberately forgiving rule (case-insensitive "true" is true,
// anything else — "false", "yes", garbage — is false, never an error), and
// GetString/GetStringArray format or split whatever is present rather than
// validating it against a grammar. The shipped datapack's boolean
// attributes are exclusively "true"/"false" today, but a value outside
// that set must keep silently reading as false to match the required
// behavior, not start erroring.
//
// Accessors are added alongside the types they return: there are currently
// no accessors for composite domain types (e.g. a game position), because
// those types don't exist in this package yet.
type StatSet struct {
	values map[string]any
}

// NewStatSet creates an empty StatSet.
func NewStatSet() *StatSet {
	return &StatSet{values: make(map[string]any)}
}

// NewStatSetWithCapacity creates an empty StatSet, pre-sizing the backing
// map to hold size entries without reallocation.
func NewStatSetWithCapacity(size int) *StatSet {
	return &StatSet{values: make(map[string]any, size)}
}

// NewStatSetFrom creates a StatSet as a shallow copy of set.
func NewStatSetFrom(set *StatSet) *StatSet {
	values := make(map[string]any, len(set.values))
	for k, v := range set.values {
		values[k] = v
	}
	return &StatSet{values: values}
}

// Set stores value under key, overwriting any previous value.
func (s *StatSet) Set(key string, value any) {
	s.values[key] = value
}

// Unset removes key.
func (s *StatSet) Unset(key string) {
	delete(s.values, key)
}

// Has reports whether key is present.
func (s *StatSet) Has(key string) bool {
	_, ok := s.values[key]
	return ok
}

// errValueRequired reports that key was read through a mandatory accessor
// but held no value coercible to wanted.
func errValueRequired(key, wanted string, val any) error {
	return fmt.Errorf("commons: StatSet key %q requires a %s value, got %v: %w", key, wanted, val, ErrValueRequired)
}

// coerceBool coerces val to a bool from a bool, string ("true",
// case-insensitive), or numeric (nonzero) representation. ok reports whether
// val was a recognized kind; bool has no present-but-malformed case.
func coerceBool(val any) (v bool, ok bool) {
	if b, ok := val.(bool); ok {
		return b, true
	}
	if str, ok := val.(string); ok {
		return strings.EqualFold(str, "true"), true
	}
	if n, ok := asNumber(val); ok {
		return n != 0, true
	}
	return false, false
}

// GetBool returns the value at key as a bool, coercing a string ("true",
// case-insensitive) or numeric (nonzero) representation. Returns
// ErrValueRequired if key is absent or the value cannot be coerced.
func (s *StatSet) GetBool(key string) (bool, error) {
	val := s.values[key]
	v, ok := coerceBool(val)
	if !ok {
		return false, errValueRequired(key, "bool", val)
	}
	return v, nil
}

// GetBoolDefault is like GetBool but returns defaultValue instead of an
// error when key is absent or cannot be coerced.
func (s *StatSet) GetBoolDefault(key string, defaultValue bool) bool {
	val := s.values[key]
	v, ok := coerceBool(val)
	if !ok {
		return defaultValue
	}
	return v
}

// asNumber coerces any of the numeric representations StatSet stores
// (signed/unsigned integers of any width, float32, float64) into a float64,
// so a single coercion path serves every float-targeted accessor (GetBool,
// GetFloat64, GetFloat32) regardless of how the value was originally stored.
// Integer-targeted accessors use asInt64 instead, since routing an int64
