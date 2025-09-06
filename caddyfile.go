package local_dns

import (
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	// Register global app
	httpcaddyfile.RegisterGlobalOption("local_dns", parseApp)
	// Register site directive - specify order
	httpcaddyfile.RegisterHandlerDirective("local_dns", parseHandler)
	httpcaddyfile.RegisterDirectiveOrder("local_dns", httpcaddyfile.Before, "reverse_proxy")
}

// parseApp configures the "local_dns" global option from Caddyfile.
func parseApp(d *caddyfile.Dispenser, _ any) (any, error) {
	app := new(App)

	// consume the option name
	if !d.Next() {
		return nil, d.ArgErr()
	}

	// let your existing UnmarshalCaddyfile do the heavy lifting
	if err := app.UnmarshalCaddyfile(d); err != nil {
		return nil, err
	}

	// wrap into an httpcaddyfile.App
	return httpcaddyfile.App{
		Name:  "local_dns",
		Value: caddyconfig.JSON(app, nil),
	}, nil
}

// parseHandler configures the "local_dns" directive inside site blocks.
func parseHandler(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m Handler
	if err := m.UnmarshalCaddyfile(h.Dispenser); err != nil {
		return nil, err
	}
	return &m, nil
}
