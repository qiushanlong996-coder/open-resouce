package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpoints(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/healthz", "/readyz"} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()

			newHandler().ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
			}
			if requestID := response.Header().Get(requestIDHeader); requestID == "" {
				t.Fatal("expected response request ID")
			}

			var body healthResponse
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Service != "gateway" || body.Status != "ok" {
				t.Fatalf("unexpected response: %#v", body)
			}
		})
	}
}

func TestUnsupportedRoute(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	newHandler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}

	var body errorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "route_not_found" || body.RequestID == "" {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestListenAddress(t *testing.T) {
	t.Setenv("HOST", "")
	t.Setenv("PORT", "")
	if address := listenAddress(); address != "127.0.0.1:8080" {
		t.Fatalf("unexpected default address: %s", address)
	}

	t.Setenv("HOST", "0.0.0.0")
	t.Setenv("PORT", "18080")
	if address := listenAddress(); address != "0.0.0.0:18080" {
		t.Fatalf("unexpected configured address: %s", address)
	}
}

func TestRequestIDIsPropagated(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set(requestIDHeader, "client-request-id")
	response := httptest.NewRecorder()

	newHandler().ServeHTTP(response, request)

	if requestID := response.Header().Get(requestIDHeader); requestID != "client-request-id" {
		t.Fatalf("expected propagated request ID, got %q", requestID)
	}
}

func TestMethodNotAllowedUsesJSONError(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	response := httptest.NewRecorder()

	newHandler().ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, response.Code)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow header %q, got %q", http.MethodGet, allow)
	}

	var body errorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "method_not_allowed" {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestAccessLogContainsRequestMetadata(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	handler := requestIDMiddleware(accessLogMiddleware(logger, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusCreated)
	})))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", nil)
	request.Header.Set(requestIDHeader, "log-request-id")

	handler.ServeHTTP(httptest.NewRecorder(), request)

	logLine := output.String()
	for _, expected := range []string{
		`"request_id":"log-request-id"`,
		`"method":"POST"`,
		`"path":"/api/v1/projects"`,
		`"status":201`,
	} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("expected log to contain %q, got %s", expected, logLine)
		}
	}
}

func TestAPIVersionEntry(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	response := httptest.NewRecorder()

	newHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var body serviceInfoResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Service != "gateway" || body.APIVersion != "v1" || body.Status != "ok" {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestUnknownVersionedRoute(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	response := httptest.NewRecorder()

	newHandler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}

	var body errorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "route_not_found" {
		t.Fatalf("unexpected response: %#v", body)
	}
}
