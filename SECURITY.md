# Security

## Trust model (MVP)

- **Admin UI** (`/`, `/api/v1/admin/*`): localhost only
- **Playlist + images**: reachable on the LAN bind address with **no authentication**
- Bind defaults to `0.0.0.0:8787` — restrict with Windows Firewall to your LAN profile

## Recommendations

1. Only run on a trusted home/LAN network
2. Create an inbound allow rule for TCP 8787 limited to Private networks when possible
3. Do not port-forward 8787 through your router
4. Treat Delete in the junk UI as destructive (removes files from disk)

## Reporting

Open a GitHub issue describing the concern. Do not file auth-bypass reports against the intentional MVP LAN model unless admin localhost isolation fails.
