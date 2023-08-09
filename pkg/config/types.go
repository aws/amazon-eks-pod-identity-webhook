package config

type IdentityConfigObject struct {
	Identities []Identity `json:"identities,omitempty"`
}

type Identity struct {
	Namespace      string `json:"namespace"`
	ServiceAccount string `json:"serviceAccount"`
}
