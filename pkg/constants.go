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

const (
	// Default token expiration in seconds if none is defined,
	// which is 24hrs as that is max for EKS
	DefaultTokenExpiration = int64(86400)
)

var (
	// Minimum token expiration in seconds.
	// The value is used if user configured the shorter duration than this value
	// This value must be at least 600(=10min) due to kubernetes spec (ref ServiceAccountTokenProjection.ExpirationSeconds).
	// Note: this can be configured by cli flags
	MinTokenExpiration = int64(3600)
)
