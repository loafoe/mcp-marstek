# mcp-marstek

A standalone [MCP](https://modelcontextprotocol.io) server that allows an
agent to read status from and control a Marstek battery system (Venus E3,
Venus C, Venus D, Venus A) over the local LAN.

Built with the official
[modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk).

## Disclaimer

This project is not affiliated with, endorsed by, or connected to Marstek in
any way. This is an independent, community-developed tool.

**USE AT YOUR OWN RISK.** This software interacts with battery hardware and
energy systems. Improper use could potentially affect your battery system's
operation. The authors and contributors are not responsible for any damage,
data loss, or other issues that may arise from using this software.

## Prerequisites

1. Device must be connected to your local network (WiFi or Ethernet)
2. Open API feature must be enabled in the Marstek mobile app
3. Default UDP port is 30000 (configurable in the app, recommended range:
   49152-65535)

## Tools

### Read-only tools (always available)

| Tool | Description |
|------|-------------|
| `get_device` | Get device information (model, firmware, MAC, IP) |
| `get_energy_status` | **Combined**: battery SOC/temperature/capacity, solar power/voltage/current, grid and battery power flows, cumulative energy totals, and CT clamp per-phase readings |
| `get_operating_mode` | Get current operating mode (pairs with `set_operating_mode`) |
| `get_network_status` | **Combined**: WiFi connection details (SSID, signal, IP) and Bluetooth state |

### Write tools (disabled in read-only mode)

| Tool | Description |
|------|-------------|
| `set_operating_mode` | **Combined**: switch to any mode (auto, ai, ups, passive) with optional power/countdown for passive |
| `set_grid_export_limit` | Set device power rating/grid export limit (800, 1200, 1500, 2200, or 2500W) via `Set.Ver` |
| `set_led` | Control panel LED on/off |
| `set_dod` | Set depth of discharge (30-88%) |

## Configuration

See [`config.example.yaml`](./config.example.yaml). At minimum you need to
set the battery IP address:

```yaml
battery:
  addr: "192.168.1.100"
```

Values may reference environment variables via `${VAR_NAME}` (expanded at
load time), so the battery address can be injected from a Kubernetes Secret
while the rest of the config lives in a plain ConfigMap.

## Running

```sh
go build -o mcp-marstek ./cmd/mcp-marstek
./mcp-marstek --config config.yaml --transport http --addr :8080
```

Flags (all overridable via env var):

| Flag           | Env var                        | Default                           |
|----------------|--------------------------------|-----------------------------------|
| `--config`     | `MCP_MARSTEK_CONFIG`           | `/etc/mcp-marstek/config.yaml`    |
| `--transport`  | `MCP_MARSTEK_TRANSPORT`        | `http`                            |
| `--addr`       | `MCP_MARSTEK_ADDR`             | `:8080`                           |
| `--path`       | `MCP_MARSTEK_PATH`             | `/mcp`                            |
| `--read-only`  | `MCP_MARSTEK_READ_ONLY`        | `false`                           |

`--transport stdio` runs the server over stdio for local MCP client testing
(e.g. Claude Desktop, `mcp-inspector`). The HTTP transport serves stateless
MCP (Streamable HTTP with `Stateless: true`) at `--path` and a `/health`
endpoint for Kubernetes probes.

## Read-only mode

Pass `--read-only` (or set `MCP_MARSTEK_READ_ONLY=true`) to disable all
write tools. This is useful when you want to grant an agent read-only access
to the battery system without allowing any settings to be changed.

## Container image

Images are built with [`ko`](https://ko.build) (no Dockerfile needed) and
published to `ghcr.io/loafoe/mcp-marstek`, signed keylessly with
[cosign](https://docs.sigstore.dev/cosign/overview/) via GitHub Actions OIDC.

Verify a signature:

```sh
cosign verify ghcr.io/loafoe/mcp-marstek:latest \
  --certificate-identity-regexp 'https://github.com/loafoe/mcp-marstek/.github/workflows/release.yml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Build locally:

```sh
KO_DOCKER_REPO=ko.local ko build --bare --local ./cmd/mcp-marstek
```

## Development

```sh
go vet ./...
go test ./... -race
```

## License

Apache License 2.0 - see [LICENSE](./LICENSE) for details.