package local_dns

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/mietzen/caddy-local-dns/provider"
	"go.uber.org/zap"
)

func init() {
	caddy.RegisterModule(App{})
	caddy.RegisterModule(Handler{})
}

// App is the global app that manages DNS providers
type App struct {
	Providers map[string]*ProviderConfig `json:"providers,omitempty"`
	CaddyIP   string                     `json:"caddy_ip,omitempty"`
	Debug     bool                       `json:"debug,omitempty"`

	logger  *zap.Logger
	clients map[string]provider.DNSService
}

// ProviderConfig holds the configuration for a DNS provider
type ProviderConfig struct {
	Type       string `json:"type"` // "opnsense", "pihole", etc.
	Hostname   string `json:"hostname,omitempty"`
	APIKey     string `json:"api_key,omitempty"`
	APISecret  string `json:"api_secret,omitempty"`
	DNSService string `json:"dns_service,omitempty"` // "unbound", "dnsmasq", etc.
	Insecure   bool   `json:"insecure,omitempty"`
}

// Handler is the HTTP handler that processes individual site configurations
type Handler struct {
	Provider   string `json:"provider,omitempty"`
	IPOverride string `json:"ip_override,omitempty"`

	logger *zap.Logger
	app    *App
}

// App methods
func (App) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "local_dns",
		New: func() caddy.Module { return new(App) },
	}
}

func (a *App) Provision(ctx caddy.Context) error {
	a.logger = ctx.Logger(a)
	a.clients = make(map[string]provider.DNSService)

	// Validate global caddy_ip
	if a.CaddyIP != "" {
		if net.ParseIP(a.CaddyIP) == nil {
			return fmt.Errorf("invalid caddy_ip address: %s", a.CaddyIP)
		}
	}

	// Initialize providers
	for name, config := range a.Providers {
		client, err := a.createProvider(config)
		if err != nil {
			return fmt.Errorf("failed to create provider %s: %w", name, err)
		}
		a.clients[name] = client

		logMsg := "initialized DNS provider"
		fields := []zap.Field{
			zap.String("name", name),
			zap.String("type", config.Type),
		}
		if a.Debug {
			fields = append(fields,
				zap.String("hostname", config.Hostname),
				zap.String("dns_service", config.DNSService),
				zap.Bool("insecure", config.Insecure),
			)
		}
		a.logger.Info(logMsg, fields...)
	}

	return nil
}

func (a *App) Start() error {
	return nil
}

func (a *App) Stop() error {
	return nil
}

func (a *App) createProvider(config *ProviderConfig) (provider.DNSService, error) {
	switch config.Type {
	case "opnsense":
		return provider.NewOPNsenseProvider(config.Hostname, config.APIKey, config.APISecret, config.DNSService, config.Insecure, a.logger, a.Debug)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", config.Type)
	}
}

// Handler methods
func (Handler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.local_dns",
		New: func() caddy.Module { return new(Handler) },
	}
}

func (h *Handler) Provision(ctx caddy.Context) error {
	h.logger = ctx.Logger(h)

	// Get the app instance
	appIface, err := ctx.App("local_dns")
	if err != nil {
		return fmt.Errorf("local_dns app not configured")
	}
	h.app = appIface.(*App)

	if h.Provider == "" {
		return errors.New("provider name is required")
	}

	if _, exists := h.app.clients[h.Provider]; !exists {
		return fmt.Errorf("provider %s not found in global configuration", h.Provider)
	}

	return nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	// Get the domain from the request
	domain := r.Host

	// Remove port if present
	if colonIndex := strings.LastIndex(domain, ":"); colonIndex != -1 {
		domain = domain[:colonIndex]
	}

	// Handle the DNS record
	if err := h.handleDomain(domain); err != nil {
		h.logger.Error("failed to handle domain", zap.String("domain", domain), zap.Error(err))
		// Don't fail the request, just log the error
	}

	return next.ServeHTTP(w, r)
}

func (h *Handler) handleDomain(domain string) error {
	provider, exists := h.app.clients[h.Provider]
	if !exists {
		return fmt.Errorf("provider %s not found", h.Provider)
	}

	// Determine IP to use: ip_override takes precedence, then fall back to global caddy_ip
	ip := h.IPOverride
	if ip == "" {
		ip = h.app.CaddyIP
	}

	if ip == "" {
		return errors.New("no IP address configured: set either ip_override in handler or caddy_ip in global config")
	}

	// Validate IP
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	h.logger.Info("handling domain",
		zap.String("domain", domain),
		zap.String("ip", ip),
		zap.String("provider", h.Provider))

	// Check if record exists
	existing, err := provider.FindRecord(domain)
	if err != nil {
		return fmt.Errorf("failed to find existing record: %w", err)
	}

	if existing != nil {
		// Check if update is needed
		if existing.IP == ip && existing.Enabled {
			h.logger.Info("DNS record already exists and is correct", zap.String("domain", domain))
			return nil
		}

		// Update existing record
		h.logger.Info("updating existing DNS record", zap.String("domain", domain))
		return provider.UpdateRecord(domain, ip)
	}

	// Create new record
	h.logger.Info("creating new DNS record", zap.String("domain", domain))
	return provider.CreateRecord(domain, ip)
}

// Caddyfile unmarshaling for App (global config)
func (a *App) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	a.Providers = make(map[string]*ProviderConfig)

	for d.Next() {
		for nesting := d.Nesting(); d.NextBlock(nesting); {
			switch d.Val() {
			case "provider":
				if !d.NextArg() {
					return d.ArgErr()
				}
				providerName := d.Val()

				config := &ProviderConfig{}
				if !d.NextArg() {
					return d.ArgErr()
				}
				config.Type = d.Val()

				// Parse provider block
				for nesting := d.Nesting(); d.NextBlock(nesting); {
					switch d.Val() {
					case "hostname":
						if !d.AllArgs(&config.Hostname) {
							return d.ArgErr()
						}
					case "api_key":
						if !d.AllArgs(&config.APIKey) {
							return d.ArgErr()
						}
					case "api_secret":
						if !d.AllArgs(&config.APISecret) {
							return d.ArgErr()
						}
					case "dns_service":
						if !d.AllArgs(&config.DNSService) {
							return d.ArgErr()
						}
					case "insecure":
						config.Insecure = true
					}
				}

				a.Providers[providerName] = config
			case "caddy_ip":
				if !d.AllArgs(&a.CaddyIP) {
					return d.ArgErr()
				}
			case "debug":
				a.Debug = true
			}
		}
	}
	return nil
}

// Caddyfile unmarshaling for Handler (site-specific config)
func (h *Handler) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		if d.NextArg() {
			h.Provider = d.Val()
		}
		// No block parsing needed since we removed caddy_ip from handler
	}
	return nil
}

// Interface compliance
var (
	_ caddy.App                   = (*App)(nil)
	_ caddy.Provisioner           = (*App)(nil)
	_ caddyfile.Unmarshaler       = (*App)(nil)
	_ caddy.Module                = (*Handler)(nil)
	_ caddy.Provisioner           = (*Handler)(nil)
	_ caddyfile.Unmarshaler       = (*Handler)(nil)
	_ caddyhttp.MiddlewareHandler = (*Handler)(nil)
)
