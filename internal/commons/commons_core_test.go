package commons

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"
)

// ---- from fields_test.go ----
func TestFieldsMandatoryAccessors(t *testing.T) {
	s := NewStatSet()
	s.Set("int", 42)
	s.Set("int64", int64(9000000000))
	s.Set("float64", 3.14)
	s.Set("float", float32(2.5))
	s.Set("string", "hi")
	s.Set("intArray", []int{1, 2, 3})
	s.Set("float64Array", []float64{1.5, 2.5})
	s.Set("stringArray", []string{"a", "b"})

	f := NewFields(s, "test")
	if got := f.Int("int"); got != 42 {
		t.Errorf("Int() = %v, want 42", got)
	}
	if got := f.Int64("int64"); got != 9000000000 {
		t.Errorf("Int64() = %v, want 9000000000", got)
	}
	if got := f.Float64("float64"); got != 3.14 {
		t.Errorf("Float64() = %v, want 3.14", got)
	}
	if got := f.Float32("float"); got != 2.5 {
		t.Errorf("Float32() = %v, want 2.5", got)
	}
	if got := f.String("string"); got != "hi" {
		t.Errorf("String() = %v, want hi", got)
	}
	if got := f.IntArray("intArray"); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("IntArray() = %v, want [1 2 3]", got)
	}
	if got := f.Float64Array("float64Array"); !reflect.DeepEqual(got, []float64{1.5, 2.5}) {
		t.Errorf("Float64Array() = %v, want [1.5 2.5]", got)
	}
	if got := f.StringArray("stringArray"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("StringArray() = %v, want [a b]", got)
	}
	if err := f.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}

func TestFieldsMandatoryAccessorRecordsErrorOnAbsentKey(t *testing.T) {
	f := NewFields(NewStatSet(), "widget")
	if got := f.Int("missing"); got != 0 {
		t.Errorf("Int() = %v, want 0", got)
	}
	err := f.Err()
	if err == nil {
		t.Fatal("Err() = nil, want error for missing mandatory key")
	}
	if !errors.Is(err, ErrValueRequired) {
		t.Errorf("Err() = %v, want wrapping ErrValueRequired", err)
	}
}

func TestFieldsDefaultAccessors(t *testing.T) {
	s := NewStatSet()
	s.Set("int", 7)
	f := NewFields(s, "test")

	if got := f.IntDefault("int", 99); got != 7 {
		t.Errorf("IntDefault() present = %v, want 7", got)
	}
	if got := f.IntDefault("absent", 99); got != 99 {
		t.Errorf("IntDefault() absent = %v, want 99", got)
	}
	if got := f.StringDefault("absent", "fallback"); got != "fallback" {
		t.Errorf("StringDefault() absent = %v, want fallback", got)
	}
	if got := f.BoolDefault("absent", true); got != true {
		t.Errorf("BoolDefault() absent = %v, want true", got)
	}
	if err := f.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}

func TestFieldsDefaultAccessorStillErrorsOnMalformedValue(t *testing.T) {
	s := NewStatSet()
	s.Set("int", "not-a-number")
	f := NewFields(s, "test")

	if got := f.IntDefault("int", 99); got != 99 {
		t.Errorf("IntDefault() malformed = %v, want default 99", got)
	}
	if f.Err() == nil {
		t.Fatal("Err() = nil, want error for present-but-malformed value")
	}
}

func TestFieldsInt32LiteralDefault(t *testing.T) {
	s := NewStatSet()
	s.Set("decimal", "42")
	s.Set("hex", "0x7fffffff")
	f := NewFields(s, "test")

	if got := f.Int32LiteralDefault("decimal", 99); got != 42 {
		t.Errorf("Int32LiteralDefault(decimal) = %v, want 42", got)
	}
	if got := f.Int32LiteralDefault("hex", 99); got != 1<<31-1 {
		t.Errorf("Int32LiteralDefault(hex) = %v, want %v", got, int32(1<<31-1))
	}
	if got := f.Int32LiteralDefault("absent", 99); got != 99 {
		t.Errorf("Int32LiteralDefault(absent) = %v, want 99", got)
	}
	if err := f.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}

