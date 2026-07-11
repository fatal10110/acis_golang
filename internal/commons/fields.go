package commons

import "fmt"

// Fields is a sticky-error view over a StatSet: each accessor reads freely
// and records the first failure internally instead of returning it, so a
// model constructor can read every field of a StatSet and check Err once at
// the end rather than after every read. Once an error is recorded, every
// later accessor call is a no-op that returns its zero value or supplied
// default without touching the underlying StatSet.
type Fields struct {
	set    *StatSet
	prefix string
	err    error
}

// NewFields wraps set for sticky-error reading. prefix identifies the value
// being built (e.g. "npc template") and is prepended to the first error
// Fields records.
func NewFields(set *StatSet, prefix string) *Fields {
	return &Fields{set: set, prefix: prefix}
}

// Err reports the first error recorded by any accessor call, if any.
func (f *Fields) Err() error {
	return f.err
}

// Has reports whether key is present in the underlying StatSet, regardless
// of any error already recorded.
func (f *Fields) Has(key string) bool {
	return f.set.Has(key)
}

// Fail records err, prefixed, as f's first error if none is already set.
// Later calls, including those made internally by other accessors, are
// no-ops, so a chain of reads surfaces exactly the first failure.
func (f *Fields) Fail(err error) {
	if f.err == nil {
		f.err = fmt.Errorf("%s: %w", f.prefix, err)
	}
}

// Int returns the value at key as an int, recording an error if key is
// absent or cannot be coerced.
func (f *Fields) Int(key string) int {
	if f.err != nil {
		return 0
	}
	v, err := f.set.GetInt(key)
	if err != nil {
		f.Fail(err)
		return 0
	}
	return v
}

// IntDefault returns the value at key as an int, or def if key is absent. A
// present-but-malformed value still records an error.
func (f *Fields) IntDefault(key string, def int) int {
	if f.err != nil {
		return def
	}
	v, err := f.set.GetIntDefault(key, def)
	if err != nil {
		f.Fail(err)
		return def
	}
	return v
}

// Int32 returns the value at key as an int32, recording an error if key is
// absent, cannot be coerced or overflows int32.
func (f *Fields) Int32(key string) int32 {
	if f.err != nil {
		return 0
	}
	v, err := f.set.GetInt32(key)
	if err != nil {
		f.Fail(err)
		return 0
	}
	return v
}

// Int32Default returns the value at key as an int32, or def if key is
// absent. A present-but-malformed or overflowing value still records an error.
func (f *Fields) Int32Default(key string, def int32) int32 {
	if f.err != nil {
		return def
	}
	v, err := f.set.GetInt32Default(key, def)
	if err != nil {
		f.Fail(err)
		return def
	}
	return v
}

// Int64 returns the value at key as an int64, recording an error if key is
// absent or cannot be coerced.
func (f *Fields) Int64(key string) int64 {
	if f.err != nil {
		return 0
	}
	v, err := f.set.GetInt64(key)
	if err != nil {
		f.Fail(err)
		return 0
	}
	return v
}

// Int64Default returns the value at key as an int64, or def if key is absent.
// A present-but-malformed value still records an error.
func (f *Fields) Int64Default(key string, def int64) int64 {
	if f.err != nil {
		return def
	}
	v, err := f.set.GetInt64Default(key, def)
	if err != nil {
		f.Fail(err)
		return def
	}
	return v
}

// Float64 returns the value at key as a float64, recording an error if key is
// absent or cannot be coerced.
func (f *Fields) Float64(key string) float64 {
	if f.err != nil {
		return 0
	}
	v, err := f.set.GetFloat64(key)
	if err != nil {
		f.Fail(err)
		return 0
	}
	return v
}

// Float64Default returns the value at key as a float64, or def if key is
// absent. A present-but-malformed value still records an error.
func (f *Fields) Float64Default(key string, def float64) float64 {
	if f.err != nil {
		return def
	}
	v, err := f.set.GetFloat64Default(key, def)
	if err != nil {
		f.Fail(err)
		return def
	}
	return v
}

// Float32 returns the value at key as a float32, recording an error if key
// is absent or cannot be coerced.
func (f *Fields) Float32(key string) float32 {
	if f.err != nil {
		return 0
	}
	v, err := f.set.GetFloat32(key)
	if err != nil {
		f.Fail(err)
		return 0
	}
	return v
}

