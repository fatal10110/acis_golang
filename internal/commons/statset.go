// Package commons holds small typed data structures shared across the
// server that don't belong to any one game system.
package commons

import (
	"fmt"
	"strconv"
	"strings"
)

// StatSet is a typed attribute bag: values are stored as heterogeneous
// key/value pairs and read back through typed accessors that coerce between
// the stored representation and the requested type (e.g. a value stored as
// the string "1;2;3" can be read back via GetIntArray).
//
// The plain accessors (GetInt, GetString, ...) panic if key is absent or the
// stored value cannot be coerced to the requested type, so a caller reading
// a key it considers mandatory gets a loud failure instead of a zero value
// masquerading as real data. Callers that can tolerate a missing or
// malformed value use the *Default variant instead.
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

// panicRequired panics because key was read through a mandatory accessor but
// held no value coercible to wanted.
func panicRequired(key, wanted string, val any) {
	panic(fmt.Sprintf("commons: StatSet key %q requires a %s value, got %v", key, wanted, val))
}

// GetBool returns the value at key as a bool, coercing a string ("true",
// case-insensitive) or numeric (nonzero) representation. Panics if key is
// absent or the value cannot be coerced.
func (s *StatSet) GetBool(key string) bool {
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
	panicRequired(key, "bool", val)
	return false
}

// GetBoolDefault is like GetBool but returns defaultValue instead of
// panicking when key is absent or cannot be coerced.
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
// string representation. Panics if key is absent or the value cannot be
// coerced.
func (s *StatSet) GetByte(key string) byte {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return byte(int64(n))
	}
	if str, ok := val.(string); ok {
		n, err := strconv.ParseInt(str, 10, 8)
		if err != nil {
			panic(err)
		}
		return byte(n)
	}
	panicRequired(key, "byte", val)
	return 0
}

// GetByteDefault is like GetByte but returns defaultValue instead of
// panicking when key is absent or cannot be coerced.
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
// numeric string, or bool (1/0) representation. Panics if key is absent or
// the value cannot be coerced.
func (s *StatSet) GetDouble(key string) float64 {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return n
	}
	if str, ok := val.(string); ok {
		n, err := strconv.ParseFloat(str, 64)
		if err != nil {
			panic(err)
		}
		return n
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1
		}
		return 0
	}
	panicRequired(key, "double", val)
	return 0
}

// GetDoubleDefault is like GetDouble but returns defaultValue instead of
// panicking when key is absent or cannot be coerced.
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
// parsing each part. Panics if key is absent or the value cannot be
// coerced.
func (s *StatSet) GetDoubleArray(key string) []float64 {
	val := s.values[key]
	if arr, ok := val.([]float64); ok {
		return arr
	}
	if n, ok := asNumber(val); ok {
		return []float64{n}
	}
	if str, ok := val.(string); ok {
		parts := strings.Split(str, ";")
		out := make([]float64, len(parts))
		for i, p := range parts {
			n, err := strconv.ParseFloat(p, 64)
			if err != nil {
				panic(err)
			}
			out[i] = n
		}
		return out
	}
	panicRequired(key, "double array", val)
	return nil
}

// GetFloat32 returns the value at key as a float32, coercing a numeric,
// numeric string, or bool (1/0) representation. Panics if key is absent or
// the value cannot be coerced.
func (s *StatSet) GetFloat32(key string) float32 {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return float32(n)
	}
	if str, ok := val.(string); ok {
		n, err := strconv.ParseFloat(str, 32)
		if err != nil {
			panic(err)
		}
		return float32(n)
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1
		}
		return 0
	}
	panicRequired(key, "float", val)
	return 0
}

// GetFloat32Default is like GetFloat32 but returns defaultValue instead of
// panicking when key is absent or cannot be coerced.
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
// string, or bool (1/0) representation. Panics if key is absent or the
// value cannot be coerced.
func (s *StatSet) GetInt(key string) int {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return int(n)
	}
	if str, ok := val.(string); ok {
		n, err := strconv.Atoi(str)
		if err != nil {
			panic(err)
		}
		return n
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1
		}
		return 0
	}
	panicRequired(key, "int", val)
	return 0
}

// GetIntDefault is like GetInt but returns defaultValue instead of
// panicking when key is absent or cannot be coerced.
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
// each part. Panics if key is absent or the value cannot be coerced.
func (s *StatSet) GetIntArray(key string) []int {
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
				panic(err)
			}
			out[i] = n
		}
		return out
	}
	panicRequired(key, "int array", val)
	return nil
}

// GetIntArrayDefault is like GetIntArray but returns defaultArray instead of
// panicking when key is absent or cannot be coerced.
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

// GetList returns the slice stored at key, or nil if key is absent. Panics
// if the stored value is not a []T.
func GetList[T any](s *StatSet, key string) []T {
	val := s.values[key]
	if val == nil {
		return nil
	}
	return val.([]T)
}

// GetLong returns the value at key as an int64, coercing a numeric, numeric
// string, or bool (1/0) representation. Panics if key is absent or the
// value cannot be coerced.
func (s *StatSet) GetLong(key string) int64 {
	val := s.values[key]
	if n, ok := asNumber(val); ok {
		return int64(n)
	}
	if str, ok := val.(string); ok {
		n, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			panic(err)
		}
		return n
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1
		}
		return 0
	}
	panicRequired(key, "long", val)
	return 0
}

// GetLongDefault is like GetLong but returns defaultValue instead of
// panicking when key is absent or cannot be coerced.
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
// parsing each part. Panics if key is absent or the value cannot be
// coerced.
func (s *StatSet) GetLongArray(key string) []int64 {
	val := s.values[key]
	if arr, ok := val.([]int64); ok {
		return arr
	}
	if n, ok := asNumber(val); ok {
		return []int64{int64(n)}
	}
	if str, ok := val.(string); ok {
		parts := strings.Split(str, ";")
		out := make([]int64, len(parts))
		for i, p := range parts {
			n, err := strconv.ParseInt(p, 10, 64)
			if err != nil {
				panic(err)
			}
			out[i] = n
		}
		return out
	}
	panicRequired(key, "long array", val)
	return nil
}

// GetMap returns the map stored at key, or nil if key is absent. Panics if
// the stored value is not a map[K]V.
func GetMap[K comparable, V any](s *StatSet, key string) map[K]V {
	val := s.values[key]
	if val == nil {
		return nil
	}
	return val.(map[K]V)
}

// GetString returns the value at key formatted as a string. Panics if key
// is absent.
func (s *StatSet) GetString(key string) string {
	val, ok := s.values[key]
	if !ok || val == nil {
		panic(fmt.Sprintf("commons: StatSet key %q requires a string value, but is unset", key))
	}
	return toString(val)
}

// GetStringDefault is like GetString but returns defaultValue instead of
// panicking when key is absent.
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
// value on ";". Panics if key is absent or the value is neither a []string
// nor a string.
func (s *StatSet) GetStringArray(key string) []string {
	val := s.values[key]
	if arr, ok := val.([]string); ok {
		return arr
	}
	if str, ok := val.(string); ok {
		return strings.Split(str, ";")
	}
	panicRequired(key, "string array", val)
	return nil
}

// GetStringArrayDefault is like GetStringArray but returns defaultArray
// instead of panicking when key is absent or cannot be coerced.
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
