package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/harnessrelay/interceptor/internal/api"
	"github.com/harnessrelay/interceptor/internal/config"
	"github.com/harnessrelay/interceptor/internal/discovery"
	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/harness"
	"github.com/harnessrelay/interceptor/internal/logging"
	"github.com/harnessrelay/interceptor/internal/security"
	"github.com/harnessrelay/interceptor/internal/session"
	dashboard "github.com/harnessrelay/interceptor/web"
)

const version = "dev"

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printUsage()
		return
	}

	switch args[0] {
	case "serve":
		if err := serve(); err != nil {
			fmt.Fprintf(os.Stderr, "harnessd: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Print(`harnessd runs the HarnessRelay Interceptor daemon.

Usage:
  harnessd --help
  harnessd version
  harnessd serve
`)
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if os.Geteuid() == 0 && !cfg.Security.AllowRootForTesting {
		return errors.New("refusing to run as root; set HARNESSRELAY_ALLOW_ROOT_FOR_TESTING=1 only for tests")
	}
	logger := logging.New(os.Stdout, slog.LevelInfo)
	bus := events.NewBus()
	sessions := session.NewManagerWithBus(bus)
	harnesses := harness.DiscoverInstalled(context.Background())
	authToken := cfg.Security.AuthToken
	if authToken == "" {
		authToken = security.GenerateToken()
		logger.Warn("generated local dashboard token for this process; run make install or set HARNESSRELAY_TOKEN for a stable token",
			slog.String("local_auth_token", authToken),
		)
	}
	auth := security.NewAuthenticator(authToken)

	// Pairing infrastructure
	configDir, _ := config.ConfigDir()
	var deviceIdentity *security.DeviceIdentity
	var deviceStore *security.PairedDeviceStore
	var pairingMgr *security.PairingManager
	var mdnsAdvertiser *discovery.Advertiser

	if cfg.Security.PairingEnabled {
		hostname, _ := os.Hostname()
		var err error
		deviceIdentity, err = security.LoadOrCreateDeviceIdentity(configDir, hostname)
		if err != nil {
			logger.Warn("failed to load device identity, pairing disabled", slog.String("error", err.Error()))
		} else {
			logger.Info("device identity loaded",
				slog.String("device_id", deviceIdentity.ID),
				slog.String("device_name", deviceIdentity.Name),
			)

			deviceStore, err = security.NewPairedDeviceStore(configDir)
			if err != nil {
				logger.Warn("failed to load paired devices", slog.String("error", err.Error()))
			} else {
				auth.SetPairedDeviceStore(deviceStore)
				pairingMgr = security.NewPairingManager(deviceStore)
				logger.Info("pairing manager initialized",
					slog.Int("paired_devices", len(deviceStore.List())),
				)
			}

			// Start mDNS advertisement
			mdnsAdvertiser = discovery.NewAdvertiser(discovery.Config{
				DeviceID:    deviceIdentity.ID,
				DeviceName:  deviceIdentity.Name,
				Port:        cfg.Port,
				Version:     version,
				Fingerprint: security.ShortFingerprint(deviceIdentity.PublicKey),
			}, logger)
			if err := mdnsAdvertiser.Start(); err != nil {
				logger.Warn("mDNS advertisement failed", slog.String("error", err.Error()))
			}
		}
	}

	if !security.IsLocalBind(cfg.Address()) {
		if len(cfg.AllowedIPs) > 0 {
			logger.Info("harnessd binding outside localhost with IP allowlist enforced",
				slog.String("bind_address", cfg.BindAddress),
				slog.Int("allowed_entries", len(cfg.AllowedIPs)),
			)
		} else {
			logger.Warn("harnessd binding outside localhost; remote access can control terminal sessions",
				slog.String("bind_address", cfg.BindAddress),
				slog.Int("port", cfg.Port),
			)
		}
	}

	router := api.NewRouter(api.Options{
		Logger:     logger,
		Version:    version,
		StaticFS:   dashboardFS(),
		Sessions:   sessions,
		Events:     bus,
		Auth:       auth,
		Harnesses:  harnesses,
		AllowedIPs: cfg.AllowedIPs,
		Identity:   deviceIdentity,
		Pairing:    pairingMgr,
		Devices:    deviceStore,
	})

	server := &http.Server{
		Addr:              cfg.Address(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("harnessd starting",
			slog.String("bind_address", cfg.BindAddress),
			slog.Int("port", cfg.Port),
			slog.String("version", version),
			slog.String("config_format", config.Format),
			slog.Int64("terminal_history_limit_bytes", cfg.Terminal.HistoryLimitBytes),
			slog.Int("detected_harnesses", len(harnesses)),
			slog.Int("allowed_ips", len(cfg.AllowedIPs)),
		)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("harnessd shutdown requested")
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}

	if mdnsAdvertiser != nil {
		mdnsAdvertiser.Stop()
	}
	if pairingMgr != nil {
		pairingMgr.Stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}

	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	logger.Info("harnessd stopped")
	return nil
}

func dashboardFS() fs.FS {
	return dashboard.FS()
}
