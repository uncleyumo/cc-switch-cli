package ccstate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// Value is one JSON value: *Object, []Value, string, json.Number, bool or nil.
//
// A plain map[string]any would be enough to read a settings file, but writing
// it back would reorder every key and produce a whole-file diff on every
// apply. Keeping insertion order is what lets ccsc touch eight keys and leave
// the rest of the file byte-identical.
type Value any

// Object is an insertion-ordered JSON object.
type Object struct {
	keys []string
	vals map[string]Value
}

// NewObject returns an empty object.
func NewObject() *Object { return &Object{vals: map[string]Value{}} }

// Len returns the number of keys.
func (o *Object) Len() int {
	if o == nil {
		return 0
	}
	return len(o.keys)
}

// Keys returns the keys in file order.
func (o *Object) Keys() []string {
	if o == nil {
		return nil
	}
	out := make([]string, len(o.keys))
	copy(out, o.keys)
	return out
}

// Get returns the value stored under k.
func (o *Object) Get(k string) (Value, bool) {
	if o == nil {
		return nil, false
	}
	v, ok := o.vals[k]
	return v, ok
}

// Has reports whether k is present.
func (o *Object) Has(k string) bool {
	_, ok := o.Get(k)
	return ok
}

// Set stores v under k, appending the key when it is new.
func (o *Object) Set(k string, v Value) {
	if o.vals == nil {
		o.vals = map[string]Value{}
	}
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

// Delete removes k and reports whether it was present.
func (o *Object) Delete(k string) bool {
	if o == nil {
		return false
	}
	if _, ok := o.vals[k]; !ok {
		return false
	}
	delete(o.vals, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i:i], o.keys[i+1:]...)
			break
		}
	}
	return true
}

// Object returns the nested object at k, or nil.
func (o *Object) Object(k string) *Object {
	v, ok := o.Get(k)
	if !ok {
		return nil
	}
	obj, _ := v.(*Object)
	return obj
}

// EnsureObject returns the nested object at k, creating (or replacing a
// non-object value with) an empty one.
func (o *Object) EnsureObject(k string) *Object {
	if obj := o.Object(k); obj != nil {
		return obj
	}
	obj := NewObject()
	o.Set(k, obj)
	return obj
}

// String returns the string stored under k.
func (o *Object) String(k string) (string, bool) {
	v, ok := o.Get(k)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// Number returns the integer stored under k.
func (o *Object) Number(k string) (int64, bool) {
	v, ok := o.Get(k)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return i, true
	case float64:
		return int64(n), true
	}
	return 0, false
}

// NewNumber wraps an integer as a JSON number, so it round-trips without
// picking up a trailing ".0".
func NewNumber(n int64) json.Number { return json.Number(strconv.FormatInt(n, 10)) }

// Parse decodes data into an ordered Value tree.
func Parse(data []byte) (Value, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data after the JSON value")
	}
	return v, nil
}

func parseValue(dec *json.Decoder) (Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := NewObject()
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyTok.(string)
				if !ok {
					return nil, fmt.Errorf("object key is not a string")
				}
				val, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				obj.Set(key, val)
			}
			if _, err := dec.Token(); err != nil { // closing '}'
				return nil, err
			}
			return obj, nil
		case '[':
			arr := []Value{}
			for dec.More() {
				val, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, val)
			}
			if _, err := dec.Token(); err != nil { // closing ']'
				return nil, err
			}
			return arr, nil
		}
		return nil, fmt.Errorf("unexpected delimiter %q", t)
	case string, json.Number, bool, nil:
		return t, nil
	}
	return nil, fmt.Errorf("unexpected JSON token %v", tok)
}

// Marshal encodes v with the given indent unit.
func Marshal(v Value, indent string) ([]byte, error) {
	if indent == "" {
		indent = "  "
	}
	var buf bytes.Buffer
	if err := writeValue(&buf, v, indent, ""); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeValue(buf *bytes.Buffer, v Value, indent, cur string) error {
	switch t := v.(type) {
	case *Object:
		if t.Len() == 0 {
			buf.WriteString("{}")
			return nil
		}
		buf.WriteString("{\n")
		for i, k := range t.keys {
			buf.WriteString(cur)
			buf.WriteString(indent)
			kb, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(kb)
			buf.WriteString(": ")
			if err := writeValue(buf, t.vals[k], indent, cur+indent); err != nil {
				return err
			}
			if i < len(t.keys)-1 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
		}
		buf.WriteString(cur)
		buf.WriteString("}")
		return nil
	case []Value:
		if len(t) == 0 {
			buf.WriteString("[]")
			return nil
		}
		buf.WriteString("[\n")
		for i, e := range t {
			buf.WriteString(cur)
			buf.WriteString(indent)
			if err := writeValue(buf, e, indent, cur+indent); err != nil {
				return err
			}
			if i < len(t)-1 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
		}
		buf.WriteString(cur)
		buf.WriteString("]")
		return nil
	case nil:
		buf.WriteString("null")
		return nil
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		return nil
	case string:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
		return nil
	case json.Number:
		buf.WriteString(t.String())
		return nil
	case int:
		buf.WriteString(strconv.Itoa(t))
		return nil
	case int64:
		buf.WriteString(strconv.FormatInt(t, 10))
		return nil
	case float64:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
		return nil
	}
	return fmt.Errorf("cannot encode a value of type %T", v)
}
