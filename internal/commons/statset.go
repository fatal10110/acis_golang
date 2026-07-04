// Package commons holds small typed data structures shared across the
// server that don't belong to any one game system.
package commons

import (
	"errors"
	"fmt"
	"strconv"
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
// can tolerate a missing or malformed value use the *Default variant
// instead.
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

// GetBool returns the value at key as a bool, coercing a string ("true",
// case-insensitive) or numeric (nonzero) representation. Returns
// ErrValueRequired if key is absent or the value cannot be coerced.
func (s *StatSet) GetBool(key string) (bool, error) {
	val := s.values[key]
	if b, ok := val.(bool); ok {
		return b, nil
	}
	if str, ok := val.(string); ok {
		return strings.EqualFold(str, "true"), nil
	}
	if n, ok := asNumber(val); ok {
		return n != 0, nil
	}
	return false, errValueRequired(key, "bool", val)
}

// GetBoolDefault is like GetBool but returns defaultValue instead of an
// error when key is absent or cannot be coerced.
func (s *StatSet) GetBoolDefault(key string, defaultValue bool) bool {
	val := s.values[key]
	if b, ok := val.(bool); ok {
		return b
	}
	if str, ok := val.(string); ok {
		return strings.EqualFold(str, "true")
	}
	if n, ok := asNumber(val); ok {
		return n != 0
	}
	return defaultValue
}

// asNumber coerces any of the numeric representations StatSet stores
// (signed/unsigned integers of any width, float32, float64) into a float64,
// so a single coercion path serves every numeric accessor regardless of how
// the value was originally stored.
func asNumber(val any) (float64, bool) {
	switch v := val.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	}
	return 0, false
}

// GetByte returns the value at key as a byte, coercing a numeric or numeric
// string representation. Returns ErrValueRequired if key is absent or the
// value cannot be coerced.
func (s *StatSet) GetByte(key string) (byte, error) {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return byte(int64(n)), nil
	}
	if str, ok := val.(string); ok {
		n, err := strconv.ParseInt(str, 10, 8)
		if err != nil {
			return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
		}
		return byte(n), nil
	}
	return 0, errValueRequired(key, "byte", val)
}

// GetByteDefault is like GetByte but returns defaultValue instead of an
// error when key is absent or cannot be coerced.
func (s *StatSet) GetByteDefault(key string, defaultValue byte) byte {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return byte(int64(n))
	}
	if str, ok := val.(string); ok {
		if n, err := strconv.ParseInt(str, 10, 8); err == nil {
			return byte(n)
		}
	}
	return defaultValue
}

// GetDouble returns the value at key as a float64, coercing a numeric,
// numeric string, or bool (1/0) representation. Returns ErrValueRequired if
// key is absent or the value cannot be coerced.
func (s *StatSet) GetDouble(key string) (float64, error) {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return n, nil
	}
	if str, ok := val.(string); ok {
		n, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
		}
		return n, nil
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1, nil
		}
		return 0, nil
	}
	return 0, errValueRequired(key, "double", val)
}

// GetDoubleDefault is like GetDouble but returns defaultValue instead of an
// error when key is absent or cannot be coerced.
func (s *StatSet) GetDoubleDefault(key string, defaultValue float64) float64 {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return n
	}
	if str, ok := val.(string); ok {
		if n, err := strconv.ParseFloat(str, 64); err == nil {
			return n
		}
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1
		}
		return 0
	}
	return defaultValue
}

// GetDoubleArray returns the value at key as a []float64. A single number
// coerces to a one-element slice; a string coerces by splitting on ";" and
// parsing each part. Returns ErrValueRequired if key is absent or the value
// cannot be coerced.
func (s *StatSet) GetDoubleArray(key string) ([]float64, error) {
	val := s.values[key]
	if arr, ok := val.([]float64); ok {
		return arr, nil
	}
	if n, ok := asNumber(val); ok {
		return []float64{n}, nil
	}
	if str, ok := val.(string); ok {
		parts := strings.Split(str, ";")
		out := make([]float64, len(parts))
		for i, p := range parts {
			n, err := strconv.ParseFloat(p, 64)
			if err != nil {
				return nil, fmt.Errorf("commons: StatSet key %q: %w", key, err)
			}
			out[i] = n
		}
		return out, nil
	}
	return nil, errValueRequired(key, "double array", val)
}

