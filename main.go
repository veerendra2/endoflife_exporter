package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/veerendra2/endoflife_exporter/internal/collector"
	"github.com/veerendra2/endoflife_exporter/internal/config"
	"github.com/veerendra2/gopackages/slogsetup"
)

const AppName = "endoflife_exporter"

var cli struct {
	Address string           `env:"ADDRESS" default:":8080" help:"The address where the server should listen on."`
	Config  string           `env:"CONFIG_FILE" default:"config.yml" help:"Configuration file path"`
	Log     slogsetup.Config `embed:"" prefix:"log." envprefix:"LOG_"`
}

func main() {
	kongCtx := kong.Parse(&cli, kong.Name(AppName))
	kongCtx.FatalIfErrorf(kongCtx.Error)

	slog.SetDefault(slogsetup.New(cli.Log))

	slog.Info("Loading configuration", "file", cli.Config)
	cfg, err := config.LoadConfig(cli.Config)
	if err != nil {
		slog.Error("Failed to load configuration", slog.Any("err", err))
		os.Exit(1)
	}

	exporter, err := collector.NewExporter(*cfg)
	if err != nil {
		slog.Error("Failed to create exporter", slog.Any("err", err))
		os.Exit(1)
	}
	prometheus.MustRegister(exporter)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err = w.Write([]byte("<body>Metrics are available at <a href=\"/metrics\">/metrics</a></body>")); err != nil {
			slog.Warn("Failed to write", slog.Any("err", err))
		}
	})
	http.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:              cli.Address,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server died unexpected.", slog.Any("error", err))
		}
		slog.Error("Server stopped.")
	}()

	// All components should be terminated gracefully. For that we are listen
	// for the SIGINT and SIGTERM signals and try to gracefully shutdown the
	// started components. This ensures that established connections or tasks
	// are not interrupted.
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	slog.Debug("Start listining for SIGINT and SIGTERM signal.")
	<-done
	slog.Info("Shutdown started.")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("HTTP shutdown error: %v", err)
	}

	slog.Info("Shutdown done.")
}
