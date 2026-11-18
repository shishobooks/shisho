package gettingstarted

// Getting Started steps, in the order the panel lists them.
const (
	//tygo:emit export type GettingStartedStepKey = typeof StepLibrary | typeof StepPlugins | typeof StepInvite | typeof StepDevice | typeof StepToRead | typeof StepStartReading;
	StepLibrary      = "library"
	StepPlugins      = "plugins"
	StepInvite       = "invite"
	StepDevice       = "device"
	StepToRead       = "to_read"
	StepStartReading = "start_reading"
)

// GettingStartedStep is one step's status for the signed-in user. Applies
// says whether the step is offered to them now: their role allows it and
// what it needs exists, such as a library they can reach. Done and NotForMe
// are false for a step that doesn't apply.
//
//nolint:revive // named for Getting Started, as generated TS types are global
type GettingStartedStep struct {
	Key      string `json:"key" tstype:"GettingStartedStepKey"`
	Applies  bool   `json:"applies"`
	Done     bool   `json:"done"`
	NotForMe bool   `json:"not_for_me"`
}

// GettingStartedResponse is the signed-in user's Getting Started list.
// Enabled is false for a user it was never turned on for, and always in Demo
// Mode. Steps is empty while the list is off or dismissed. AutoOpen asks the client to open the panel once
// and then clear it.
//
//nolint:revive // GettingStartedResponse name is mandated by the {Entity}Response convention (ADR 0004)
type GettingStartedResponse struct {
	Enabled   bool                 `json:"enabled"`
	Dismissed bool                 `json:"dismissed"`
	AutoOpen  bool                 `json:"auto_open"`
	Steps     []GettingStartedStep `json:"steps"`
}
