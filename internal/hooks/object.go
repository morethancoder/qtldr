package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// object is a JSON object that keeps its key order, so rewriting a user's
// settings file does not reshuffle it.
type object struct {
	keys []string
	vals map[string]json.RawMessage
}

func (o *object) get(k string) (json.RawMessage, bool) {
	v, ok := o.vals[k]
	return v, ok
}

func (o *object) set(k string, v json.RawMessage) {
	if o.vals == nil {
		o.vals = map[string]json.RawMessage{}
	}
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

// UnmarshalJSON reads keys in document order.
func (o *object) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return fmt.Errorf("want an object")
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return err
		}
		o.set(t.(string), v)
	}
	_, err := dec.Token()
	return err
}

// MarshalJSON writes keys in their original order.
func (o object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(o.vals[k])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