func TestFieldsInt32LiteralDefaultRejectsMalformedValue(t *testing.T) {
	s := NewStatSet()
	s.Set("bad", "not-a-number")
	f := NewFields(s, "test")

	if got := f.Int32LiteralDefault("bad", 99); got != 99 {
		t.Errorf("Int32LiteralDefault(bad) = %v, want default 99", got)
	}
	if f.Err() == nil {
		t.Fatal("Err() = nil, want error for present-but-malformed value")
	}
}

func TestFieldsInt32LiteralDefaultRejectsOverflow(t *testing.T) {
	s := NewStatSet()
	s.Set("overflow", "0x80000000")
	f := NewFields(s, "test")

	if got := f.Int32LiteralDefault("overflow", 99); got != 99 {
		t.Errorf("Int32LiteralDefault(overflow) = %v, want default 99", got)
	}
	if f.Err() == nil {
		t.Fatal("Err() = nil, want error for overflowing value")
	}
}

func TestFieldsStickyAfterFirstError(t *testing.T) {
	s := NewStatSet()
	s.Set("second", 5)
	f := NewFields(s, "test")

	f.Int("first") // absent, records the first error
	firstErr := f.Err()
	if firstErr == nil {
		t.Fatal("Err() = nil after reading a missing mandatory key")
	}

	if got := f.Int("second"); got != 0 {
		t.Errorf("Int() after error = %v, want 0 (untouched)", got)
	}
	if got := f.IntDefault("second", 42); got != 42 {
		t.Errorf("IntDefault() after error = %v, want the supplied default", got)
	}
	if f.Err() != firstErr {
		t.Errorf("Err() changed after first error: got %v, want %v", f.Err(), firstErr)
	}
}

func TestFieldsHasIgnoresRecordedError(t *testing.T) {
	s := NewStatSet()
	s.Set("present", 1)
	f := NewFields(s, "test")
	f.Int("missing")

	if !f.Has("present") {
		t.Error("Has() = false for a present key after an unrelated error")
	}
	if f.Has("absent") {
		t.Error("Has() = true for an absent key")
	}
}

func TestFieldsFailRecordsOnlyFirstError(t *testing.T) {
	f := NewFields(NewStatSet(), "widget")
	f.Fail(errors.New("first"))
	f.Fail(errors.New("second"))

	if got := f.Err().Error(); got != "widget: first" {
		t.Errorf("Err() = %q, want %q", got, "widget: first")
	}
}

func TestFieldGenericHelpers(t *testing.T) {
	type color int
	const (
		colorRed color = iota
		colorBlue
	)
	names := map[string]color{"RED": colorRed, "BLUE": colorBlue}

	s := NewStatSet()
	s.Set("color", "BLUE")
	s.Set("list", []int{1, 2})
	s.Set("obj", "hello")
	f := NewFields(s, "test")

	if got := FieldEnum[color](f, "color", names); got != colorBlue {
		t.Errorf("FieldEnum() = %v, want colorBlue", got)
	}
	if got := FieldEnumDefault[color](f, "missing", names, colorRed); got != colorRed {
		t.Errorf("FieldEnumDefault() absent = %v, want colorRed", got)
	}
	if got := FieldList[int](f, "list"); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("FieldList() = %v, want [1 2]", got)
	}
	if got, ok := FieldObject[string](f, "obj"); !ok || got != "hello" {
		t.Errorf("FieldObject() = (%v, %v), want (hello, true)", got, ok)
	}
	if _, ok := FieldObject[int](f, "obj"); ok {
		t.Error("FieldObject() with wrong type = true, want false")
	}
	if err := f.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}

	// Once an error is recorded, generic helpers stop touching the StatSet.
	f2 := NewFields(s, "test")
	f2.Int("missing")
	if got := FieldEnum[color](f2, "color", names); got != colorRed {
		t.Errorf("FieldEnum() after error = %v, want zero value colorRed", got)
	}
	if got := FieldList[int](f2, "list"); got != nil {
		t.Errorf("FieldList() after error = %v, want nil", got)
	}
	if _, ok := FieldObject[string](f2, "obj"); ok {
		t.Error("FieldObject() after error = true, want false")
	}
}

