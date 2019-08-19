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
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog"
)

var (
	requestCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_count",
			Help: "Counter of requests broken out for each verb, path, and response code.",
		},
		[]string{"verb", "path", "code"},
	)
	requestLatencies = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_latencies",
			Help: "Response latency distribution in microseconds for each verb and path",
			// Use buckets ranging from 125 ms to 8 seconds.
			Buckets: prometheus.ExponentialBuckets(125000, 2.0, 7),
		},
		[]string{"verb", "path"},
	)
	requestLatenciesSummary = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "http_request_duration_microseconds",
			Help: "Response latency summary in microseconds for each verb and path.",
			// Make the sliding window of 1h.
			MaxAge: time.Hour,
		},
		[]string{"verb", "path"},
	)
)

func register() {
	prometheus.MustRegister(requestCounter)
	prometheus.MustRegister(requestLatencies)
	prometheus.MustRegister(requestLatenciesSummary)
}

func monitor(verb, path string, httpCode int, reqStart time.Time) {
	elapsed := float64((time.Since(reqStart)) / time.Microsecond)

	requestCounter.WithLabelValues(verb, path, strconv.Itoa(httpCode)).Inc()
	requestLatencies.WithLabelValues(verb, path).Observe(elapsed)
	requestLatenciesSummary.WithLabelValues(verb, path).Observe(elapsed)
}

func init() {
	register()
}

// Middleware is a type for decorating requests.
type Middleware func(http.Handler) http.Handler

// Apply wraps a list of middlewares around a handler and returns it
func Apply(h http.Handler, middlewares ...Middleware) http.Handler {
	for _, adapter := range middlewares {
		h = adapter(h)
	}
	return h
}

type statusLoggingResponseWriter struct {
	http.ResponseWriter
	status    int
	bodyBytes int
}

func (w *statusLoggingResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusLoggingResponseWriter) Write(data []byte) (int, error) {
	length, err := w.ResponseWriter.Write(data)
	w.bodyBytes += length
	return length, err
}

// InstrumentRoute is a middleware for adding the following metrics for each
// route:
//
//     # Counter
//     http_request_count{"verb", "path", "code}
//     # Histogram
//     http_request_latencies{"verb", "path"}
//     # Summary
//     http_request_duration_microseconds{"verb", "path", "code}
//
func InstrumentRoute() Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := time.Now()

			wrappedWriter := &statusLoggingResponseWriter{w, http.StatusOK, 0}

			defer func() {
				monitor(r.Method, r.URL.Path, wrappedWriter.status, now)
			}()
			h.ServeHTTP(wrappedWriter, r)

		})
	}
}

func Logging() Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wrappedWriter := &statusLoggingResponseWriter{w, http.StatusOK, 0}

			defer func() {
				klog.V(4).Infof("path=%s method=%s status=%d user_agent=%s body_bytes=%d",
					r.URL.Path,
					r.Method,
					wrappedWriter.status,
					r.Header.Get("User-Agent"),
					wrappedWriter.bodyBytes,
				)
			}()

			err := r.ParseForm()
			if err != nil {
				klog.Errorf("Error parsing form: %v", err)
				http.Error(w, `{"error": "error parsing form"}`, http.StatusBadRequest)
				return
			}

			h.ServeHTTP(wrappedWriter, r)
		})
	}
}
