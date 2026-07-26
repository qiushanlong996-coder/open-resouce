package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/healthz" {
			t.Fatalf("path = %q, want /healthz", request.URL.Path)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := checkHealth(context.Background(), server.URL+"/healthz"); err != nil {
		t.Fatalf("checkHealth() error = %v", err)
	}
}

func TestCheckHealthRejectsUnhealthyStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if err := checkHealth(context.Background(), server.URL); err == nil {
		t.Fatal("checkHealth() error = nil, want unhealthy status error")
	}
}

func TestHealthcheckURLUsesConfiguredPort(t *testing.T) {
	t.Setenv("PORT", "19090")
	if got := healthcheckURL(); got != "http://127.0.0.1:19090/healthz" {
		t.Fatalf("healthcheckURL() = %q", got)
	}
}
