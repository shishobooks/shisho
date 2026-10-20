package delivery

// Rule delivers every File of type SourceFileType as DeliveredFileType: the
// same File Type, or another one a chain of Conversions reaches.
type Rule struct {
	SourceFileType    string `json:"source_file_type" validate:"required" tstype:"FileType"`
	DeliveredFileType string `json:"delivered_file_type" validate:"required" tstype:"DeliveredFileType"`
}

// RuleOption is a source File Type a rule can name, with every File Type it
// can be delivered as, its own first.
type RuleOption struct {
	SourceFileType     string   `json:"source_file_type" tstype:"FileType"`
	DeliveredFileTypes []string `json:"delivered_file_types" tstype:"DeliveredFileType[]"`
}

// DefaultRulesResponse is the current user's default Delivery Rules, with
// the choices the rules editor offers.
type DefaultRulesResponse struct {
	// FollowsBuiltIn is true while the default follows Shisho's built-in
	// rules, which Rules then holds.
	FollowsBuiltIn bool   `json:"follows_built_in"`
	Rules          []Rule `json:"rules"`
	// Options lists every source File Type a rule can name, in the built-in
	// rules' order.
	Options []RuleOption `json:"options"`
}

// UpdateDeliveryRulesPayload is the request body for PUT
// /user/delivery-rules: the customized list, in order. It can't be empty.
type UpdateDeliveryRulesPayload struct {
	Rules []Rule `json:"rules" validate:"dive" mod:"dive"`
}
