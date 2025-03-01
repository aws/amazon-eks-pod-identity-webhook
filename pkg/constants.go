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
	// 24hrs as that is max for EKS
	MaxTokenExpiration = int64(86400)
	// Default token expiration in seconds if none is defined, 22hrs
	DefaultTokenExpiration = int64(86400)
	// Used for the minimum jitter value when using the default token expiration
	DefaultMinTokenExpiration = int64(79200)
	// 10mins is min for kube-apiserver
	MinTokenExpiration = int64(600)

	// AWS SDK defined environment variables.
	AwsEnvVarContainerCredentialsFullUri     = "AWS_CONTAINER_CREDENTIALS_FULL_URI"
	AwsEnvVarContainerAuthorizationTokenFile = "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE"
)
