package httputil

import "encoding/json"

// NullableInt distinguishes between "field absent" (Set=false), "field present
// as null" (Set=true, Value=nil), and "field present with value" (Set=true,
// Value=&n). Standard encoding/json cannot differentiate absent from null on
// pointer types, so we use a custom UnmarshalJSON. Payload fields of this type
// need a `tstype:"number | null"` tag so tygo emits the wire shape.
type NullableInt struct {
	Value *int
	Set   bool
}

func (n *NullableInt) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.Value = nil
		return nil
	}
	var v int
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}
