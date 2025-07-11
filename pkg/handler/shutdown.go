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

package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"k8s.io/klog/v2"
)

// ShutdownFromContext gracefully shuts down an HTTP server when the provided context is cancelled.
// It waits for ctx to be cancelled, then attempts a graceful shutdown with the given timeout.
// If graceful shutdown fails, it will attempt to force close the server.
//
// Parameters:
//   - ctx: Context to watch for cancellation signal
//   - server: HTTP server to shutdown
//   - timeout: Maximum time to wait for graceful shutdown
//
// Returns:
//   - <-chan struct{}: Closed when shutdown process completes
//   - <-chan error: Receives shutdown error if shutdown wasn't graceful (nil on success)
func ShutdownFromContext(ctx context.Context, server *http.Server, timeout time.Duration) (<-chan struct{}, <-chan error) {
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		defer close(errCh)

		<-ctx.Done()
		klog.Info("Context cancelled, beginning graceful shutdown")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			// Try to force close the server because the server didn't shutdown gracefully within the timeout
			if closeErr := server.Close(); closeErr != nil {
				errCh <- errors.Join(
					fmt.Errorf("graceful server shutdown failed: %w", err),
					fmt.Errorf("server force close failed: %w", closeErr),
				)
			} else {
				errCh <- fmt.Errorf("graceful server shutdown failed, server has been force closed: %w", err)
			}
		} else {
			// Graceful shutdown succeeded
			errCh <- nil
		}
	}()

	return doneCh, errCh
}
