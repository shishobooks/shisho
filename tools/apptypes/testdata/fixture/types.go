// Package fixture holds the shapes the generator handles.
package fixture

import (
	"encoding/json"
	"time"

	"github.com/shishobooks/shisho/tools/apptypes/testdata/other"
)

const (
	//tygo:emit export type Kind = typeof KindBook | typeof KindDefault;
	KindBook    = "book"
	KindDefault = "default"
)

//tygo:emit export type KindAlias = Kind;

// Widget is documented.
type Widget struct {
	other.Base `tstype:",extends"`
	// Shadow is shallower than Base's, so it wins.
	Shadow     int             `json:"shadow"`
	Name       string          `json:"name"`
	Maybe      *string         `json:"maybe,omitempty"`
	Kind       string          `json:"kind" tstype:"Kind"`
	Kinds      []string        `json:"kinds" tstype:"KindAlias[]"`
	Color      *string         `json:"color" tstype:"Color | null"`
	At         time.Time       `json:"at"`
	Size       int64           `json:"size"`
	Small      int32           `json:"small"`
	Ratio      float64         `json:"ratio"`
	Tags       map[string]int  `json:"tags"`
	Raw        json.RawMessage `json:"raw"`
	Any        any             `json:"any"`
	Shared     other.Shared    `json:"shared"`
	Mine       Shared          `json:"mine"`
	Parent     *Widget         `json:"parent"`
	Children   []*Widget       `json:"children"`
	ByID       map[int]*Widget `json:"by_id"`
	Hidden     string          `json:"-"`
	Skipped    string          `json:"skipped" tstype:"-"`
	unexported string
	NoTag      bool
}

// Shared is named like other.Shared.
type Shared struct {
	Default string `json:"default"`
}

// Generic types aren't generated.
type Page[T any] struct {
	Items []T `json:"items"`
}

// Count is a named basic type.
type Count int
