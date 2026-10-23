package review

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// KV is one entry of an OrderedMap.
type KV struct {
	Key   string
	Value any
}

// OrderedMap is a JSON object that keeps its keys in file order, which
// matters for language globs where the first match wins.
type OrderedMap []KV

// UnmarshalJSON reads a JSON object keeping key order.
func (m *OrderedMap) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("expected an object")
	}

	out := OrderedMap{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		var value any
		if err := dec.Decode(&value); err != nil {
			return err
		}
		out = append(out, KV{Key: keyTok.(string), Value: value})
	}
	*m = out
	return nil
}

// MarshalJSON writes the entries as a JSON object in order.
func (m OrderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, kv := range m {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, _ := json.Marshal(kv.Key)
		v, err := json.Marshal(kv.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// String returns the value as display text.
func (kv KV) String() string {
	switch v := kv.Value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}