// GetFloat32 returns the value at key as a float32, coercing a numeric,
// numeric string, or bool (1/0) representation. Returns ErrValueRequired if
// key is absent or the value cannot be coerced.
func (s *StatSet) GetFloat32(key string) (float32, error) {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return float32(n), nil
	}
	if str, ok := val.(string); ok {
		n, err := strconv.ParseFloat(str, 32)
		if err != nil {
			return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
		}
		return float32(n), nil
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1, nil
		}
		return 0, nil
	}
	return 0, errValueRequired(key, "float", val)
}

// GetFloat32Default is like GetFloat32 but returns defaultValue instead of
// an error when key is absent or cannot be coerced.
func (s *StatSet) GetFloat32Default(key string, defaultValue float32) float32 {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return float32(n)
	}
	if str, ok := val.(string); ok {
		if n, err := strconv.ParseFloat(str, 32); err == nil {
			return float32(n)
		}
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1
		}
		return 0
	}
	return defaultValue
}

// GetInt returns the value at key as an int, coercing a numeric, numeric
// string, or bool (1/0) representation. Returns ErrValueRequired if key is
// absent or the value cannot be coerced.
func (s *StatSet) GetInt(key string) (int, error) {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return int(n), nil
	}
	if str, ok := val.(string); ok {
		n, err := strconv.Atoi(str)
		if err != nil {
			return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
		}
		return n, nil
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1, nil
		}
		return 0, nil
	}
	return 0, errValueRequired(key, "int", val)
}

// GetIntDefault is like GetInt but returns defaultValue instead of an error
// when key is absent or cannot be coerced.
func (s *StatSet) GetIntDefault(key string, defaultValue int) int {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return int(n)
	}
	if str, ok := val.(string); ok {
		if n, err := strconv.Atoi(str); err == nil {
			return n
		}
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1
		}
		return 0
	}
	return defaultValue
}

// GetIntArray returns the value at key as a []int. A single number coerces
// to a one-element slice; a string coerces by splitting on ";" and parsing
// each part. Returns ErrValueRequired if key is absent or the value cannot
// be coerced.
func (s *StatSet) GetIntArray(key string) ([]int, error) {
	val := s.values[key]
	if arr, ok := val.([]int); ok {
		return arr, nil
	}
	if n, ok := asNumber(val); ok {
		return []int{int(n)}, nil
	}
	if str, ok := val.(string); ok {
		parts := strings.Split(str, ";")
		out := make([]int, len(parts))
		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil {
				return nil, fmt.Errorf("commons: StatSet key %q: %w", key, err)
			}
			out[i] = n
		}
		return out, nil
	}
	return nil, errValueRequired(key, "int array", val)
}

// GetIntArrayDefault is like GetIntArray but returns defaultArray instead of
// an error when key is absent or cannot be coerced.
func (s *StatSet) GetIntArrayDefault(key string, defaultArray []int) []int {
	val := s.values[key]
	if arr, ok := val.([]int); ok {
		return arr
	}
	if n, ok := asNumber(val); ok {
		return []int{int(n)}
	}
	if str, ok := val.(string); ok {
		parts := strings.Split(str, ";")
		out := make([]int, len(parts))
		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil {
				return defaultArray
			}
			out[i] = n
		}
		return out
	}
	return defaultArray
}

// GetList returns the slice stored at key, or nil if key is absent. Returns
// ErrValueRequired if the stored value is not a []T.
func GetList[T any](s *StatSet, key string) ([]T, error) {
	val := s.values[key]
	if val == nil {
		return nil, nil
	}
	arr, ok := val.([]T)
	if !ok {
		return nil, errValueRequired(key, "list", val)
	}
	return arr, nil
}

// GetLong returns the value at key as an int64, coercing a numeric, numeric
// string, or bool (1/0) representation. Returns ErrValueRequired if key is
// absent or the value cannot be coerced.
func (s *StatSet) GetLong(key string) (int64, error) {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return int64(n), nil
	}
	if str, ok := val.(string); ok {
		n, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
		}
		return n, nil
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1, nil
		}
		return 0, nil
	}
	return 0, errValueRequired(key, "long", val)
}

// GetLongDefault is like GetLong but returns defaultValue instead of an
// error when key is absent or cannot be coerced.
func (s *StatSet) GetLongDefault(key string, defaultValue int64) int64 {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return int64(n)
	}
	if str, ok := val.(string); ok {
		if n, err := strconv.ParseInt(str, 10, 64); err == nil {
			return n
		}
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1
		}
		return 0
	}
	return defaultValue
}

