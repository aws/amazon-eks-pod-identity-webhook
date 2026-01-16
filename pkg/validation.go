/*
  Copyright 2020 Amazon.com, Inc. or its affiliates. All Rights Reserved.

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
	"fmt"
	"strings"
)

func ValidateTLSCipherSuites(cipherNames []string) ([]uint16, error) {
	if len(cipherNames) == 0 {
		return nil, nil
	}

	// Create a map of all available cipher suites
	availableSuites := make(map[string]uint16)
	for _, suite := range tls.CipherSuites() {
		availableSuites[suite.Name] = suite.ID
	}
	// Also include insecure suites just in case, though discouraged
	for _, suite := range tls.InsecureCipherSuites() {
		availableSuites[suite.Name] = suite.ID
	}

	var ids []uint16
	for _, name := range cipherNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		id, ok := availableSuites[name]
		if !ok {
			return nil, fmt.Errorf("unsupported cipher suite: %s", name)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func ValidateTLSMinVersion(version string) (uint16, error) {
	switch version {
	case "1.0":
		return tls.VersionTLS10, nil
	case "1.1":
		return tls.VersionTLS11, nil
	case "1.2":
		return tls.VersionTLS12, nil
	case "1.3":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("unsupported version %s", version)
	}
}

func ValidateMinTokenExpiration(expiration int64) (int64) {
	if expiration < MinTokenExpiration {
		return MinTokenExpiration
	}
	return expiration
}
