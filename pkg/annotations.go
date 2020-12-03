/*
  Copyright 2010 Amazon.com, Inc. or its affiliates. All Rights Reserved.

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
	// The audience annotation
	AudienceAnnotation = "audience"
	// Role ARN annotation
	RoleARNAnnotation = "role-arn"
	// A true/false value to add AWS_STS_REGIONAL_ENDPOINTS. Overrides any setting on the webhook
	UseRegionalSTSAnnotation = "sts-regional-endpoints"
	// Expiration in seconds for serviceAccountToken annotation
	TokenExpirationAnnotation = "token-expiration"

	// A comma-separated list of container names to skip adding environment variables and volumes to. Applies to `initContainers` and `containers`
	SkipContainersAnnotation = "skip-containers"
)
