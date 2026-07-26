package main

import (
	"context"
	"encoding/json"
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

func newHandler() http.Handler {
	mux := http.NewServeMux()
	health := func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(writer).Encode(healthResponse{
			Service: "gateway",
			Status:  "ok",
		})
	}

	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /readyz", health)

	return mux
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

func main() {
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
