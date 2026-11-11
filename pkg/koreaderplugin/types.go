package koreaderplugin

// ServerInfo is what every successful plugin response says about the
// server, so the plugin can decide whether to update (ADR 0016).
type ServerInfo struct {
	// ProtocolMin and ProtocolMax are the plugin protocol versions the
	// server accepts. A plugin outside them gets 426 Upgrade Required.
	ProtocolMin int `json:"protocol_min"`
	ProtocolMax int `json:"protocol_max"`
	// PluginVersion and PluginHash name the plugin the server serves, and
	// PluginProtocol is the protocol that plugin speaks.
	PluginVersion  string `json:"plugin_version"`
	PluginHash     string `json:"plugin_hash"`
	PluginProtocol int    `json:"plugin_protocol"`
}

// PairPayload exchanges a Device short code for the Device's sign-in.
type PairPayload struct {
	Code string `json:"code" mod:"trim,lcase" validate:"required,max=32"`
}

// SignInResponse is what the plugin keeps to sign in to a server as a
// KOReader Device.
type SignInResponse struct {
	Username   string     `json:"username"`
	Credential string     `json:"credential"`
	DeviceID   string     `json:"device_id"`
	DeviceName string     `json:"device_name"`
	Server     ServerInfo `json:"server"`
}

// StatusResponse answers a signed-in plugin's check-in.
type StatusResponse struct {
	DeviceID   string     `json:"device_id"`
	DeviceName string     `json:"device_name"`
	Server     ServerInfo `json:"server"`
}
