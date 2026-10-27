package models

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/pkg/errors"
)

// StringList is a list of strings stored in one TEXT column as a JSON array.
// A nil list is stored as an empty one and always scans back non-nil, so it
// serializes to [] rather than null.
type StringList []string

func (l StringList) Value() (driver.Value, error) {
	if l == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]string(l))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return string(b), nil
}

func (l *StringList) Scan(src any) error {
	var raw []byte
	switch v := src.(type) {
	case nil:
		*l = StringList{}
		return nil
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return errors.Errorf("cannot scan %T into StringList", src)
	}
	list := []string{}
	if err := json.Unmarshal(raw, &list); err != nil {
		return errors.WithStack(err)
	}
	*l = list
	return nil
}
