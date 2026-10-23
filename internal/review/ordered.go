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
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	token, err := decoder.Token()
	if err != nil {
		return err
	}

	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return fmt.Errorf("expected an object")
	}

	entries := OrderedMap{}

	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}

		var value any
		if err := decoder.Decode(&value); err != nil {
			return err
		}

		// Inside an object the decoder only yields string keys.
		entries = append(entries, KV{Key: keyToken.(string), Value: value})
	}

	*m = entries

	return nil
}

// MarshalJSON writes the entries as a JSON object in order.
func (m OrderedMap) MarshalJSON() ([]byte, error) {
	var object bytes.Buffer

	object.WriteByte('{')

	for i, entry := range m {
		if i > 0 {
			object.WriteByte(',')
		}

		// Marshalling a string cannot fail.
		key, _ := json.Marshal(entry.Key)

		value, err := json.Marshal(entry.Value)
		if err != nil {
			return nil, err
		}

		object.Write(key)
		object.WriteByte(':')
		object.Write(value)
	}

	object.WriteByte('}')

	return object.Bytes(), nil
}

// String returns the value as display text.
func (kv KV) String() string {
	switch value := kv.Value.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	default:
		return fmt.Sprint(value)
	}
}
