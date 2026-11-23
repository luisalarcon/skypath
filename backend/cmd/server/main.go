// Command server runs the SkyPath flight search API.
package main

import (
	"context"
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

	"github.com/luisalarcon/skypath/backend/internal/config"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe the running server's health endpoint and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration:\n%v\n", err)
		os.Exit(2)
	}

	if *healthcheck {
		os.Exit(probe(cfg.Port))
	}

	logger := cfg.NewLogger(os.Stdout)
	if err := run(logger, cfg); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// run loads the schedule, serves until a stop signal arrives, then shuts down
// gracefully. The schedule is loaded before the listener opens, so the
// service never answers a search from a partial data set.
func run(logger *slog.Logger, cfg config.Config) error {
	handler, err := newHandler(cfg, logger)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// The write timeout must outlast the request deadline, or a slow
		// request would be cut off before it could report its own timeout.
		WriteTimeout: cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:  60 * time.Second,
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// probe calls the local health endpoint and returns a process exit code. It
// lets the container health check run without any extra tools in the image.
func probe(port int) int {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
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
