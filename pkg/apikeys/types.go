package apikeys

// CreateAPIKeyPayload is the request body for POST /user/api-keys.
type CreateAPIKeyPayload struct {
	Name string `json:"name"`
}

// UpdateAPIKeyNamePayload is the request body for PATCH /user/api-keys/:id.
type UpdateAPIKeyNamePayload struct {
	Name string `json:"name"`
}
