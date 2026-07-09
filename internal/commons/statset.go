// Package commons holds small typed data structures shared across the
// server that don't belong to any one game system.
package commons

import (
	"errors"
	"fmt"
	"math"
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
// can tolerate a missing value use the *Default variant instead — but a
// *present* value that fails to parse is still an error there: an optional
// attribute being absent is normal, a mangled number in a data file is
// corruption and must not silently become the default.
//
// That "present but malformed is still an error" rule applies to every
// accessor whose target type has a genuine parse failure mode: the numeric
// accessors (GetByte, GetInt, GetInt32, GetLong, GetDouble, GetFloat32 and
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

// coerceByte coerces val to a byte from a numeric or numeric string
// representation. ok reports whether val was a recognized kind; err reports
// a recognized-but-malformed numeric string.
func coerceByte(val any) (v byte, ok bool, err error) {
	if n, ok := asNumber(val); ok {
		return byte(int64(n)), true, nil
	}
	if str, ok := val.(string); ok {
		n, err := strconv.ParseInt(str, 10, 8)
		if err != nil {
			return 0, true, err
		}
		return byte(n), true, nil
	}
	return 0, false, nil
}

// GetByte returns the value at key as a byte, coercing a numeric or numeric
// string representation. Returns ErrValueRequired if key is absent or the
// value cannot be coerced.
func (s *StatSet) GetByte(key string) (byte, error) {
	val := s.values[key]
	v, ok, err := coerceByte(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return 0, errValueRequired(key, "byte", val)
	}
	return v, nil
}

// GetByteDefault is like GetByte but returns defaultValue when key is
// absent (or holds a value of a kind bytes don't coerce from). A string
// value that fails to parse is still an error.
func (s *StatSet) GetByteDefault(key string, defaultValue byte) (byte, error) {
	val := s.values[key]
	v, ok, err := coerceByte(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return defaultValue, nil
	}
	return v, nil
}

// coerceDouble coerces val to a float64 from a numeric, numeric string, or
// bool (1/0) representation. ok reports whether val was a recognized kind;
// err reports a recognized-but-malformed numeric string.
func coerceDouble(val any) (v float64, ok bool, err error) {
	if n, ok := asNumber(val); ok {
		return n, true, nil
	}
	if str, ok := val.(string); ok {
		n, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return 0, true, err
		}
		return n, true, nil
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1, true, nil
		}
		return 0, true, nil
	}
	return 0, false, nil
}

// GetDouble returns the value at key as a float64, coercing a numeric,
// numeric string, or bool (1/0) representation. Returns ErrValueRequired if
// key is absent or the value cannot be coerced.
func (s *StatSet) GetDouble(key string) (float64, error) {
	val := s.values[key]
	v, ok, err := coerceDouble(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return 0, errValueRequired(key, "double", val)
	}
	return v, nil
}

// GetDoubleDefault is like GetDouble but returns defaultValue when key is
// absent (or holds a value of a kind float64 doesn't coerce from). A string
// value that fails to parse is still an error.
func (s *StatSet) GetDoubleDefault(key string, defaultValue float64) (float64, error) {
	val := s.values[key]
	v, ok, err := coerceDouble(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return defaultValue, nil
	}
	return v, nil
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

// coerceFloat32 coerces val to a float32 from a numeric, numeric string, or
// bool (1/0) representation. ok reports whether val was a recognized kind;
// err reports a recognized-but-malformed numeric string.
func coerceFloat32(val any) (v float32, ok bool, err error) {
	if n, ok := asNumber(val); ok {
		return float32(n), true, nil
	}
	if str, ok := val.(string); ok {
		n, err := strconv.ParseFloat(str, 32)
		if err != nil {
			return 0, true, err
		}
		return float32(n), true, nil
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1, true, nil
		}
		return 0, true, nil
	}
	return 0, false, nil
}

// GetFloat32 returns the value at key as a float32, coercing a numeric,
// numeric string, or bool (1/0) representation. Returns ErrValueRequired if
// key is absent or the value cannot be coerced.
func (s *StatSet) GetFloat32(key string) (float32, error) {
	val := s.values[key]
	v, ok, err := coerceFloat32(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return 0, errValueRequired(key, "float", val)
	}
	return v, nil
}

// GetFloat32Default is like GetFloat32 but returns defaultValue when key is
// absent (or holds a value of a kind float32 doesn't coerce from). A string
// value that fails to parse is still an error.
func (s *StatSet) GetFloat32Default(key string, defaultValue float32) (float32, error) {
	val := s.values[key]
	v, ok, err := coerceFloat32(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return defaultValue, nil
	}
	return v, nil
}

// coerceInt coerces val to an int from a numeric, numeric string, or bool
// (1/0) representation. ok reports whether val was a recognized kind; err
// reports a recognized-but-malformed numeric string.
func coerceInt(val any) (v int, ok bool, err error) {
	if n, ok := asNumber(val); ok {
		return int(n), true, nil
	}
	if str, ok := val.(string); ok {
		n, err := strconv.Atoi(str)
		if err != nil {
			return 0, true, err
		}
		return n, true, nil
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1, true, nil
		}
		return 0, true, nil
	}
	return 0, false, nil
}

