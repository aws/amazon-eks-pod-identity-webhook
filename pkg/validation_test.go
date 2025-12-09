/*
  Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.

  Licensed under the Apache License, Version 2.0 (the "License").
  You may not use this file except in compliance with the License.
  A copy of the License is located at

      http://www.apache.org/licenses/LICENSE-2.0

  or in the "license" file accompanying this file. This file is distributed
  on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
  express or implied. See the License for the specific language governing
  permissions and limitations under the License.
*/
package pkg

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateTLSMinVersion(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedVer uint16
		expectError bool
	}{
		{
			name:        "valid 1.0",
			input:       "1.0",
			expectedVer: tls.VersionTLS10,
			expectError: false,
		},
		{
			name:        "valid 1.1",
			input:       "1.1",
			expectedVer: tls.VersionTLS11,
			expectError: false,
		},
		{
			name:        "valid 1.2",
			input:       "1.2",
			expectedVer: tls.VersionTLS12,
			expectError: false,
		},
		{
			name:        "valid 1.3",
			input:       "1.3",
			expectedVer: tls.VersionTLS13,
			expectError: false,
		},
		{
			name:        "invalid version",
			input:       "1.4",
			expectedVer: 0,
			expectError: true,
		},
		{
			name:        "invalid format",
			input:       "TLS1.2",
			expectedVer: 0,
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			expectedVer: 0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateTLSMinVersion(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedVer, got)
			}
		})
	}
}
