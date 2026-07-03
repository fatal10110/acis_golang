package commons

import (
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

	if !s.GetBool("bool") {
		t.Errorf("GetBool = false, want true")
	}
	if got := s.GetByte("byte"); got != 5 {
		t.Errorf("GetByte = %d, want 5", got)
	}
	if got := s.GetDouble("double"); got != 3.14 {
		t.Errorf("GetDouble = %v, want 3.14", got)
	}
	if got := s.GetFloat32("float"); got != 2.5 {
		t.Errorf("GetFloat32 = %v, want 2.5", got)
	}
	if got := s.GetInt("int"); got != 42 {
		t.Errorf("GetInt = %d, want 42", got)
	}
	if got := s.GetLong("long"); got != 9000000000 {
		t.Errorf("GetLong = %d, want 9000000000", got)
	}
	if got := s.GetString("string"); got != "hi" {
		t.Errorf("GetString = %q, want hi", got)
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

	if !s.GetBool("bool") {
		t.Errorf("GetBool = false, want true")
	}
	if got := s.GetInt("int"); got != 42 {
		t.Errorf("GetInt = %d, want 42", got)
	}
	if got := s.GetLong("long"); got != 9000000000 {
		t.Errorf("GetLong = %d, want 9000000000", got)
	}
	if got := s.GetDouble("double"); got != 3.14 {
		t.Errorf("GetDouble = %v, want 3.14", got)
	}
	if got := s.GetIntArray("intArray"); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("GetIntArray = %v, want [1 2 3]", got)
	}
	if got := s.GetStringArray("stringArray"); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("GetStringArray = %v, want [a b c]", got)
	}
	if got := s.GetDoubleArray("doubleArray"); !reflect.DeepEqual(got, []float64{1.5, 2.5}) {
		t.Errorf("GetDoubleArray = %v, want [1.5 2.5]", got)
	}
	if got := s.GetLongArray("longArray"); !reflect.DeepEqual(got, []int64{10, 20}) {
		t.Errorf("GetLongArray = %v, want [10 20]", got)
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

func TestStatSetGetIntPanicsWhenMissing(t *testing.T) {
	s := NewStatSet()
	defer func() {
		if recover() == nil {
			t.Errorf("GetInt on missing key: expected panic, got none")
		}
	}()
	s.GetInt("missing")
}

func TestStatSetGetStringPanicsWhenMissing(t *testing.T) {
	s := NewStatSet()
	defer func() {
		if recover() == nil {
			t.Errorf("GetString on missing key: expected panic, got none")
		}
	}()
	s.GetString("missing")
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

	if got := s.GetInt("k"); got != 1 {
		t.Errorf("original mutated: GetInt(k) = %d, want 1", got)
	}
	if got := copySet.GetInt("k"); got != 2 {
		t.Errorf("copy GetInt(k) = %d, want 2", got)
	}
}

func TestStatSetGetListAndGetMap(t *testing.T) {
	s := NewStatSet()
	s.Set("list", []int{1, 2, 3})
	s.Set("map", map[string]int{"a": 1})

	if got := GetList[int](s, "list"); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("GetList = %v, want [1 2 3]", got)
	}
	if got := GetList[int](s, "missing"); got != nil {
		t.Errorf("GetList(missing) = %v, want nil", got)
	}

	if got := GetMap[string, int](s, "map"); !reflect.DeepEqual(got, map[string]int{"a": 1}) {
		t.Errorf("GetMap = %v, want map[a:1]", got)
	}
	if got := GetMap[string, int](s, "missing"); got != nil {
		t.Errorf("GetMap(missing) = %v, want nil", got)
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