// GetInt returns the value at key as an int, coercing a numeric, numeric
// string, or bool (1/0) representation. Returns ErrValueRequired if key is
// absent or the value cannot be coerced.
func (s *StatSet) GetInt(key string) (int, error) {
	val := s.values[key]
	v, ok, err := coerceInt(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return 0, errValueRequired(key, "int", val)
	}
	return v, nil
}

// GetInt32 returns the value at key as an int32, coercing a numeric, numeric
// string, or bool (1/0) representation. Returns ErrValueRequired if key is
// absent or the value cannot be coerced, and an error if the value is out
// of int32 range.
func (s *StatSet) GetInt32(key string) (int32, error) {
	v, err := s.GetInt(key)
	if err != nil {
		return 0, err
	}
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0, fmt.Errorf("commons: StatSet key %q: value %d overflows int32", key, v)
	}
	return int32(v), nil
}

// GetInt32Default is like GetInt32 but returns defaultValue when key is
// absent (or holds a value of a kind int doesn't coerce from). A string
// value that fails to parse, or a present value out of int32 range, is
// still an error.
func (s *StatSet) GetInt32Default(key string, defaultValue int32) (int32, error) {
	v, err := s.GetIntDefault(key, int(defaultValue))
	if err != nil {
		return 0, err
	}
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0, fmt.Errorf("commons: StatSet key %q: value %d overflows int32", key, v)
	}
	return int32(v), nil
}

// GetIntDefault is like GetInt but returns defaultValue when key is absent
// (or holds a value of a kind int doesn't coerce from). A string value that
// fails to parse is still an error.
func (s *StatSet) GetIntDefault(key string, defaultValue int) (int, error) {
	val := s.values[key]
	v, ok, err := coerceInt(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return defaultValue, nil
	}
	return v, nil
}

// GetIntArray returns the value at key as a []int. A single number coerces
// to a one-element slice; a string coerces by splitting on ";" and parsing
// each part. Returns ErrValueRequired if key is absent or the value cannot
// be coerced.
func (s *StatSet) GetIntArray(key string) ([]int, error) {
	val := s.values[key]
	v, ok, err := coerceIntArray(val)
	if err != nil {
		return nil, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return nil, errValueRequired(key, "int array", val)
	}
	return v, nil
}

// GetIntArrayDefault is like GetIntArray but returns defaultArray when key
// is absent (or holds a value of a kind an int slice doesn't coerce from).
// A string value with an element that fails to parse is still an error.
func (s *StatSet) GetIntArrayDefault(key string, defaultArray []int) ([]int, error) {
	val := s.values[key]
	v, ok, err := coerceIntArray(val)
	if err != nil {
		return nil, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return defaultArray, nil
	}
	return v, nil
}

// coerceIntArray coerces val to a []int from a []int, a single number
// (one-element slice), or a ";"-separated numeric string. ok reports
// whether val was a recognized kind; err reports a recognized-but-malformed
// element.
func coerceIntArray(val any) (v []int, ok bool, err error) {
	if arr, ok := val.([]int); ok {
		return arr, true, nil
	}
	if n, ok := asNumber(val); ok {
		return []int{int(n)}, true, nil
	}
	if str, ok := val.(string); ok {
		parts := strings.Split(str, ";")
		out := make([]int, len(parts))
		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil {
				return nil, true, err
			}
			out[i] = n
		}
		return out, true, nil
	}
	return nil, false, nil
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
	v, ok, err := coerceLong(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return 0, errValueRequired(key, "long", val)
	}
	return v, nil
}

// GetLongDefault is like GetLong but returns defaultValue when key is
// absent (or holds a value of a kind int64 doesn't coerce from). A string
// value that fails to parse is still an error.
func (s *StatSet) GetLongDefault(key string, defaultValue int64) (int64, error) {
	val := s.values[key]
	v, ok, err := coerceLong(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return defaultValue, nil
	}
	return v, nil
}

// coerceLong coerces val to an int64 from a numeric, numeric string, or
// bool (1/0) representation. ok reports whether val was a recognized kind;
// err reports a recognized-but-malformed numeric string.
func coerceLong(val any) (v int64, ok bool, err error) {
	if n, ok := asNumber(val); ok {
		return int64(n), true, nil
	}
	if str, ok := val.(string); ok {
		n, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return 0, true, err
		}
		return n, true, nil
	}
	if b, ok := val.(bool); ok {
		if b {
			return 1, true, nil
		}
		return 0, true, nil
	}
	return 0, false, nil
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

// GetEnumDefault is like GetEnum but returns defaultValue when key is
// absent (or holds a value of a kind E doesn't coerce from). A string value
// that matches no entry in names is still an error.
func GetEnumDefault[E any](s *StatSet, key string, names map[string]E, defaultValue E) (E, error) {
	var zero E
	val := s.values[key]
	if e, ok := val.(E); ok {
		return e, nil
	}
	if str, ok := val.(string); ok {
		e, ok := names[str]
		if !ok {
			return zero, errValueRequired(key, "enum", val)
		}
		return e, nil
	}
	return defaultValue, nil
}
