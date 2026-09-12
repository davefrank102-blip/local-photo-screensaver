# Security

## Phase 0 (spike)

**There is no pairing, authentication, or TLS yet.**

- The companion server serves photo bytes to any client that can reach it on the LAN.
- Treat `--addr` / port `8787` as **LAN-only**. Use a host firewall; do not port-forward to the internet.
- Image URLs use **opaque IDs** (not filesystem paths). Path traversal on the ID is rejected.
- Photos are read only from the `--photos` directory tree.

Later phases are expected to add pairing, least-privilege binds, and stronger client verification. Until then: **LAN trust only**.

## Reporting

This repository is private and experimental. If you find a security issue while collaborating, contact the repo owner directly.
