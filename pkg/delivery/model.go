package delivery

import "github.com/uptrace/bun"

// UserDeliveryRule is one rule of a user's customized default Delivery
// Rules. A user with no rows follows BuiltInRules.
type UserDeliveryRule struct {
	bun.BaseModel `bun:"table:user_delivery_rules,alias:udr"`

	UserID            int    `bun:"user_id,pk"`
	SourceFileType    string `bun:"source_file_type,pk"`
	DeliveredFileType string `bun:"delivered_file_type,notnull"`
	Position          int    `bun:"position,notnull"`
}
