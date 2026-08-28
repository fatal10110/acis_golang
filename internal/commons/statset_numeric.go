package commons

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

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
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	}
	return 0, false
}

// asInt64 coerces any of the numeric representations StatSet stores
// (signed/unsigned integers of any width, float32, float64) into an int64
// by converting each kind directly, never routing through float64 — so an
// int64 value beyond float64's 53-bit mantissa round-trips exactly through
// the integer accessors (GetByte, GetInt, GetInt64, and their array/Default
// variants).
func asInt64(val any) (int64, bool, error) {
	switch v := val.(type) {
	case int:
		return int64(v), true, nil
	case int8:
		return int64(v), true, nil
	case int16:
		return int64(v), true, nil
	case int32:
		return int64(v), true, nil
	case int64:
		return v, true, nil
	case uint:
		return int64(v), true, nil
	case uint8:
		return int64(v), true, nil
	case uint16:
		return int64(v), true, nil
	case uint32:
		return int64(v), true, nil
	case uint64:
		if v > math.MaxInt64 {
			return 0, true, fmt.Errorf("value %d overflows int64", v)
		}
		return int64(v), true, nil
	case float32:
		return int64(v), true, nil
	case float64:
		return int64(v), true, nil
	}
	return 0, false, nil
}

// coerceByte coerces val to a byte from a numeric or numeric string
// representation. ok reports whether val was a recognized kind; err reports
// a recognized-but-malformed numeric string.
func coerceByte(val any) (v byte, ok bool, err error) {
	if n, ok, err := asInt64(val); err != nil {
		return 0, true, err
	} else if ok {
		return byte(n), true, nil
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

// coerceFloat64 coerces val to a float64 from a numeric, numeric string, or
// bool (1/0) representation. ok reports whether val was a recognized kind;
// err reports a recognized-but-malformed numeric string.
func coerceFloat64(val any) (v float64, ok bool, err error) {
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

// GetFloat64 returns the value at key as a float64, coercing a numeric,
// numeric string, or bool (1/0) representation. Returns ErrValueRequired if
// key is absent or the value cannot be coerced.
func (s *StatSet) GetFloat64(key string) (float64, error) {
	val := s.values[key]
	v, ok, err := coerceFloat64(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return 0, errValueRequired(key, "float64", val)
	}
	return v, nil
}

// GetFloat64Default is like GetFloat64 but returns defaultValue when key is
// absent (or holds a value of a kind float64 doesn't coerce from). A string
// value that fails to parse is still an error.
func (s *StatSet) GetFloat64Default(key string, defaultValue float64) (float64, error) {
	val := s.values[key]
	v, ok, err := coerceFloat64(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return defaultValue, nil
	}
	return v, nil
}

// GetFloat64Array returns the value at key as a []float64. A single number
// coerces to a one-element slice; a string coerces by splitting on ";" and
// parsing each part. Returns ErrValueRequired if key is absent or the value
// cannot be coerced.
func (s *StatSet) GetFloat64Array(key string) ([]float64, error) {
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
	return nil, errValueRequired(key, "float64 array", val)
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
	if n, ok, err := asInt64(val); err != nil {
		return 0, true, err
	} else if ok {
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
// is absent, holds a value of a kind an int slice doesn't coerce from, or
// holds a string with an element that fails to parse.
func (s *StatSet) GetIntArrayDefault(key string, defaultArray []int) ([]int, error) {
	val := s.values[key]
	v, ok, err := coerceIntArray(val)
	if err != nil || !ok {
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
	if n, ok, err := asInt64(val); err != nil {
		return nil, true, err
	} else if ok {
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

// GetInt64 returns the value at key as an int64, coercing a numeric, numeric
// string, or bool (1/0) representation. Returns ErrValueRequired if key is
// absent or the value cannot be coerced.
func (s *StatSet) GetInt64(key string) (int64, error) {
	val := s.values[key]
	v, ok, err := coerceInt64(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return 0, errValueRequired(key, "int64", val)
	}
	return v, nil
}

// GetInt64Default is like GetInt64 but returns defaultValue when key is
// absent (or holds a value of a kind int64 doesn't coerce from). A string
// value that fails to parse is still an error.
func (s *StatSet) GetInt64Default(key string, defaultValue int64) (int64, error) {
	val := s.values[key]
	v, ok, err := coerceInt64(val)
	if err != nil {
		return 0, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	}
	if !ok {
		return defaultValue, nil
	}
	return v, nil
}

// coerceInt64 coerces val to an int64 from a numeric, numeric string, or
// bool (1/0) representation. ok reports whether val was a recognized kind;
// err reports a recognized-but-malformed numeric string.
func coerceInt64(val any) (v int64, ok bool, err error) {
	if n, ok, err := asInt64(val); err != nil {
		return 0, true, err
	} else if ok {
		return n, true, nil
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

// GetInt64Array returns the value at key as a []int64. A single number
// coerces to a one-element slice; a string coerces by splitting on ";" and
// parsing each part. Returns ErrValueRequired if key is absent or the value
// cannot be coerced.
func (s *StatSet) GetInt64Array(key string) ([]int64, error) {
	val := s.values[key]
	if arr, ok := val.([]int64); ok {
		return arr, nil
	}
	if n, ok, err := asInt64(val); err != nil {
		return nil, fmt.Errorf("commons: StatSet key %q: %w", key, err)
	} else if ok {
		return []int64{n}, nil
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
	return nil, errValueRequired(key, "int64 array", val)
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
