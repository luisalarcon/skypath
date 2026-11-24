// Command web runs the SkyPath web frontend: a server-rendered search page
// that talks to the flight search API on the user's behalf.
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

	"github.com/luisalarcon/skypath/frontend/internal/client"
	"github.com/luisalarcon/skypath/frontend/internal/config"
	"github.com/luisalarcon/skypath/frontend/internal/web"
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

	logger := cfg.Logger(os.Stdout)
	if err := run(logger, cfg); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// run serves until a stop signal arrives, then shuts down gracefully. The
// frontend holds no data of its own, so it can start before the API is
// reachable; until then searches show the "temporarily unavailable" notice.
func run(logger *slog.Logger, cfg config.Config) error {
	api := client.New(cfg.BackendURL, &http.Client{Timeout: cfg.BackendTimeout})

	srv := &http.Server{
		Addr: cfg.Addr(),
		Handler: web.NewHandler(web.Dependencies{
			API:            api,
			Logger:         logger,
			RequestTimeout: cfg.RequestTimeout,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// The write timeout must outlast the request deadline, or a slow
		// request would be cut off before it could render its own error page.
		WriteTimeout: cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", srv.Addr, "backend", cfg.BackendURL)
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
