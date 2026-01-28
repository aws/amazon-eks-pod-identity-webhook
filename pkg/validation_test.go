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

func TestValidateTLSCipherSuites(t *testing.T) {
	// Pick a couple of standard cipher suites that should be available
	// Note: We need to ensure we pick ones that are in tls.CipherSuites() or tls.InsecureCipherSuites()
	// TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 is standard in Go
	validCipherName := "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
	validCipherID := uint16(tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256)

	tests := []struct {
		name        string
		input       []string
		expectedIDs []uint16
		expectError bool
	}{
		{
			name:        "empty input",
			input:       []string{},
			expectedIDs: nil,
			expectError: false,
		},
		{
			name:        "nil input",
			input:       nil,
			expectedIDs: nil,
			expectError: false,
		},
		{
			name:        "valid cipher",
			input:       []string{validCipherName},
			expectedIDs: []uint16{validCipherID},
			expectError: false,
		},
		{
			name:        "valid cipher with whitespace",
			input:       []string{" " + validCipherName + " "},
			expectedIDs: []uint16{validCipherID},
			expectError: false,
		},
		{
			name:        "invalid cipher",
			input:       []string{"INVALID_CIPHER_SUITE"},
			expectedIDs: nil,
			expectError: true,
		},
		{
			name:        "mixed valid and invalid",
			input:       []string{validCipherName, "INVALID_CIPHER_SUITE"},
			expectedIDs: nil,
			expectError: true,
		},
		{
			name:        "empty string in slice",
			input:       []string{validCipherName, ""},
			expectedIDs: []uint16{validCipherID},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateTLSCipherSuites(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedIDs, got)
			}
		})
	}
}

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
