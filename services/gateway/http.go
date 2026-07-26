package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const requestIDHeader = "X-Request-ID"

type contextKey string

const requestIDContextKey contextKey = "request-id"

type errorResponse struct {
	Error     apiError `json:"error"`
	RequestID string   `json:"request_id"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func requestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey).(string)
	return requestID
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(value[:])
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := strings.TrimSpace(request.Header.Get(requestIDHeader))
		if requestID == "" || len(requestID) > 128 {
			requestID = newRequestID()
		}

		writer.Header().Set(requestIDHeader, requestID)
		ctx := context.WithValue(request.Context(), requestIDContextKey, requestID)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func accessLogMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}

		next.ServeHTTP(recorder, request)

		logger.Info("request completed",
			"request_id", requestIDFromContext(request.Context()),
			"method", request.Method,
			"path", request.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	})
}

func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}

func writeAPIError(writer http.ResponseWriter, request *http.Request, status int, code, message string) {
	writeJSON(writer, status, errorResponse{
		Error: apiError{
			Code:    code,
			Message: message,
		},
		RequestID: requestIDFromContext(request.Context()),
	})
}

func requireMethod(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != method {
			writer.Header().Set("Allow", method)
			writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
			return
		}
		next(writer, request)
	}
}
