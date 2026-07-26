package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const defaultPort = "8080"
const defaultHost = "127.0.0.1"

type healthResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

type serviceInfoResponse struct {
	Service    string `json:"service"`
	APIVersion string `json:"api_version"`
	Status     string `json:"status"`
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	health := requireMethod(http.MethodGet, func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, healthResponse{
			Service: "gateway",
			Status:  "ok",
		})
	})

	mux.HandleFunc("/healthz", health)
	mux.HandleFunc("/readyz", health)
	mux.HandleFunc("/api/v1", requireMethod(http.MethodGet, func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, serviceInfoResponse{
			Service:    "gateway",
			APIVersion: "v1",
			Status:     "ok",
		})
	}))
	mux.HandleFunc("/api/v1/projects", requireMethod(http.MethodGet, projectListHandler))
	mux.HandleFunc("/api/v1/projects/{slug}/documents/{documentSlug}/comments/{commentID}", documentCommentHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/documents/{documentSlug}/comments", documentCommentsHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/documents", requireMethod(http.MethodGet, documentListHandler))
	mux.HandleFunc("/api/v1/projects/{slug}/documents/{documentSlug}", requireMethod(http.MethodGet, documentDetailHandler))
	mux.HandleFunc("/api/v1/projects/", requireMethod(http.MethodGet, projectDetailHandler))
	mux.HandleFunc("/api/v1/", requireMethod(http.MethodGet, func(writer http.ResponseWriter, request *http.Request) {
		writeAPIError(writer, request, http.StatusNotFound, "route_not_found", "请求的接口不存在")
	}))
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writeAPIError(writer, request, http.StatusNotFound, "route_not_found", "请求的接口不存在")
	})

	logger := slog.Default()
	handler := corsMiddleware(allowedOriginsFromEnvironment(), mux)
	handler = accessLogMiddleware(logger, handler)
	handler = securityHeadersMiddleware(handler)
	return requestIDMiddleware(handler)
}

func listenAddress() string {
	host := os.Getenv("HOST")
	if host == "" {
		host = defaultHost
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	return net.JoinHostPort(host, port)
}

func healthcheckURL() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}
	return "http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz"
}

func checkHealth(ctx context.Context, url string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("health endpoint returned " + response.Status)
	}
	return nil
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := checkHealth(ctx, healthcheckURL()); err != nil {
			slog.Error("gateway healthcheck failed", "error", err)
			os.Exit(1)
		}
		return
	}

	server := &http.Server{
		Addr:              listenAddress(),
		Handler:           newHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-shutdownSignal.Done()

		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownContext); err != nil {
			slog.Error("gateway shutdown failed", "error", err)
		}
	}()

	slog.Info("gateway listening", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("gateway stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}
