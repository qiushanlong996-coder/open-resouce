package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
