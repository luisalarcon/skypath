// Command server runs the SkyPath flight search API.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Embed the IANA time zone database so local flight times resolve the same
	// way on every host, including minimal container images that ship none.
	_ "time/tzdata"
)

const (
	defaultPort     = "8080"
	shutdownTimeout = 10 * time.Second
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe the running server's health endpoint and exit")
	flag.Parse()

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	if *healthcheck {
		os.Exit(probe(port))
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger, port); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, port string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", srv.Addr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// probe calls the local health endpoint and returns a process exit code. It
// lets the container health check run without any extra tools in the image.
func probe(port string) int {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/healthz", port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: unexpected status", resp.Status)
		return 1
	}
	return 0
}