// Float32Default returns the value at key as a float32, or def if key is
// absent. A present-but-malformed value still records an error.
func (f *Fields) Float32Default(key string, def float32) float32 {
	if f.err != nil {
		return def
	}
	v, err := f.set.GetFloat32Default(key, def)
	if err != nil {
		f.Fail(err)
		return def
	}
	return v
}

// String returns the value at key formatted as a string, recording an error
// if key is absent.
func (f *Fields) String(key string) string {
	if f.err != nil {
		return ""
	}
	v, err := f.set.GetString(key)
	if err != nil {
		f.Fail(err)
		return ""
	}
	return v
}

// StringDefault returns the value at key formatted as a string, or def if
// key is absent.
func (f *Fields) StringDefault(key string, def string) string {
	if f.err != nil {
		return def
	}
	return f.set.GetStringDefault(key, def)
}

// Bool returns the value at key as a bool, recording an error if key is
// absent or cannot be coerced.
func (f *Fields) Bool(key string) bool {
	if f.err != nil {
		return false
	}
	v, err := f.set.GetBool(key)
	if err != nil {
		f.Fail(err)
		return false
	}
	return v
}

// BoolDefault returns the value at key as a bool, or def if key is absent or
// cannot be coerced.
func (f *Fields) BoolDefault(key string, def bool) bool {
	if f.err != nil {
		return def
	}
	return f.set.GetBoolDefault(key, def)
}

// IntArray returns the value at key as a []int, recording an error if key is
// absent or cannot be coerced.
func (f *Fields) IntArray(key string) []int {
	if f.err != nil {
		return nil
	}
	v, err := f.set.GetIntArray(key)
	if err != nil {
		f.Fail(err)
		return nil
	}
	return v
}

// IntArrayDefault returns the value at key as a []int, or def if key is
// absent. A present-but-malformed value still records an error.
func (f *Fields) IntArrayDefault(key string, def []int) []int {
	if f.err != nil {
		return def
	}
	v, err := f.set.GetIntArrayDefault(key, def)
	if err != nil {
		f.Fail(err)
		return def
	}
	return v
}

// Float64Array returns the value at key as a []float64, recording an error if
// key is absent or cannot be coerced.
func (f *Fields) Float64Array(key string) []float64 {
	if f.err != nil {
		return nil
	}
	v, err := f.set.GetFloat64Array(key)
	if err != nil {
		f.Fail(err)
		return nil
	}
	return v
}

// StringArray returns the value at key as a []string, recording an error if
// key is absent or cannot be coerced.
func (f *Fields) StringArray(key string) []string {
	if f.err != nil {
		return nil
	}
	v, err := f.set.GetStringArray(key)
	if err != nil {
		f.Fail(err)
		return nil
	}
	return v
}

// StringArrayDefault returns the value at key as a []string, or def if key is
// absent or cannot be coerced.
func (f *Fields) StringArrayDefault(key string, def []string) []string {
	if f.err != nil {
		return def
	}
	return f.set.GetStringArrayDefault(key, def)
}

// FieldObject returns the value stored at key in f's underlying StatSet as T
// and true, or the zero value and false if key is absent, f already carries
// an error, or the stored value isn't a T. Mirrors GetObject for Fields.
func FieldObject[T any](f *Fields, key string) (T, bool) {
	var zero T
	if f.err != nil {
		return zero, false
	}
	return GetObject[T](f.set, key)
}

// FieldList returns the slice stored at key in f's underlying StatSet,
// recording an error if the stored value is not a []T. Mirrors GetList for
// Fields.
func FieldList[T any](f *Fields, key string) []T {
	if f.err != nil {
		return nil
	}
	v, err := GetList[T](f.set, key)
	if err != nil {
		f.Fail(err)
		return nil
	}
	return v
}

// FieldEnum returns the value at key in f's underlying StatSet as E,
// recording an error if key is absent or doesn't match an entry in names.
// Mirrors GetEnum for Fields.
func FieldEnum[E any](f *Fields, key string, names map[string]E) E {
	var zero E
	if f.err != nil {
		return zero
	}
	v, err := GetEnum[E](f.set, key, names)
	if err != nil {
		f.Fail(err)
		return zero
	}
	return v
}

// FieldEnumDefault returns the value at key in f's underlying StatSet as E,
// or def if key is absent. A present value that matches no entry in names
// still records an error. Mirrors GetEnumDefault for Fields.
func FieldEnumDefault[E any](f *Fields, key string, names map[string]E, def E) E {
	if f.err != nil {
		return def
	}
	v, err := GetEnumDefault[E](f.set, key, names, def)
	if err != nil {
		f.Fail(err)
		return def
	}
	return v
}