// GetLongArray returns the value at key as a []int64. A single number
// coerces to a one-element slice; a string coerces by splitting on ";" and
// parsing each part. Returns ErrValueRequired if key is absent or the value
// cannot be coerced.
func (s *StatSet) GetLongArray(key string) ([]int64, error) {
	val := s.values[key]
	if arr, ok := val.([]int64); ok {
		return arr, nil
	}
	if n, ok := asNumber(val); ok {
		return []int64{int64(n)}, nil
	}
	if str, ok := val.(string); ok {
		parts := strings.Split(str, ";")
		out := make([]int64, len(parts))
		for i, p := range parts {
			n, err := strconv.ParseInt(p, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("commons: StatSet key %q: %w", key, err)
			}
			out[i] = n
		}
		return out, nil
	}
	return nil, errValueRequired(key, "long array", val)
}

// GetMap returns the map stored at key, or nil if key is absent. Returns
// ErrValueRequired if the stored value is not a map[K]V.
func GetMap[K comparable, V any](s *StatSet, key string) (map[K]V, error) {
	val := s.values[key]
	if val == nil {
		return nil, nil
	}
	m, ok := val.(map[K]V)
	if !ok {
		return nil, errValueRequired(key, "map", val)
	}
	return m, nil
}

// GetString returns the value at key formatted as a string. Returns
// ErrValueRequired if key is absent.
func (s *StatSet) GetString(key string) (string, error) {
	val, ok := s.values[key]
	if !ok || val == nil {
		return "", errValueRequired(key, "string", val)
	}
	return toString(val), nil
}

// GetStringDefault is like GetString but returns defaultValue instead of an
// error when key is absent.
func (s *StatSet) GetStringDefault(key string, defaultValue string) string {
	val, ok := s.values[key]
	if !ok || val == nil {
		return defaultValue
	}
	return toString(val)
}

// toString formats val as a string for the value kinds StatSet stores.
func toString(val any) string {
	if str, ok := val.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", val)
}

// GetStringArray returns the value at key as a []string, splitting a string
// value on ";". Returns ErrValueRequired if key is absent or the value is
// neither a []string nor a string.
func (s *StatSet) GetStringArray(key string) ([]string, error) {
	val := s.values[key]
	if arr, ok := val.([]string); ok {
		return arr, nil
	}
	if str, ok := val.(string); ok {
		return strings.Split(str, ";"), nil
	}
	return nil, errValueRequired(key, "string array", val)
}

// GetStringArrayDefault is like GetStringArray but returns defaultArray
// instead of an error when key is absent or cannot be coerced.
func (s *StatSet) GetStringArrayDefault(key string, defaultArray []string) []string {
	val := s.values[key]
	if arr, ok := val.([]string); ok {
		return arr
	}
	if str, ok := val.(string); ok {
		return strings.Split(str, ";")
	}
	return defaultArray
}

// GetObject returns the value stored at key as T and true, or the zero
// value and false if key is absent or holds a value of a different type.
func GetObject[T any](s *StatSet, key string) (T, bool) {
	var zero T
	val, ok := s.values[key]
	if !ok || val == nil {
		return zero, false
	}
	t, ok := val.(T)
	if !ok {
		return zero, false
	}
	return t, true
}

// GetEnum returns the value at key as E. A value already stored as E is
// returned directly; a string value is looked up in names, keyed by the
// enum's canonical name. Returns ErrValueRequired if key is absent, the
// stored value is neither E nor a string, or the string matches no entry in
// names.
func GetEnum[E any](s *StatSet, key string, names map[string]E) (E, error) {
	var zero E
	val := s.values[key]
	if e, ok := val.(E); ok {
		return e, nil
	}
	if str, ok := val.(string); ok {
		if e, ok := names[str]; ok {
			return e, nil
		}
	}
	return zero, errValueRequired(key, "enum", val)
}

// GetEnumDefault is like GetEnum but returns defaultValue instead of an
// error when key is absent or cannot be resolved to E.
func GetEnumDefault[E any](s *StatSet, key string, names map[string]E, defaultValue E) E {
	val := s.values[key]
	if e, ok := val.(E); ok {
		return e
	}
	if str, ok := val.(string); ok {
		if e, ok := names[str]; ok {
			return e
		}
	}
	return defaultValue
}
