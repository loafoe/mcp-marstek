package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/loafoe/mcp-marstek/internal/config"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestServer_ReadOnly_NoWriteTools verifies that in read-only mode only
// read tools are registered.
func TestServer_ReadOnly_NoWriteTools(t *testing.T) {
	cfg := &config.Config{
		Battery: config.BatteryConfig{
			Addr: "192.168.1.100",
		},
		ReadOnly: true,
	}
	server := New(cfg, discardLogger())
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)

	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer session.Close()

	var names []string
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() error = %v", err)
		}
		names = append(names, tool.Name)
	}

	writeTools := map[string]bool{
		"set_operating_mode":    true,
		"set_grid_export_limit": true,
		"set_led":               true,
		"set_dod":               true,
	}
	for _, name := range names {
		if writeTools[name] {
			t.Errorf("unexpected write tool %q in read-only mode", name)
		}
	}
	// Verify read tools are present
	readTools := []string{"get_device", "get_energy_status", "get_operating_mode", "get_network_status"}
	for _, name := range readTools {
		found := false
		for _, n := range names {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected read tool %q to be present, got tools: %v", name, names)
		}
	}
}

// TestServer_ReadWrite_HasWriteTools verifies write tools are registered
// when not in read-only mode.
func TestServer_ReadWrite_HasWriteTools(t *testing.T) {
	cfg := &config.Config{
		Battery: config.BatteryConfig{
			Addr: "192.168.1.100",
		},
		ReadOnly: false,
	}
	server := New(cfg, discardLogger())
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)

	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer session.Close()

	var names []string
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() error = %v", err)
		}
		names = append(names, tool.Name)
	}

	writeTools := []string{"set_operating_mode", "set_grid_export_limit", "set_led", "set_dod"}
	for _, name := range writeTools {
		found := false
		for _, n := range names {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected write tool %q to be present, got tools: %v", name, names)
		}
	}
}

// TestServer_ListTools verifies tools/list works
func TestServer_ListTools(t *testing.T) {
	cfg := &config.Config{
		Battery: config.BatteryConfig{
			Addr: "192.168.1.100",
		},
		ReadOnly: false,
	}
	server := New(cfg, discardLogger())
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)

	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer session.Close()

	res, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(res.Tools) == 0 {
		t.Fatal("expected at least one tool")
	}
}

// TestStateless_ServerDiscover verifies the SEP-2575 server/discover RPC
func TestStateless_ServerDiscover(t *testing.T) {
	cfg := &config.Config{
		Battery: config.BatteryConfig{
			Addr: "192.168.1.100",
		},
		ReadOnly: false,
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return New(cfg, discardLogger())
	}, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	meta := metaBlock("2026-07-28")
	body := `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{` + meta + `}}`
	resp := mustPostMCP(t, srv.URL, "2026-07-28", "server/discover", "", body)

	var dr struct {
		Result struct {
			SupportedVersions []string `json:"supportedVersions"`
			Capabilities      struct {
				Tools *struct{} `json:"tools"`
			} `json:"capabilities"`
		} `json:"result"`
		Error *struct{} `json:"error"`
	}
	if err := json.Unmarshal([]byte(resp), &dr); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, resp)
	}
	if dr.Error != nil {
		t.Fatalf("unexpected error in server/discover: %s", resp)
	}

	if len(dr.Result.SupportedVersions) == 0 || dr.Result.SupportedVersions[0] != "2026-07-28" {
		t.Errorf("supportedVersions = %v, want latest to be 2026-07-28", dr.Result.SupportedVersions)
	}
	if dr.Result.Capabilities.Tools == nil {
		t.Error("expected tools capability to be advertised")
	}
}

// TestSetOperatingMode_InvalidMode verifies the merged set_operating_mode
// tool rejects unknown mode values.
func TestSetOperatingMode_InvalidMode(t *testing.T) {
	cfg := &config.Config{
		Battery: config.BatteryConfig{
			Addr: "192.168.1.100",
		},
		ReadOnly: false,
	}
	server := New(cfg, discardLogger())
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)

	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "set_operating_mode",
		Arguments: map[string]any{"mode": "invalid_mode_xyz"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !res.IsError {
		t.Fatal("expected tool error for invalid mode, got success")
	}
}

// Helper functions for stateless HTTP tests

func metaBlock(protoVer string) string {
	return `"_meta":{"io.modelcontextprotocol/protocolVersion":` + `"` + protoVer + `"` + `,"io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"test-client","version":"0.1.0"}}`
}

func mustPostMCP(t *testing.T, url, protocolVersion, method, name, body string) string {
	t.Helper()
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", protocolVersion)
	req.Header.Set("Mcp-Method", method)
	if name != "" {
		req.Header.Set("Mcp-Name", name)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return string(respBody)
}
