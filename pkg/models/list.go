package models

import (
	"time"

	"github.com/uptrace/bun"
)

// List permission levels.
const (
	ListPermissionViewer  = "viewer"
	ListPermissionEditor  = "editor"
	ListPermissionManager = "manager"
)

// List default sort options.
const (
	ListSortAddedAtDesc = "added_at_desc"
	ListSortAddedAtAsc  = "added_at_asc"
	ListSortTitleAsc    = "title_asc"
	ListSortTitleDesc   = "title_desc"
	ListSortAuthorAsc   = "author_asc"
	ListSortAuthorDesc  = "author_desc"
	ListSortManual      = "manual"
)

type List struct {
	bun.BaseModel `bun:"table:lists,alias:l" tstype:"-"`

	ID          int       `bun:",pk,nullzero" json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	UserID      int       `bun:",nullzero" json:"user_id"`
	User        *User     `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty" tstype:"User"`
	Name        string    `bun:",nullzero" json:"name"`
	Description *string   `json:"description"`
	IsOrdered   bool      `json:"is_ordered"`
	DefaultSort string    `bun:",nullzero" json:"default_sort"`

	// Relations
	ListBooks  []*ListBook  `bun:"rel:has-many,join:id=list_id" json:"list_books,omitempty" tstype:"ListBook[]"`
	ListShares []*ListShare `bun:"rel:has-many,join:id=list_id" json:"list_shares,omitempty" tstype:"ListShare[]"`
}

type ListBook struct {
	bun.BaseModel `bun:"table:list_books,alias:lb" tstype:"-"`

	ID            int       `bun:",pk,nullzero" json:"id"`
	ListID        int       `bun:",nullzero" json:"list_id"`
	List          *List     `bun:"rel:belongs-to,join:list_id=id" json:"list,omitempty" tstype:"List"`
	BookID        int       `bun:",nullzero" json:"book_id"`
	Book          *Book     `bun:"rel:belongs-to,join:book_id=id" json:"book,omitempty" tstype:"Book"`
	AddedAt       time.Time `json:"added_at"`
	AddedByUserID *int      `json:"added_by_user_id"`
	AddedByUser   *User     `bun:"rel:belongs-to,join:added_by_user_id=id" json:"added_by_user,omitempty" tstype:"User"`
	SortOrder     *int      `json:"sort_order"`
}

type ListShare struct {
	bun.BaseModel `bun:"table:list_shares,alias:ls" tstype:"-"`

	ID             int       `bun:",pk,nullzero" json:"id"`
	ListID         int       `bun:",nullzero" json:"list_id"`
	List           *List     `bun:"rel:belongs-to,join:list_id=id" json:"list,omitempty" tstype:"List"`
	UserID         int       `bun:",nullzero" json:"user_id"`
	User           *User     `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty" tstype:"User"`
	Permission     string    `bun:",nullzero" json:"permission"`
	CreatedAt      time.Time `json:"created_at"`
	SharedByUserID *int      `json:"shared_by_user_id"`
	SharedByUser   *User     `bun:"rel:belongs-to,join:shared_by_user_id=id" json:"shared_by_user,omitempty" tstype:"User"`
}
