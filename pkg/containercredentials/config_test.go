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

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const (
	namespaceFoo               = "foo"
	namespaceFooServiceAccount = "ns-foo-sa"
	namespaceBar               = "bar"
	namespaceBarServiceAccount = "ns-bar-sa"

	audience = "audience"
	fullUri  = "fullUri"

	defaultTimeout      = 10 * time.Second
	defaultPollInterval = 1 * time.Second
)

func TestFileConfig_Watcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dirPath, err := os.MkdirTemp("", "test")
	assert.NoError(t, err)
	defer os.RemoveAll(dirPath)

	filePath := filepath.Join(dirPath, "file")
	assert.NoError(t, os.WriteFile(filePath, defaultConfigObjectBytes(), 0666))

	fileConfig := NewFileConfig(audience, fullUri)
	assert.NoError(t, fileConfig.StartWatcher(ctx, filePath))
	verifyConfigObject(t, fileConfig, defaultConfigObject())

	newConfigObject := defaultConfigObject()
	newConfigObject.Identities = append(newConfigObject.Identities, Identity{
		Namespace:      "new-ns",
		ServiceAccount: "new-sa",
	})
	newConfigObjectBytes, err := json.Marshal(newConfigObject)
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(filePath, newConfigObjectBytes, 0666))
	verifyConfigObject(t, fileConfig, newConfigObject)
}

func TestFileConfig_WatcherNotStarted(t *testing.T) {
	fileConfig := NewFileConfig(audience, fullUri)
	patchConfig := fileConfig.Get("non-existent", "non-existent")
	assert.Nil(t, patchConfig)
}

func TestFileConfig_Load(t *testing.T) {
	testcases := []struct {
		name                 string
		input                []byte
		expectedConfigObject *IdentityConfigObject
		expectError          bool
	}{
		{
			name:                 "Nil byte slice",
			input:                nil,
			expectedConfigObject: nil,
		},
		{
			name:                 "Empty byte slice",
			input:                make([]byte, 0),
			expectedConfigObject: nil,
		},
		{
			name:                 "Basic Test",
			input:                defaultConfigObjectBytes(),
			expectedConfigObject: defaultConfigObject(),
		},
		{
			name:        "Malformed JSON bytes",
			input:       []byte("bad json"),
			expectError: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fileConfig := NewFileConfig(audience, fullUri)
			err := fileConfig.Load(tc.input)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				verifyConfigObject(t, fileConfig, tc.expectedConfigObject)
			}
		})
	}

}

func TestFileConfig_Get(t *testing.T) {
	fileConfig := NewFileConfig(audience, fullUri)
	err := fileConfig.Load(defaultConfigObjectBytes())
	assert.NoError(t, err)

	assert.NotNil(t, fileConfig.cache)
	assert.Len(t, fileConfig.cache, 2)

	patchConfig := fileConfig.Get(namespaceFoo, namespaceFooServiceAccount)
	assert.NotNil(t, patchConfig)
	assert.Equal(t, audience, patchConfig.Audience)
	assert.Equal(t, fullUri, patchConfig.FullUri)

	patchConfig = fileConfig.Get("non-existent", "non-existent")
	assert.Nil(t, patchConfig)
}

func defaultConfigObject() *IdentityConfigObject {
	return &IdentityConfigObject{
		Identities: []Identity{
			{
				Namespace:      namespaceFoo,
				ServiceAccount: namespaceFooServiceAccount,
			},
			{
				Namespace:      namespaceBar,
				ServiceAccount: namespaceBarServiceAccount,
			},
		},
	}
}

func defaultConfigObjectBytes() []byte {
	configObject := defaultConfigObject()
	jsonBytes, err := json.Marshal(configObject)
	if err != nil {
		panic(err)
	}
	return jsonBytes
}

func verifyConfigObject(t *testing.T, fileConfig *FileConfig, expected *IdentityConfigObject) {
	assert.Eventually(t, func() bool {
		return reflect.DeepEqual(fileConfig.identityConfigObject, expected)
	}, defaultTimeout, defaultPollInterval)
}
