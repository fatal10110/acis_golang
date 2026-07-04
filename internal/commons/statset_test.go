package commons

import (
	"errors"
	"reflect"
	"testing"
)

func TestStatSetGettersFromTypedValues(t *testing.T) {
	s := NewStatSet()
	s.Set("bool", true)
	s.Set("byte", byte(5))
	s.Set("double", 3.14)
	s.Set("float", float32(2.5))
	s.Set("int", 42)
	s.Set("long", int64(9000000000))
	s.Set("string", "hi")

	if got, err := s.GetBool("bool"); err != nil || !got {
		t.Errorf("GetBool = (%v, %v), want (true, nil)", got, err)
	}
	if got, err := s.GetByte("byte"); err != nil || got != 5 {
		t.Errorf("GetByte = (%v, %v), want (5, nil)", got, err)
	}
	if got, err := s.GetDouble("double"); err != nil || got != 3.14 {
		t.Errorf("GetDouble = (%v, %v), want (3.14, nil)", got, err)
	}
	if got, err := s.GetFloat32("float"); err != nil || got != 2.5 {
		t.Errorf("GetFloat32 = (%v, %v), want (2.5, nil)", got, err)
	}
	if got, err := s.GetInt("int"); err != nil || got != 42 {
		t.Errorf("GetInt = (%v, %v), want (42, nil)", got, err)
	}
	if got, err := s.GetLong("long"); err != nil || got != 9000000000 {
		t.Errorf("GetLong = (%v, %v), want (9000000000, nil)", got, err)
	}
	if got, err := s.GetString("string"); err != nil || got != "hi" {
		t.Errorf("GetString = (%v, %v), want (hi, nil)", got, err)
	}
}

func TestStatSetGettersFromStringCoercion(t *testing.T) {
	s := NewStatSet()
	s.Set("bool", "true")
	s.Set("int", "42")
	s.Set("long", "9000000000")
	s.Set("double", "3.14")
	s.Set("intArray", "1;2;3")
	s.Set("stringArray", "a;b;c")
	s.Set("doubleArray", "1.5;2.5")
	s.Set("longArray", "10;20")

	if got, err := s.GetBool("bool"); err != nil || !got {
		t.Errorf("GetBool = (%v, %v), want (true, nil)", got, err)
	}
	if got, err := s.GetInt("int"); err != nil || got != 42 {
		t.Errorf("GetInt = (%v, %v), want (42, nil)", got, err)
	}
	if got, err := s.GetLong("long"); err != nil || got != 9000000000 {
		t.Errorf("GetLong = (%v, %v), want (9000000000, nil)", got, err)
	}
	if got, err := s.GetDouble("double"); err != nil || got != 3.14 {
		t.Errorf("GetDouble = (%v, %v), want (3.14, nil)", got, err)
	}
	if got, err := s.GetIntArray("intArray"); err != nil || !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("GetIntArray = (%v, %v), want ([1 2 3], nil)", got, err)
	}
	if got, err := s.GetStringArray("stringArray"); err != nil || !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("GetStringArray = (%v, %v), want ([a b c], nil)", got, err)
	}
	if got, err := s.GetDoubleArray("doubleArray"); err != nil || !reflect.DeepEqual(got, []float64{1.5, 2.5}) {
		t.Errorf("GetDoubleArray = (%v, %v), want ([1.5 2.5], nil)", got, err)
	}
	if got, err := s.GetLongArray("longArray"); err != nil || !reflect.DeepEqual(got, []int64{10, 20}) {
		t.Errorf("GetLongArray = (%v, %v), want ([10 20], nil)", got, err)
	}
}

func TestStatSetDefaults(t *testing.T) {
	s := NewStatSet()

	if got := s.GetIntDefault("missing", 7); got != 7 {
		t.Errorf("GetIntDefault = %d, want 7", got)
	}
	if got := s.GetStringDefault("missing", "fallback"); got != "fallback" {
		t.Errorf("GetStringDefault = %q, want fallback", got)
	}
	if got := s.GetBoolDefault("missing", true); !got {
		t.Errorf("GetBoolDefault = false, want true")
	}
	if got := s.GetIntArrayDefault("missing", []int{1, 2}); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("GetIntArrayDefault = %v, want [1 2]", got)
	}
	if got := s.GetStringArrayDefault("missing", []string{"x"}); !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("GetStringArrayDefault = %v, want [x]", got)
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
