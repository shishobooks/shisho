// Package other declares a type with the same name as one in fixture, which
// each keeps in its own namespace.
package other

// Shared is named like fixture.Shared.
type Shared struct {
	Note string `json:"note"`
}

//tygo:emit export type Color = "red" | "green";

// Base is embedded by fixture.Widget.
type Base struct {
	ID      int    `json:"id"`
	Shadow  string `json:"shadow"`
	Ignored string `json:"-"`
}
