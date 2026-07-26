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

	request := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
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

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	newHandler().ServeHTTP(response, request)

	expectedHeaders := map[string]string{
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
		"Permissions-Policy":      "camera=(), geolocation=(), microphone=()",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
	}
	for header, expected := range expectedHeaders {
		if actual := response.Header().Get(header); actual != expected {
			t.Errorf("expected %s header %q, got %q", header, expected, actual)
		}
	}
}

func TestCORSPreflight(t *testing.T) {
	t.Parallel()

	handler := requestIDMiddleware(corsMiddleware(map[string]struct{}{
		"http://127.0.0.1:5173": {},
	}, http.NotFoundHandler()))
	request := httptest.NewRequest(http.MethodOptions, "/api/v1", nil)
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, response.Code)
	}
	if origin := response.Header().Get("Access-Control-Allow-Origin"); origin != "http://127.0.0.1:5173" {
		t.Fatalf("unexpected allowed origin: %q", origin)
	}
	if vary := response.Header().Get("Vary"); vary != "Origin" {
		t.Fatalf("expected Vary Origin, got %q", vary)
	}
}

func TestCORSRejectsUnknownPreflightOrigin(t *testing.T) {
	t.Parallel()

	handler := requestIDMiddleware(corsMiddleware(map[string]struct{}{
		"http://127.0.0.1:5173": {},
	}, http.NotFoundHandler()))
	request := httptest.NewRequest(http.MethodOptions, "/api/v1", nil)
	request.Header.Set("Origin", "https://untrusted.example")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, response.Code)
	}
	if allowedOrigin := response.Header().Get("Access-Control-Allow-Origin"); allowedOrigin != "" {
		t.Fatalf("expected no allowed origin, got %q", allowedOrigin)
	}

	var body errorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "origin_not_allowed" {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestProjectListDefaults(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	response := httptest.NewRecorder()

	newHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var body projectListResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 4 || body.Pagination.Total != 4 || body.Pagination.Page != 1 {
		t.Fatalf("unexpected response: %#v", body)
	}
	if body.RequestID == "" {
		t.Fatal("expected request ID")
	}
}

func TestProjectListFiltersAndPaginates(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects?q=Agent&category=RAG%20Agent&page_size=1", nil)
	response := httptest.NewRecorder()

	newHandler().ServeHTTP(response, request)

	var body projectListResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != "paperclip" {
		t.Fatalf("unexpected projects: %#v", body.Data)
	}
	if body.Pagination.Total != 1 || body.Pagination.TotalPages != 1 {
		t.Fatalf("unexpected pagination: %#v", body.Pagination)
	}
}

func TestProjectListSortsByStars(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects?sort=stars&page_size=2", nil)
	response := httptest.NewRecorder()

	newHandler().ServeHTTP(response, request)

	var body projectListResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 2 || body.Data[0].ID != "atlas" || body.Data[1].ID != "paperclip" {
		t.Fatalf("unexpected order: %#v", body.Data)
	}
}

func TestProjectListRejectsInvalidQuery(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/api/v1/projects?page=0",
		"/api/v1/projects?page_size=51",
		"/api/v1/projects?sort=unknown",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			newHandler().ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
			}
			var body errorResponse
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Error.Code != "invalid_query" {
				t.Fatalf("unexpected response: %#v", body)
			}
		})
	}
}

func TestProjectDetail(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/atlas-agent", nil)
	response := httptest.NewRecorder()

	newHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var body projectDetailResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Slug != "atlas-agent" || body.Data.CurrentVersion != "0.8.0" {
		t.Fatalf("unexpected project: %#v", body.Data)
	}
	if len(body.Data.Highlights) == 0 || len(body.Data.UseCases) == 0 || body.RequestID == "" {
		t.Fatalf("incomplete response: %#v", body)
	}
}

func TestProjectDetailNotFound(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/api/v1/projects/",
		"/api/v1/projects/not-found",
		"/api/v1/projects/atlas-agent/extra",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			newHandler().ServeHTTP(response, request)

			if response.Code != http.StatusNotFound {
				t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
			}
			var body errorResponse
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Error.Code != "project_not_found" {
				t.Fatalf("unexpected response: %#v", body)
			}
		})
	}
}

func TestProjectDetailMethodNotAllowed(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/atlas-agent", nil)
	response := httptest.NewRecorder()

	newHandler().ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, response.Code)
	}
	var body errorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "method_not_allowed" {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestDocumentList(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/atlas-agent/documents", nil)
	response := httptest.NewRecorder()

	newHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	var body documentListResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].Slug != "quick-start" || body.RequestID == "" {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestDocumentDetail(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/atlas-agent/documents/quick-start", nil)
	response := httptest.NewRecorder()

	newHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	var body documentDetailResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.ID != "doc-atlas-quick-start" || body.Data.Markdown == "" {
		t.Fatalf("unexpected document: %#v", body.Data)
	}
	if len(body.Data.Outline) != 4 || len(body.Data.Blocks) != 4 {
		t.Fatalf("expected outline and stable blocks, got %#v", body.Data)
	}
}

func TestDocumentErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		code string
	}{
		{path: "/api/v1/projects/not-found/documents", code: "project_not_found"},
		{path: "/api/v1/projects/atlas-agent/documents/not-found", code: "document_not_found"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			newHandler().ServeHTTP(response, request)

			if response.Code != http.StatusNotFound {
				t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
			}
			var body errorResponse
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Error.Code != test.code {
				t.Fatalf("expected error %q, got %#v", test.code, body)
			}
		})
	}
}
