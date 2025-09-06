# Caddy Local DNS Module

A Caddy module that automatically creates DNS records in your local DNS server when configuring reverse proxies or other services.

## Supported Providers

- **OPNsense** (Unbound DNS or Dnsmasq)

## Installation

Build Caddy with this module:

```bash
xcaddy build --with github.com/mietzen/caddy-local-dns
```

## Configuration

### Global Configuration

Configure DNS providers in the global block:

```caddyfile
{
    local_dns {
        provider opnsense {
            hostname opnsense.local
            api_key your_api_key_here
            api_secret your_api_secret_here
            dns_service unbound # or dnsmasq
            insecure  # optional, for self-signed certs
        }
        caddy_ip 192.168.1.50 # IP of the Host running Caddy
        debug  # optional, enable debug logging
    }
}
```

### Site Configuration

Use the provider in your site blocks:

```caddyfile
service.example.com {
    reverse_proxy localhost:8080
    local_dns opnsense
}

api.example.com {
    reverse_proxy localhost:3000
    local_dns opnsense
}
```

## How It Works

1. When Caddy processes a request, the module extracts the domain name
2. It checks if a DNS record exists for that domain
3. If not (or if it's different), it creates/updates the record via the provider's API
4. The DNS server is automatically reconfigured

## OPNsense Setup

1. Go to **System > Access > Users** and create an API user
2. Generate API credentials in **System > Access > Users > [user] > API keys**
3. Ensure the user has access to the Unbound DNS service