// ---- from gameduration_test.go ----
func TestParseGameDuration(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr bool
	}{
		{name: "seconds", in: "120sec", want: 120 * time.Second},
		{name: "minutes", in: "5min", want: 5 * time.Minute},
		{name: "hours", in: "2hour", want: 2 * time.Hour},
		{name: "no sentinel", in: "no", want: -time.Second},
		{name: "no sentinel case-insensitive", in: "NO", want: -time.Second},
		{name: "unrecognized suffix defaults to zero", in: "120", want: 0},
		{name: "empty defaults to zero", in: "", want: 0},
		{name: "malformed number is an error", in: "abcsec", wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseGameDuration(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ParseGameDuration(%q) error = nil, want error", c.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseGameDuration(%q) error = %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("ParseGameDuration(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// ---- from legacyhash_test.go ----
// cases are known-good vectors for the algorithm, not values re-derived
// from the formula under test.
func TestLegacyStringHash(t *testing.T) {
	cases := []struct {
		in   string
		want int32
	}{
		{"", 0},
		{"a", 97},
		{"hello", 99162322},
		{"StatSet", -232503986},
		{"multisell/1.xml", 1568074198},
		{"Hello, World!", 1498789909},
		{"The quick brown fox jumps over the lazy dog", -609428141},
		{"123456", 1450575459},
		{"日本語", 25921943},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", -1203646688},
	}

	for _, c := range cases {
		if got := LegacyStringHash(c.in); got != c.want {
			t.Errorf("LegacyStringHash(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// ---- from statset_test.go ----
func TestStatSetGettersFromTypedValues(t *testing.T) {
	s := NewStatSet()
	s.Set("bool", true)
	s.Set("byte", byte(5))
	s.Set("float64", 3.14)
	s.Set("float", float32(2.5))
	s.Set("int", 42)
	s.Set("int64", int64(9000000000))
	s.Set("string", "hi")

	if got, err := s.GetBool("bool"); err != nil || !got {
		t.Errorf("GetBool = (%v, %v), want (true, nil)", got, err)
	}
	if got, err := s.GetByte("byte"); err != nil || got != 5 {
		t.Errorf("GetByte = (%v, %v), want (5, nil)", got, err)
	}
	if got, err := s.GetFloat64("float64"); err != nil || got != 3.14 {
		t.Errorf("GetFloat64 = (%v, %v), want (3.14, nil)", got, err)
	}
	if got, err := s.GetFloat32("float"); err != nil || got != 2.5 {
		t.Errorf("GetFloat32 = (%v, %v), want (2.5, nil)", got, err)
	}
	if got, err := s.GetInt("int"); err != nil || got != 42 {
		t.Errorf("GetInt = (%v, %v), want (42, nil)", got, err)
	}
	if got, err := s.GetInt64("int64"); err != nil || got != 9000000000 {
		t.Errorf("GetInt64 = (%v, %v), want (9000000000, nil)", got, err)
	}
	if got, err := s.GetString("string"); err != nil || got != "hi" {
		t.Errorf("GetString = (%v, %v), want (hi, nil)", got, err)
	}
}

func TestStatSetGetInt64PreservesInt64PrecisionBeyondFloat64Mantissa(t *testing.T) {
	s := NewStatSet()
	want := int64(1) << 60
	s.Set("k", want)

	if got, err := s.GetInt64("k"); err != nil || got != want {
		t.Errorf("GetInt64 = (%v, %v), want (%v, nil)", got, err, want)
	}
	if got, err := s.GetInt64Default("k", 0); err != nil || got != want {
		t.Errorf("GetInt64Default = (%v, %v), want (%v, nil)", got, err, want)
	}
	if got, err := s.GetInt64Array("k"); err != nil || len(got) != 1 || got[0] != want {
		t.Errorf("GetInt64Array = (%v, %v), want ([%v], nil)", got, err, want)
	}
}

func TestStatSetIntegerAccessorsRecognizeUnsignedKinds(t *testing.T) {
	s := NewStatSet()
	s.Set("uint", uint(5))
	s.Set("uint32", uint32(6))
	s.Set("uint64", uint64(7))

	if got, err := s.GetInt("uint"); err != nil || got != 5 {
		t.Errorf("GetInt(uint) = (%v, %v), want (5, nil)", got, err)
	}
	if got, err := s.GetInt("uint32"); err != nil || got != 6 {
		t.Errorf("GetInt(uint32) = (%v, %v), want (6, nil)", got, err)
	}
	if got, err := s.GetInt64("uint64"); err != nil || got != 7 {
		t.Errorf("GetInt64(uint64) = (%v, %v), want (7, nil)", got, err)
	}
}

func TestStatSetIntegerAccessorsRejectUint64Overflow(t *testing.T) {
	s := NewStatSet()
	s.Set("overflow", uint64(math.MaxInt64)+1)

	checks := []struct {
		name string
		get  func() error
	}{
		{"GetByte", func() error { _, err := s.GetByte("overflow"); return err }},
		{"GetInt", func() error { _, err := s.GetInt("overflow"); return err }},
		{"GetIntArray", func() error { _, err := s.GetIntArray("overflow"); return err }},
		{"GetInt64", func() error { _, err := s.GetInt64("overflow"); return err }},
		{"GetInt64Array", func() error { _, err := s.GetInt64Array("overflow"); return err }},
	}
	for _, check := range checks {
		if err := check.get(); err == nil {
			t.Errorf("%s(uint64 overflow) err = nil, want error", check.name)
		}
	}
}

func TestStatSetGettersFromStringCoercion(t *testing.T) {
	s := NewStatSet()
	s.Set("bool", "true")
	s.Set("int", "42")
	s.Set("int64", "9000000000")
	s.Set("float64", "3.14")
	s.Set("intArray", "1;2;3")
	s.Set("stringArray", "a;b;c")
	s.Set("float64Array", "1.5;2.5")
	s.Set("int64Array", "10;20")

	if got, err := s.GetBool("bool"); err != nil || !got {
		t.Errorf("GetBool = (%v, %v), want (true, nil)", got, err)
	}
	if got, err := s.GetInt("int"); err != nil || got != 42 {
		t.Errorf("GetInt = (%v, %v), want (42, nil)", got, err)
	}
	if got, err := s.GetInt64("int64"); err != nil || got != 9000000000 {
		t.Errorf("GetInt64 = (%v, %v), want (9000000000, nil)", got, err)
	}
	if got, err := s.GetFloat64("float64"); err != nil || got != 3.14 {
		t.Errorf("GetFloat64 = (%v, %v), want (3.14, nil)", got, err)
	}
	if got, err := s.GetIntArray("intArray"); err != nil || !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("GetIntArray = (%v, %v), want ([1 2 3], nil)", got, err)
	}
	if got, err := s.GetStringArray("stringArray"); err != nil || !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("GetStringArray = (%v, %v), want ([a b c], nil)", got, err)
	}
	if got, err := s.GetFloat64Array("float64Array"); err != nil || !reflect.DeepEqual(got, []float64{1.5, 2.5}) {
		t.Errorf("GetFloat64Array = (%v, %v), want ([1.5 2.5], nil)", got, err)
	}
	if got, err := s.GetInt64Array("int64Array"); err != nil || !reflect.DeepEqual(got, []int64{10, 20}) {
		t.Errorf("GetInt64Array = (%v, %v), want ([10 20], nil)", got, err)
	}
}

func TestStatSetDefaults(t *testing.T) {
	s := NewStatSet()

	if got, err := s.GetIntDefault("missing", 7); err != nil || got != 7 {
		t.Errorf("GetIntDefault = (%d, %v), want (7, nil)", got, err)
	}
	if got := s.GetStringDefault("missing", "fallback"); got != "fallback" {
		t.Errorf("GetStringDefault = %q, want fallback", got)
	}
	if got := s.GetBoolDefault("missing", true); !got {
		t.Errorf("GetBoolDefault = false, want true")
	}
	if got, err := s.GetIntArrayDefault("missing", []int{1, 2}); err != nil || !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("GetIntArrayDefault = (%v, %v), want ([1 2], nil)", got, err)
	}
	if got := s.GetStringArrayDefault("missing", []string{"x"}); !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("GetStringArrayDefault = %v, want [x]", got)
	}
	if got, err := s.GetByteDefault("missing", 3); err != nil || got != 3 {
		t.Errorf("GetByteDefault = (%d, %v), want (3, nil)", got, err)
	}
	if got, err := s.GetInt64Default("missing", 9); err != nil || got != 9 {
		t.Errorf("GetInt64Default = (%d, %v), want (9, nil)", got, err)
	}
	if got, err := s.GetFloat64Default("missing", 1.5); err != nil || got != 1.5 {
		t.Errorf("GetFloat64Default = (%v, %v), want (1.5, nil)", got, err)
	}
	if got, err := s.GetFloat32Default("missing", 2.5); err != nil || got != 2.5 {
		t.Errorf("GetFloat32Default = (%v, %v), want (2.5, nil)", got, err)
	}
	if got, err := s.GetInt32Default("missing", 11); err != nil || got != 11 {
		t.Errorf("GetInt32Default = (%v, %v), want (11, nil)", got, err)
	}
}

// TestStatSetDefaultsRejectMalformedValues pins the boundary between "key
// absent" (the default substitutes) and "value present but unparsable" (an
// error): a mangled number in a data file must never silently read back as
// the default.
func TestStatSetDefaultsRejectMalformedValues(t *testing.T) {
	s := NewStatSet()
	s.Set("bad", "not-a-number")

	if _, err := s.GetIntDefault("bad", 7); err == nil {
		t.Errorf("GetIntDefault(bad) err = nil, want error")
	}
	if _, err := s.GetByteDefault("bad", 7); err == nil {
		t.Errorf("GetByteDefault(bad) err = nil, want error")
	}
	if _, err := s.GetInt64Default("bad", 7); err == nil {
		t.Errorf("GetInt64Default(bad) err = nil, want error")
	}
	if _, err := s.GetFloat64Default("bad", 7); err == nil {
		t.Errorf("GetFloat64Default(bad) err = nil, want error")
	}
	if _, err := s.GetFloat32Default("bad", 7); err == nil {
		t.Errorf("GetFloat32Default(bad) err = nil, want error")
	}
	if _, err := s.GetInt32Default("bad", 7); err == nil {
		t.Errorf("GetInt32Default(bad) err = nil, want error")
	}
	s.Set("overflow", int64(math.MaxInt32)+1)
	if _, err := s.GetInt32Default("overflow", 7); err == nil {
		t.Errorf("GetInt32Default(overflow) err = nil, want error")
	}
	s.Set("badArray", "1;x;3")
	if _, err := s.GetIntArrayDefault("badArray", []int{1}); err == nil {
		t.Errorf("GetIntArrayDefault(badArray) err = nil, want error")
	}

	// Values of a kind the accessor doesn't coerce from still take the
	// default: only a failed parse of a present string is an error.
	s.Set("otherKind", struct{}{})
	if got, err := s.GetIntDefault("otherKind", 7); err != nil || got != 7 {
		t.Errorf("GetIntDefault(otherKind) = (%d, %v), want (7, nil)", got, err)
	}
}

func TestStatSetGetIntErrorsWhenMissing(t *testing.T) {
	s := NewStatSet()
	_, err := s.GetInt("missing")
	if !errors.Is(err, ErrValueRequired) {
		t.Errorf("GetInt(missing) err = %v, want ErrValueRequired", err)
	}
}

func TestStatSetGetStringErrorsWhenMissing(t *testing.T) {
	s := NewStatSet()
	_, err := s.GetString("missing")
	if !errors.Is(err, ErrValueRequired) {
		t.Errorf("GetString(missing) err = %v, want ErrValueRequired", err)
	}
}

func TestStatSetGetIntErrorsOnUnparsableString(t *testing.T) {
	s := NewStatSet()
	s.Set("k", "not-a-number")
	if _, err := s.GetInt("k"); err == nil {
		t.Errorf("GetInt(unparsable) err = nil, want error")
	}
}

func TestStatSetGetInt32(t *testing.T) {
	s := NewStatSet()
	s.Set("ok", "42")
	s.Set("overflow", int64(math.MaxInt32)+1)
	s.Set("underflow", int64(math.MinInt32)-1)

	if got, err := s.GetInt32("ok"); err != nil || got != 42 {
		t.Errorf("GetInt32(ok) = (%v, %v), want (42, nil)", got, err)
	}
	if _, err := s.GetInt32("overflow"); err == nil {
		t.Error("GetInt32(overflow) err = nil, want error")
	}
	if _, err := s.GetInt32("underflow"); err == nil {
		t.Error("GetInt32(underflow) err = nil, want error")
	}
	if _, err := s.GetInt32("missing"); !errors.Is(err, ErrValueRequired) {
		t.Errorf("GetInt32(missing) err = %v, want ErrValueRequired", err)
	}
}

func TestStatSetUnsetAndHas(t *testing.T) {
	s := NewStatSet()
	s.Set("k", 1)
	if !s.Has("k") {
		t.Errorf("Has(k) = false, want true")
	}
	s.Unset("k")
	if s.Has("k") {
		t.Errorf("Has(k) after Unset = true, want false")
	}
}

func TestNewStatSetFromCopies(t *testing.T) {
	s := NewStatSet()
	s.Set("k", 1)

	copySet := NewStatSetFrom(s)
	copySet.Set("k", 2)

	if got, err := s.GetInt("k"); err != nil || got != 1 {
		t.Errorf("original mutated: GetInt(k) = (%v, %v), want (1, nil)", got, err)
	}
	if got, err := copySet.GetInt("k"); err != nil || got != 2 {
		t.Errorf("copy GetInt(k) = (%v, %v), want (2, nil)", got, err)
	}
}

func TestStatSetGetListAndGetMap(t *testing.T) {
	s := NewStatSet()
	s.Set("list", []int{1, 2, 3})
	s.Set("map", map[string]int{"a": 1})

	if got, err := GetList[int](s, "list"); err != nil || !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("GetList = (%v, %v), want ([1 2 3], nil)", got, err)
	}
	if got, err := GetList[int](s, "missing"); err != nil || got != nil {
		t.Errorf("GetList(missing) = (%v, %v), want (nil, nil)", got, err)
	}
	if _, err := GetList[string](s, "list"); err == nil {
		t.Errorf("GetList[string](list) err = nil, want error (wrong element type)")
	}

	if got, err := GetMap[string, int](s, "map"); err != nil || !reflect.DeepEqual(got, map[string]int{"a": 1}) {
		t.Errorf("GetMap = (%v, %v), want (map[a:1], nil)", got, err)
	}
	if got, err := GetMap[string, int](s, "missing"); err != nil || got != nil {
		t.Errorf("GetMap(missing) = (%v, %v), want (nil, nil)", got, err)
	}
}

type testColor int

const (
	colorRed testColor = iota
	colorGreen
	colorBlue
)

var testColorNames = map[string]testColor{
	"RED":   colorRed,
	"GREEN": colorGreen,
	"BLUE":  colorBlue,
}

func TestStatSetGetEnum(t *testing.T) {
	s := NewStatSet()
	s.Set("fromString", "GREEN")
	s.Set("fromTyped", colorBlue)
	s.Set("bad", "PURPLE")

	if got, err := GetEnum(s, "fromString", testColorNames); err != nil || got != colorGreen {
		t.Errorf("GetEnum(fromString) = (%v, %v), want (%v, nil)", got, err, colorGreen)
	}
	if got, err := GetEnum(s, "fromTyped", testColorNames); err != nil || got != colorBlue {
		t.Errorf("GetEnum(fromTyped) = (%v, %v), want (%v, nil)", got, err, colorBlue)
	}
	if _, err := GetEnum(s, "bad", testColorNames); !errors.Is(err, ErrValueRequired) {
		t.Errorf("GetEnum(bad) err = %v, want ErrValueRequired", err)
	}
	if _, err := GetEnum(s, "missing", testColorNames); !errors.Is(err, ErrValueRequired) {
		t.Errorf("GetEnum(missing) err = %v, want ErrValueRequired", err)
	}
}

func TestStatSetGetEnumDefault(t *testing.T) {
	s := NewStatSet()
	s.Set("fromString", "BLUE")
	s.Set("unknownName", "PURPLE")

	if got, err := GetEnumDefault(s, "fromString", testColorNames, colorRed); err != nil || got != colorBlue {
		t.Errorf("GetEnumDefault(fromString) = (%v, %v), want (%v, nil)", got, err, colorBlue)
	}
	if got, err := GetEnumDefault(s, "missing", testColorNames, colorRed); err != nil || got != colorRed {
		t.Errorf("GetEnumDefault(missing) = (%v, %v), want (%v, nil)", got, err, colorRed)
	}
	if _, err := GetEnumDefault(s, "unknownName", testColorNames, colorGreen); !errors.Is(err, ErrValueRequired) {
		t.Errorf("GetEnumDefault(unknownName) err = %v, want ErrValueRequired", err)
	}
}

func TestStatSetGetObject(t *testing.T) {
	s := NewStatSet()
	s.Set("k", 42)

	if got, ok := GetObject[int](s, "k"); !ok || got != 42 {
		t.Errorf("GetObject = (%v, %v), want (42, true)", got, ok)
	}
	if _, ok := GetObject[string](s, "k"); ok {
		t.Errorf("GetObject with wrong type: ok = true, want false")
	}
	if _, ok := GetObject[int](s, "missing"); ok {
		t.Errorf("GetObject(missing): ok = true, want false")
	}
}
