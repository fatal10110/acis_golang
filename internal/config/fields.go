package config

import "fmt"

// Fields is a sticky-error view over Properties: each accessor reads freely
// and records the first failure internally instead of returning it, so an
// options loader can read every key it needs and check Err once at the end
// rather than after every read. Once an error is recorded, every later
// accessor call is a no-op that returns its supplied default without
// touching the underlying Properties.
//
// Default semantics match Properties directly: an absent key returns the
// caller's default; a present-but-malformed value still records an error.
type Fields struct {
	props  *Properties
	prefix string
	err    error
}

// NewFields wraps props for sticky-error reading. prefix identifies the
// value being built (e.g. "ground item options") and is prepended to the
// first error Fields records. A nil props reads as though every key is
// absent, so every accessor returns its default.
func NewFields(props *Properties, prefix string) *Fields {
	if props == nil {
		props = &Properties{}
	}
	return &Fields{props: props, prefix: prefix}
}

// Err reports the first error recorded by any accessor call, if any.
func (f *Fields) Err() error {
	return f.err
}

// Fail records err, prefixed, as f's first error if none is already set.
func (f *Fields) Fail(err error) {
	if f.err == nil {
		f.err = fmt.Errorf("%s: %w", f.prefix, err)
	}
}

// String returns a string property or def when key is missing.
func (f *Fields) String(key, def string) string {
	return f.props.String(key, def)
}

// Bool returns a bool property or def when key is missing.
func (f *Fields) Bool(key string, def bool) bool {
	return f.props.Bool(key, def)
}

// Int returns an int property or def when key is missing. A
// present-but-malformed value records an error and still returns def.
func (f *Fields) Int(key string, def int) int {
	if f.err != nil {
		return def
	}
	v, err := f.props.Int(key, def)
	if err != nil {
		f.Fail(err)
		return def
	}
	return v
}

// Int64 returns an int64 property or def when key is missing. A
// present-but-malformed value records an error and still returns def.
func (f *Fields) Int64(key string, def int64) int64 {
	if f.err != nil {
		return def
	}
	v, err := f.props.Int64(key, def)
	if err != nil {
		f.Fail(err)
		return def
	}
	return v
}

// Float64 returns a float64 property or def when key is missing. A
// present-but-malformed value records an error and still returns def.
func (f *Fields) Float64(key string, def float64) float64 {
	if f.err != nil {
		return def
	}
	v, err := f.props.Float64(key, def)
	if err != nil {
		f.Fail(err)
		return def
	}
	return v
}

// IntPairs returns pairs parsed from a value shaped like "57-100;6651-3" or
// "57-0,5575-0", or def (parsed the same way) when key is missing. A
// present-but-malformed value records an error and returns nil.
func (f *Fields) IntPairs(key, def string) []IntPair {
	if f.err != nil {
		return nil
	}
	v, err := f.props.IntPairs(key, def)
	if err != nil {
		f.Fail(err)
		return nil
	}
	return v
}
