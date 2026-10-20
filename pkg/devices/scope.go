package devices

// Scope is the set of books a Device reaches through every connection: all
// the owner's books, one library, or one list.
type Scope struct {
	Type      string
	LibraryID *int
	ListID    *int
}

// Scope returns the Device's scope.
func (d *Device) Scope() Scope {
	return Scope{Type: d.ScopeType, LibraryID: d.ScopeLibraryID, ListID: d.ScopeListID}
}
