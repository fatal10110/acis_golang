package xml

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// attrValues is a sticky-error reader over one element's attribute values
// after table substitution, keyed by attribute name. Every value is the raw
// string the data file carried, so each accessor parses as well as reads:
// it records the first failure internally instead of returning it, letting a
// builder read a long attribute list and check Err once at the end. Once an
// error is recorded, every later accessor is a no-op returning its default.
//
// An absent optional attribute takes the caller's default; a present one
// that fails to parse is an error, since a mangled number in a data file is
// corruption and must not silently read as the default.
type attrValues struct {
	vals   map[string]string
	prefix string
	err    error
}

// newAttrValues wraps vals for sticky-error reading. prefix identifies the
// element being built and is prepended to the first error recorded.
func newAttrValues(vals map[string]string, prefix string) *attrValues {
	return &attrValues{vals: vals, prefix: prefix}
}

// Err reports the first error recorded by any accessor call, if any.
func (a *attrValues) Err() error { return a.err }

// fail records err, prefixed, as a's first error if none is already set.
func (a *attrValues) fail(err error) {
	if a.err == nil {
		a.err = fmt.Errorf("%s: %w", a.prefix, err)
	}
}

// has reports whether key is present, regardless of any recorded error.
func (a *attrValues) has(key string) bool {
	_, ok := a.vals[key]
	return ok
}

// str returns the value at key, recording an error if key is absent.
func (a *attrValues) str(key string) string {
	if a.err != nil {
		return ""
	}
	v, ok := a.vals[key]
	if !ok {
		a.fail(fmt.Errorf("attribute %q is required", key))
		return ""
	}
	return v
}

// strDefault returns the value at key, or def if key is absent.
func (a *attrValues) strDefault(key, def string) string {
	if a.err != nil {
		return def
	}
	if v, ok := a.vals[key]; ok {
		return v
	}
	return def
}

// int returns the value at key as a base-10 int, recording an error if key
// is absent or its value fails to parse.
func (a *attrValues) int(key string) int {
	if a.err != nil {
		return 0
	}
	raw, ok := a.vals[key]
	if !ok {
		a.fail(fmt.Errorf("attribute %q is required", key))
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		a.fail(fmt.Errorf("attribute %q: %w", key, err))
		return 0
	}
	return n
}

// int32 returns the value at key as an int32, recording an error if key is
// absent, its value fails to parse, or it overflows int32.
func (a *attrValues) int32(key string) int32 {
	n := a.int(key)
	if n < math.MinInt32 || n > math.MaxInt32 {
		a.fail(fmt.Errorf("attribute %q: value %d overflows int32", key, n))
		return 0
	}
	return int32(n)
}

// intArray returns the value at key as the ";"-separated list of ints its
// raw text spells, recording an error if key is absent or any element fails
// to parse.
func (a *attrValues) intArray(key string) []int {
	raw := a.str(key)
	if a.err != nil {
		return nil
	}
	parts := strings.Split(raw, ";")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			a.fail(fmt.Errorf("attribute %q: %w", key, err))
			return nil
		}
		out[i] = n
	}
	return out
}

// boolDefault returns the value at key as a bool, or def if key is absent.
// Any spelling other than a case-insensitive "true" reads as false rather
// than an error: the shipped data files write booleans as "true"/"false",
// and a value outside that set is not treated as corruption.
func (a *attrValues) boolDefault(key string, def bool) bool {
	if a.err != nil {
		return def
	}
	raw, ok := a.vals[key]
	if !ok {
		return def
	}
	return strings.EqualFold(raw, "true")
}

// intDefault returns the value at key as a base-10 int, or def if key is
// absent.
func (a *attrValues) intDefault(key string, def int) int {
	if a.err != nil {
		return def
	}
	raw, ok := a.vals[key]
	if !ok {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		a.fail(fmt.Errorf("attribute %q: %w", key, err))
		return def
	}
	return n
}

// int32Default returns the value at key as an int32, or def if key is
// absent. A present-but-malformed or overflowing value is still an error.
func (a *attrValues) int32Default(key string, def int32) int32 {
	n := a.intDefault(key, int(def))
	if n < math.MinInt32 || n > math.MaxInt32 {
		a.fail(fmt.Errorf("attribute %q: value %d overflows int32", key, n))
		return def
	}
	return int32(n)
}

// int32LiteralDefault returns the value at key as an int32, or def if key is
// absent. The value is parsed as an integer literal, so a base prefix such
// as 0x is accepted.
func (a *attrValues) int32LiteralDefault(key string, def int32) int32 {
	if a.err != nil {
		return def
	}
	raw, ok := a.vals[key]
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(raw, 0, 32)
	if err != nil {
		a.fail(fmt.Errorf("attribute %q: %w", key, err))
		return def
	}
	return int32(n)
}

// float32Default returns the value at key as a float32, or def if key is
// absent.
func (a *attrValues) float32Default(key string, def float32) float32 {
	if a.err != nil {
		return def
	}
	raw, ok := a.vals[key]
	if !ok {
		return def
	}
	n, err := strconv.ParseFloat(raw, 32)
	if err != nil {
		a.fail(fmt.Errorf("attribute %q: %w", key, err))
		return def
	}
	return float32(n)
}

// float64 returns the value at key as a float64, recording an error if key
// is absent.
func (a *attrValues) float64(key string) float64 {
	if a.err != nil {
		return 0
	}
	raw, ok := a.vals[key]
	if !ok {
		a.fail(fmt.Errorf("attribute %q is required", key))
		return 0
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		a.fail(fmt.Errorf("attribute %q: %w", key, err))
		return 0
	}
	return n
}

// float64Default returns the value at key as a float64, or def if key is
// absent.
func (a *attrValues) float64Default(key string, def float64) float64 {
	if a.err != nil {
		return def
	}
	if _, ok := a.vals[key]; !ok {
		return def
	}
	return a.float64(key)
}

// attrEnum returns the value at key parsed by parse, recording an error if
// key is absent or its value matches no known name.
func attrEnum[E any](a *attrValues, key string, parse func(string) (E, error)) E {
	var zero E
	if a.err != nil {
		return zero
	}
	raw, ok := a.vals[key]
	if !ok {
		a.fail(fmt.Errorf("attribute %q is required", key))
		return zero
	}
	e, err := parse(raw)
	if err != nil {
		a.fail(err)
		return zero
	}
	return e
}

// attrEnumDefault is like attrEnum but returns def when key is absent. A
// present value matching no known name is still an error.
func attrEnumDefault[E any](a *attrValues, key string, parse func(string) (E, error), def E) E {
	if a.err != nil {
		return def
	}
	if _, ok := a.vals[key]; !ok {
		return def
	}
	return attrEnum(a, key, parse)
}
