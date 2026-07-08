package commons

import (
	"errors"
	"reflect"
	"testing"
)

func TestFieldsMandatoryAccessors(t *testing.T) {
	s := NewStatSet()
	s.Set("int", 42)
	s.Set("long", int64(9000000000))
	s.Set("double", 3.14)
	s.Set("float", float32(2.5))
	s.Set("string", "hi")
	s.Set("intArray", []int{1, 2, 3})
	s.Set("doubleArray", []float64{1.5, 2.5})
	s.Set("stringArray", []string{"a", "b"})

	f := NewFields(s, "test")
	if got := f.Int("int"); got != 42 {
		t.Errorf("Int() = %v, want 42", got)
	}
	if got := f.Long("long"); got != 9000000000 {
		t.Errorf("Long() = %v, want 9000000000", got)
	}
	if got := f.Double("double"); got != 3.14 {
		t.Errorf("Double() = %v, want 3.14", got)
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
	if got := f.DoubleArray("doubleArray"); !reflect.DeepEqual(got, []float64{1.5, 2.5}) {
		t.Errorf("DoubleArray() = %v, want [1.5 2.5]", got)
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
