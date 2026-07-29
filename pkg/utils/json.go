package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
)

func MarshalForceFloat(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := encode(reflect.ValueOf(v), &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encode(v reflect.Value, buf *bytes.Buffer) error {
	if !v.IsValid() {
		buf.WriteString("null")
		return nil
	}

	if v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			buf.WriteString("null")
			return nil
		}
		return encode(v.Elem(), buf)
	}

	switch v.Kind() {

	case reflect.Struct:
		buf.WriteByte('{')

		t := v.Type()
		first := true

		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)

			if !f.IsExported() {
				continue
			}

			if !first {
				buf.WriteByte(',')
			}
			first = false

			name := f.Tag.Get("json")
			if name == "" {
				name = f.Name
			}
			if idx := len(name); idx > 0 {
				for j := 0; j < len(name); j++ {
					if name[j] == ',' {
						name = name[:j]
						break
					}
				}
			}

			key, _ := json.Marshal(name)
			buf.Write(key)
			buf.WriteByte(':')

			if err := encode(v.Field(i), buf); err != nil {
				return err
			}
		}

		buf.WriteByte('}')

	case reflect.Slice, reflect.Array:
		buf.WriteByte('[')
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encode(v.Index(i), buf); err != nil {
				return err
			}
		}
		buf.WriteByte(']')

	case reflect.Map:
		buf.WriteByte('{')
		keys := v.MapKeys()
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			key, _ := json.Marshal(fmt.Sprint(k.Interface()))
			buf.Write(key)
			buf.WriteByte(':')
			if err := encode(v.MapIndex(k), buf); err != nil {
				return err
			}
		}
		buf.WriteByte('}')

	case reflect.String:
		s, _ := json.Marshal(v.String())
		buf.Write(s)

	case reflect.Bool:
		if v.Bool() {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		buf.WriteString(strconv.FormatInt(v.Int(), 10))

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		buf.WriteString(strconv.FormatUint(v.Uint(), 10))

	case reflect.Float32, reflect.Float64:
		f := v.Float()

		if math.IsNaN(f) || math.IsInf(f, 0) {
			return fmt.Errorf("invalid float")
		}

		prec := -1
		if math.Trunc(f) == f {
			prec = 1
		}

		buf.Write(strconv.AppendFloat(nil, f, 'f', prec, 64))

	default:
		b, err := json.Marshal(v.Interface())
		if err != nil {
			return err
		}
		buf.Write(b)
	}

	return nil
}
