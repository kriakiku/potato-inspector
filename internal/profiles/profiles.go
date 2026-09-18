package profiles

// Profile is a last-mile shaping snapshot (in-memory only for PotatoNetwork).
type Profile struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	Country           string  `json:"country,omitempty"`
	Tier              string  `json:"tier,omitempty"`
	DelayMs           int     `json:"delayMs"`
	DownloadMbps      float64 `json:"downloadMbps"`
	UploadMbps        float64 `json:"uploadMbps"`
	LossPercent       float64 `json:"lossPercent"`
	Passthrough       bool    `json:"passthrough"`
	EmulationLimited  bool    `json:"emulationLimited,omitempty"`
	Warning           string  `json:"warning,omitempty"`
}

// PassthroughProfile is the boot / clear state.
func PassthroughProfile() Profile {
	return Profile{
		ID:          "passthrough",
		Name:        "Passthrough",
		Description: "No last-mile shaping; no path delay.",
		Passthrough: true,
	}
}
