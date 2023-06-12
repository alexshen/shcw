package api

import "encoding/json"

// FixedJsonValue makes it possible to unmarshal an json value whose empty
// value is incorrectly encoded as empty string.
type FixedJsonValue[T any] struct {
	Value T
}

func (v *FixedJsonValue[T]) UnmarshalJSON(data []byte) error {
	if len(data) == 2 && data[0] == '"' && data[len(data)-1] == '"' {
		return nil
	}
	return json.Unmarshal(data, &v.Value)
}
