package commons

import (
	"fmt"
	"strings"
)

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
