/*
  Copyright 2023 Amazon.com, Inc. or its affiliates. All Rights Reserved.

  Licensed under the Apache License, Version 2.0 (the "License").
  You may not use this file except in compliance with the License.
  A copy of the License is located at

      http://www.apache.org/licenses/LICENSE-2.0

  or in the "license" file accompanying this file. This file is distributed
  on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
  express or implied. See the License for the specific language governing
  permissions and limitations under the License.
*/

package containercredentials

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

func (f *FakeConfig) Get(namespace string, serviceAccount string) *PatchConfig {
	key := Identity{
		Namespace:      namespace,
		ServiceAccount: serviceAccount,
	}
	if _, ok := f.Identities[key]; ok {
		return &PatchConfig{
			Audience: f.ContainerCredentialsAudience,
			FullUri:  f.ContainerCredentialsFullUri,
		}
	}

	return nil
}
