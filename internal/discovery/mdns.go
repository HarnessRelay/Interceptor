package discovery

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/grandcat/zeroconf"
)

const (
	ServiceType = "_harnessrelay._tcp"
	ServiceDomain = "local"
)

type Config struct {
	DeviceID   string
	DeviceName string
	Port       int
	Version    string
	Fingerprint string // short 8-char hex fingerprint
}

type Advertiser struct {
	server *zeroconf.Server
	logger *slog.Logger
	cfg    Config
}

func NewAdvertiser(cfg Config, logger *slog.Logger) *Advertiser {
	return &Advertiser{
		cfg:    cfg,
		logger: logger,
	}
}

func (a *Advertiser) Start() error {
	instanceName := a.cfg.DeviceName
	if instanceName == "" {
		instanceName = "harnessrelay"
	}

	text := []string{
		fmt.Sprintf("id=%s", a.cfg.DeviceID),
		fmt.Sprintf("name=%s", a.cfg.DeviceName),
		fmt.Sprintf("version=%s", a.cfg.Version),
		fmt.Sprintf("fp=%s", a.cfg.Fingerprint),
	}

	server, err := zeroconf.Register(
		instanceName,
		ServiceType,
		ServiceDomain,
		a.cfg.Port,
		text,
		nil,
	)
	if err != nil {
		return fmt.Errorf("mDNS register: %w", err)
	}

	a.server = server
	a.logger.Info("mDNS service advertised",
		slog.String("instance", instanceName),
		slog.String("service_type", ServiceType),
		slog.Int("port", a.cfg.Port),
		slog.String("device_id", a.cfg.DeviceID),
	)
	return nil
}

func (a *Advertiser) Stop() {
	if a.server != nil {
		a.server.Shutdown()
		a.logger.Info("mDNS service stopped")
	}
}

// Browse searches for HarnessRelay daemons on the local network.
// It sends discovered services to the results channel and blocks until ctx is cancelled.
func Browse(ctx context.Context, results chan<- *zeroconf.ServiceEntry) error {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return fmt.Errorf("mDNS resolver: %w", err)
	}
	return resolver.Browse(ctx, ServiceType, ServiceDomain, results)
}
