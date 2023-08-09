package config

type FakeConfig struct {
	ContainerCredentialsAudience string
	ContainerCredentialsFullUri  string
	Identities                   map[Identity]bool
}

func NewFakeConfig(containerCredentialsAudience, containerCredentialsFullUri string, identities map[Identity]bool) *FakeConfig {
	return &FakeConfig{
		ContainerCredentialsAudience: containerCredentialsAudience,
		ContainerCredentialsFullUri:  containerCredentialsFullUri,
		Identities:                   identities,
	}
}

func (f *FakeConfig) Get(namespace string, serviceAccount string) *ContainerCredentialsPatchConfig {
	key := Identity{
		Namespace:      namespace,
		ServiceAccount: serviceAccount,
	}
	if _, ok := f.Identities[key]; ok {
		return &ContainerCredentialsPatchConfig{
			Audience: f.ContainerCredentialsAudience,
			FullUri:  f.ContainerCredentialsFullUri,
		}
	}

	return nil
}
