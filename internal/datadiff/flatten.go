package datadiff

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
)

// Flatten reduces v to canonical field strings keyed by a stable field path.
// Exported struct fields recurse by name; slices and arrays recurse by index;
// maps recurse by sorted key. Nil pointers and interfaces are skipped.
func Flatten(v any) (map[string]string, error) {
	fields := make(map[string]string)
	if err := flattenValue(fields, "", reflect.ValueOf(v)); err != nil {
		return nil, err
	}
	return fields, nil
}

func flattenValue(fields map[string]string, prefix string, v reflect.Value) error {
	if !v.IsValid() {
		return nil
	}

	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	if v.CanInterface() {
		if stringer, ok := v.Interface().(fmt.Stringer); ok && v.Kind() != reflect.Struct {
			fields[prefix] = stringer.String()
			return nil
		}
	}

	switch v.Kind() {
	case reflect.Bool:
		fields[prefix] = strconv.FormatBool(v.Bool())
		return nil
	case reflect.String:
		fields[prefix] = v.String()
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fields[prefix] = strconv.FormatInt(v.Int(), 10)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		fields[prefix] = strconv.FormatUint(v.Uint(), 10)
		return nil
	case reflect.Float32, reflect.Float64:
		fields[prefix] = FormatFloat(v.Convert(reflect.TypeOf(float64(0))).Float())
		return nil
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			name := field.Name
			if prefix != "" {
				name = prefix + "." + name
			}
			if err := flattenValue(fields, name, v.Field(i)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Slice, reflect.Array:
		name := "len"
		if prefix != "" {
			name = prefix + ".len"
		}
		fields[name] = strconv.Itoa(v.Len())
		for i := 0; i < v.Len(); i++ {
			name := fmt.Sprintf("%s[%d]", prefix, i)
			if err := flattenValue(fields, name, v.Index(i)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		name := "len"
		if prefix != "" {
			name = prefix + ".len"
		}
		fields[name] = strconv.Itoa(v.Len())
		keys := v.MapKeys()
		sorted := make([]string, len(keys))
		byKey := make(map[string]reflect.Value, len(keys))
		for i, key := range keys {
			name, err := flattenMapKey(key)
			if err != nil {
				return fmt.Errorf("flatten %s: %w", prefix, err)
			}
			sorted[i] = name
			byKey[name] = key
		}
		sort.Strings(sorted)
		for _, name := range sorted {
			fieldName := fmt.Sprintf("%s[%s]", prefix, name)
			if err := flattenValue(fields, fieldName, v.MapIndex(byKey[name])); err != nil {
				return err
			}
		}
		return nil
	default:
		if prefix == "" {
			return fmt.Errorf("unsupported root kind %s", v.Kind())
		}
		if v.CanInterface() {
			fields[prefix] = fmt.Sprint(v.Interface())
			return nil
		}
		return fmt.Errorf("unsupported kind %s at %s", v.Kind(), prefix)
	}
}

func flattenMapKey(v reflect.Value) (string, error) {
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "<nil>", nil
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return "<nil>", nil
	}
	if v.CanInterface() {
		if stringer, ok := v.Interface().(fmt.Stringer); ok && v.Kind() != reflect.Struct {
			return stringer.String(), nil
		}
	}

	switch v.Kind() {
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.String:
		return v.String(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(v.Uint(), 10), nil
	default:
		return "", fmt.Errorf("unsupported map key kind %s", v.Kind())
	}
}
