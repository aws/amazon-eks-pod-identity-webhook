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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShutdownFromContext_SuccessfulGracefulShutdown(t *testing.T) {
	// Create a test server
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Call ShutdownFromContext
	doneCh, errCh := ShutdownFromContext(ctx, server.Config, 2*time.Second)

	// Start the server
	server.Start()
	defer server.Close()

	// Cancel the context to trigger shutdown
	cancel()

	// Wait for shutdown to complete
	select {
	case <-doneCh:
		// Shutdown completed
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not complete within expected time")
	}

	// Check that no error occurred (graceful shutdown succeeded).
	if err := <-errCh; err != nil {
		t.Errorf("Expected no error for graceful shutdown, got: %v", err)
	}
}

func TestShutdownFromContext_GracefulShutdownTimeout(t *testing.T) {
	// Create a test server with a slow handler that will delay shutdown
	testServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow handler that takes time to complete
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	// Start the server
	testServer.Start()
	defer testServer.Close()

	// Start a request to the slow handler to create an active connection during shutdown
	go func() {
		client := &http.Client{}
		client.Get(testServer.URL)
	}()

	// Give the request time to start
	time.Sleep(10 * time.Millisecond)

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Call ShutdownFromContext with a very short timeout to force timeout
	doneCh, errCh := ShutdownFromContext(ctx, testServer.Config, 1*time.Nanosecond)

	// Cancel the context to trigger shutdown
	cancel()

	// Wait for shutdown to complete
	select {
	case <-doneCh:
		// Shutdown completed
	case <-time.After(1 * time.Second):
		t.Fatal("Shutdown did not complete within expected time")
	}

	// Check that we got an error (timeout should always cause an error)
	if err := <-errCh; err == nil {
		t.Error("Expected error for graceful shutdown timeout, but got nil")
	} else {
		t.Logf("Received expected error with very short timeout: %v", err)
	}
}
