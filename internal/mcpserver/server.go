// Package mcpserver wires the Marstek battery API into an MCP server
// exposing read and (optionally) write tools.
package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/loafoe/go-marstek/pkg/marstek"
	"github.com/loafoe/mcp-marstek/internal/config"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// New builds an MCP server with tools for interacting with a Marstek battery.
// If readOnly is true, only read-only information tools are registered.
func New(cfg *config.Config, logger *slog.Logger) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-marstek",
		Version: Version,
	}, nil)

	registerReadTools(server, cfg, logger)
	if !cfg.ReadOnly {
		registerWriteTools(server, cfg, logger)
	}

	return server
}

// newClient creates a Marstek client from the config.
func newClient(cfg *config.Config) *marstek.Client {
	return marstek.NewClient(cfg.Battery.Addr,
		marstek.WithPort(cfg.Battery.Port),
		marstek.WithTimeout(cfg.Battery.Timeout),
	)
}

// ---------------------------------------------------------------------------
// Read-only tools
// ---------------------------------------------------------------------------

func registerReadTools(server *mcp.Server, cfg *config.Config, logger *slog.Logger) {
	// get_device: hardware identity
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_device",
		Description: "Get Marstek device information including model, firmware version, MAC addresses, and IP address.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		BLBMac string `json:"ble_mac,omitempty" jsonschema:"BLE MAC address of the device. Defaults to '0'."`
	}) (*mcp.CallToolResult, struct {
		Device   string `json:"device"`
		Version  int    `json:"ver"`
		BLEMAC   string `json:"ble_mac"`
		WifiMAC  string `json:"wifi_mac"`
		WifiName string `json:"wifi_name"`
		IP       string `json:"ip"`
	}, error) {
		bleMac := in.BLBMac
		if bleMac == "" {
			bleMac = "0"
		}
		client := newClient(cfg)
		info, err := client.GetDevice(bleMac)
		if err != nil {
			return nil, struct {
				Device   string `json:"device"`
				Version  int    `json:"ver"`
				BLEMAC   string `json:"ble_mac"`
				WifiMAC  string `json:"wifi_mac"`
				WifiName string `json:"wifi_name"`
				IP       string `json:"ip"`
			}{}, fmt.Errorf("get_device: %w", err)
		}
		return nil, struct {
			Device   string `json:"device"`
			Version  int    `json:"ver"`
			BLEMAC   string `json:"ble_mac"`
			WifiMAC  string `json:"wifi_mac"`
			WifiName string `json:"wifi_name"`
			IP       string `json:"ip"`
		}{
			Device:   info.Device,
			Version:  info.Version,
			BLEMAC:   info.BLEMAC,
			WifiMAC:  info.WifiMAC,
			WifiName: info.WifiName,
			IP:       info.IP,
		}, nil
	})

	// get_energy_status: combines battery, PV, ES, and EM into one call
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_energy_status",
		Description: "Get comprehensive energy system status in a single call: battery SOC/temperature/capacity, solar (PV) power/voltage/current, grid and battery power flows, cumulative energy totals, and CT clamp per-phase readings. The primary tool for reading the full state of the battery and home energy system.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		InstanceID int `json:"instance_id,omitempty" jsonschema:"System instance ID. Defaults to 0."`
	}) (*mcp.CallToolResult, struct {
		Battery []struct {
			SOC           int      `json:"soc"`
			Charging      bool     `json:"charging"`
			Discharging   bool     `json:"discharging"`
			Temperature   *float64 `json:"temperature,omitempty"`
			Capacity      *float64 `json:"capacity,omitempty"`
			RatedCapacity *float64 `json:"rated_capacity,omitempty"`
		} `json:"battery"`
		Solar struct {
			Power   float64 `json:"power"`
			Voltage float64 `json:"voltage"`
			Current float64 `json:"current"`
		} `json:"solar"`
		System struct {
			BatSOC                *int     `json:"bat_soc,omitempty"`
			BatCap                *float64 `json:"bat_cap,omitempty"`
			BatPower              *float64 `json:"bat_power,omitempty"`
			PVPower               *float64 `json:"pv_power,omitempty"`
			OngridPower           *float64 `json:"ongrid_power,omitempty"`
			OffgridPower          *float64 `json:"offgrid_power,omitempty"`
			TotalPVEnergy         *float64 `json:"total_pv_energy,omitempty"`
			TotalGridOutputEnergy *float64 `json:"total_grid_output_energy,omitempty"`
			TotalGridInputEnergy  *float64 `json:"total_grid_input_energy,omitempty"`
			TotalLoadEnergy       *float64 `json:"total_load_energy,omitempty"`
		} `json:"system"`
		Meter struct {
			CTState      *int     `json:"ct_state,omitempty"`
			APower       *float64 `json:"a_power,omitempty"`
			BPower       *float64 `json:"b_power,omitempty"`
			CPower       *float64 `json:"c_power,omitempty"`
			TotalPower   *float64 `json:"total_power,omitempty"`
			InputEnergy  *float64 `json:"input_energy,omitempty"`
			OutputEnergy *float64 `json:"output_energy,omitempty"`
		} `json:"meter"`
	}, error) {
		id := in.InstanceID
		client := newClient(cfg)

		bat, batErr := client.GetBatteryStatus(id)
		pv, pvErr := client.GetPVStatus(id)
		es, esErr := client.GetESStatus(id)
		em, emErr := client.GetEMStatus(id)

		// If all four failed, return an error. Partial data is still useful.
		if batErr != nil && pvErr != nil && esErr != nil && emErr != nil {
			return nil, struct {
				Battery []struct {
					SOC           int      `json:"soc"`
					Charging      bool     `json:"charging"`
					Discharging   bool     `json:"discharging"`
					Temperature   *float64 `json:"temperature,omitempty"`
					Capacity      *float64 `json:"capacity,omitempty"`
					RatedCapacity *float64 `json:"rated_capacity,omitempty"`
				} `json:"battery"`
				Solar struct {
					Power   float64 `json:"power"`
					Voltage float64 `json:"voltage"`
					Current float64 `json:"current"`
				} `json:"solar"`
				System struct {
					BatSOC                *int     `json:"bat_soc,omitempty"`
					BatCap                *float64 `json:"bat_cap,omitempty"`
					BatPower              *float64 `json:"bat_power,omitempty"`
					PVPower               *float64 `json:"pv_power,omitempty"`
					OngridPower           *float64 `json:"ongrid_power,omitempty"`
					OffgridPower          *float64 `json:"offgrid_power,omitempty"`
					TotalPVEnergy         *float64 `json:"total_pv_energy,omitempty"`
					TotalGridOutputEnergy *float64 `json:"total_grid_output_energy,omitempty"`
					TotalGridInputEnergy  *float64 `json:"total_grid_input_energy,omitempty"`
					TotalLoadEnergy       *float64 `json:"total_load_energy,omitempty"`
				} `json:"system"`
				Meter struct {
					CTState      *int     `json:"ct_state,omitempty"`
					APower       *float64 `json:"a_power,omitempty"`
					BPower       *float64 `json:"b_power,omitempty"`
					CPower       *float64 `json:"c_power,omitempty"`
					TotalPower   *float64 `json:"total_power,omitempty"`
					InputEnergy  *float64 `json:"input_energy,omitempty"`
					OutputEnergy *float64 `json:"output_energy,omitempty"`
				} `json:"meter"`
			}{}, fmt.Errorf("get_energy_status: battery=%w, pv=%w, es=%w, em=%w", batErr, pvErr, esErr, emErr)
		}

		outputBattery := make([]struct {
			SOC           int      `json:"soc"`
			Charging      bool     `json:"charging"`
			Discharging   bool     `json:"discharging"`
			Temperature   *float64 `json:"temperature,omitempty"`
			Capacity      *float64 `json:"capacity,omitempty"`
			RatedCapacity *float64 `json:"rated_capacity,omitempty"`
		}, 0)
		if batErr == nil && bat != nil {
			outputBattery = append(outputBattery, struct {
				SOC           int      `json:"soc"`
				Charging      bool     `json:"charging"`
				Discharging   bool     `json:"discharging"`
				Temperature   *float64 `json:"temperature,omitempty"`
				Capacity      *float64 `json:"capacity,omitempty"`
				RatedCapacity *float64 `json:"rated_capacity,omitempty"`
			}{
				SOC:           bat.SOC,
				Charging:      bat.ChargFlag,
				Discharging:   bat.DischrgFlag,
				Temperature:   bat.BatTemp,
				Capacity:      bat.BatCapacity,
				RatedCapacity: bat.RatedCapacity,
			})
		}

		outputSolar := struct {
			Power   float64 `json:"power"`
			Voltage float64 `json:"voltage"`
			Current float64 `json:"current"`
		}{}
		if pvErr == nil && pv != nil {
			outputSolar.Power = pv.PVPower
			outputSolar.Voltage = pv.PVVoltage
			outputSolar.Current = pv.PVCurrent
		}

		outputSystem := struct {
			BatSOC                *int     `json:"bat_soc,omitempty"`
			BatCap                *float64 `json:"bat_cap,omitempty"`
			BatPower              *float64 `json:"bat_power,omitempty"`
			PVPower               *float64 `json:"pv_power,omitempty"`
			OngridPower           *float64 `json:"ongrid_power,omitempty"`
			OffgridPower          *float64 `json:"offgrid_power,omitempty"`
			TotalPVEnergy         *float64 `json:"total_pv_energy,omitempty"`
			TotalGridOutputEnergy *float64 `json:"total_grid_output_energy,omitempty"`
			TotalGridInputEnergy  *float64 `json:"total_grid_input_energy,omitempty"`
			TotalLoadEnergy       *float64 `json:"total_load_energy,omitempty"`
		}{}
		if esErr == nil && es != nil {
			outputSystem.BatSOC = es.BatSOC
			outputSystem.BatCap = es.BatCap
			outputSystem.BatPower = es.BatPower
			outputSystem.PVPower = es.PVPower
			outputSystem.OngridPower = es.OngridPower
			outputSystem.OffgridPower = es.OffgridPower
			outputSystem.TotalPVEnergy = es.TotalPVEnergy
			outputSystem.TotalGridOutputEnergy = es.TotalGridOutputEnergy
			outputSystem.TotalGridInputEnergy = es.TotalGridInputEnergy
			outputSystem.TotalLoadEnergy = es.TotalLoadEnergy
		}

		outputMeter := struct {
			CTState      *int     `json:"ct_state,omitempty"`
			APower       *float64 `json:"a_power,omitempty"`
			BPower       *float64 `json:"b_power,omitempty"`
			CPower       *float64 `json:"c_power,omitempty"`
			TotalPower   *float64 `json:"total_power,omitempty"`
			InputEnergy  *float64 `json:"input_energy,omitempty"`
			OutputEnergy *float64 `json:"output_energy,omitempty"`
		}{}
		if emErr == nil && em != nil {
			outputMeter.CTState = em.CTState
			outputMeter.APower = em.APower
			outputMeter.BPower = em.BPower
			outputMeter.CPower = em.CPower
			outputMeter.TotalPower = em.TotalPower
			outputMeter.InputEnergy = em.InputEnergy
			outputMeter.OutputEnergy = em.OutputEnergy
		}

		return nil, struct {
			Battery []struct {
				SOC           int      `json:"soc"`
				Charging      bool     `json:"charging"`
				Discharging   bool     `json:"discharging"`
				Temperature   *float64 `json:"temperature,omitempty"`
				Capacity      *float64 `json:"capacity,omitempty"`
				RatedCapacity *float64 `json:"rated_capacity,omitempty"`
			} `json:"battery"`
			Solar struct {
				Power   float64 `json:"power"`
				Voltage float64 `json:"voltage"`
				Current float64 `json:"current"`
			} `json:"solar"`
			System struct {
				BatSOC                *int     `json:"bat_soc,omitempty"`
				BatCap                *float64 `json:"bat_cap,omitempty"`
				BatPower              *float64 `json:"bat_power,omitempty"`
				PVPower               *float64 `json:"pv_power,omitempty"`
				OngridPower           *float64 `json:"ongrid_power,omitempty"`
				OffgridPower          *float64 `json:"offgrid_power,omitempty"`
				TotalPVEnergy         *float64 `json:"total_pv_energy,omitempty"`
				TotalGridOutputEnergy *float64 `json:"total_grid_output_energy,omitempty"`
				TotalGridInputEnergy  *float64 `json:"total_grid_input_energy,omitempty"`
				TotalLoadEnergy       *float64 `json:"total_load_energy,omitempty"`
			} `json:"system"`
			Meter struct {
				CTState      *int     `json:"ct_state,omitempty"`
				APower       *float64 `json:"a_power,omitempty"`
				BPower       *float64 `json:"b_power,omitempty"`
				CPower       *float64 `json:"c_power,omitempty"`
				TotalPower   *float64 `json:"total_power,omitempty"`
				InputEnergy  *float64 `json:"input_energy,omitempty"`
				OutputEnergy *float64 `json:"output_energy,omitempty"`
			} `json:"meter"`
		}{
			Battery: outputBattery,
			Solar:   outputSolar,
			System:  outputSystem,
			Meter:   outputMeter,
		}, nil
	})

	// get_operating_mode: current mode (read companion to set_operating_mode)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_operating_mode",
		Description: "Get the current operating mode of the energy system (Auto, AI, Manual, Passive, or UPS) along with live power and battery SOC data. Pairs with set_operating_mode.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		InstanceID int `json:"instance_id,omitempty" jsonschema:"Energy system instance ID. Defaults to 0."`
	}) (*mcp.CallToolResult, struct {
		Mode        string   `json:"mode"`
		OngridPower *float64 `json:"ongrid_power,omitempty"`
		BatSOC      *int     `json:"bat_soc,omitempty"`
	}, error) {
		client := newClient(cfg)
		status, err := client.GetESMode(in.InstanceID)
		if err != nil {
			return nil, struct {
				Mode        string   `json:"mode"`
				OngridPower *float64 `json:"ongrid_power,omitempty"`
				BatSOC      *int     `json:"bat_soc,omitempty"`
			}{}, fmt.Errorf("get_operating_mode: %w", err)
		}
		return nil, struct {
			Mode        string   `json:"mode"`
			OngridPower *float64 `json:"ongrid_power,omitempty"`
			BatSOC      *int     `json:"bat_soc,omitempty"`
		}{
			Mode:        status.Mode,
			OngridPower: status.OngridPower,
			BatSOC:      status.BatSOC,
		}, nil
	})

	// get_network_status: merges WiFi + BLE
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_network_status",
		Description: "Get network connectivity status including WiFi connection details (SSID, signal strength, IP) and Bluetooth state.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		InstanceID int `json:"instance_id,omitempty" jsonschema:"Network instance ID. Defaults to 0."`
	}) (*mcp.CallToolResult, struct {
		WiFi *struct {
			SSID  *string `json:"ssid,omitempty"`
			RSSI  int     `json:"rssi"`
			StaIP *string `json:"sta_ip,omitempty"`
		} `json:"wifi,omitempty"`
		BLE *struct {
			State  string `json:"state"`
			BLEMAC string `json:"ble_mac"`
		} `json:"ble,omitempty"`
	}, error) {
		id := in.InstanceID
		client := newClient(cfg)

		wifi, wifiErr := client.GetWifiStatus(id)
		ble, bleErr := client.GetBLEStatus(id)

		if wifiErr != nil && bleErr != nil {
			return nil, struct {
				WiFi *struct {
					SSID  *string `json:"ssid,omitempty"`
					RSSI  int     `json:"rssi"`
					StaIP *string `json:"sta_ip,omitempty"`
				} `json:"wifi,omitempty"`
				BLE *struct {
					State  string `json:"state"`
					BLEMAC string `json:"ble_mac"`
				} `json:"ble,omitempty"`
			}{}, fmt.Errorf("get_network_status: wifi=%w, ble=%w", wifiErr, bleErr)
		}

		out := struct {
			WiFi *struct {
				SSID  *string `json:"ssid,omitempty"`
				RSSI  int     `json:"rssi"`
				StaIP *string `json:"sta_ip,omitempty"`
			} `json:"wifi,omitempty"`
			BLE *struct {
				State  string `json:"state"`
				BLEMAC string `json:"ble_mac"`
			} `json:"ble,omitempty"`
		}{}

		if wifiErr == nil && wifi != nil {
			out.WiFi = &struct {
				SSID  *string `json:"ssid,omitempty"`
				RSSI  int     `json:"rssi"`
				StaIP *string `json:"sta_ip,omitempty"`
			}{
				SSID:  wifi.SSID,
				RSSI:  wifi.RSSI,
				StaIP: wifi.StaIP,
			}
		}
		if bleErr == nil && ble != nil {
			out.BLE = &struct {
				State  string `json:"state"`
				BLEMAC string `json:"ble_mac"`
			}{
				State:  ble.State,
				BLEMAC: ble.BLEMAC,
			}
		}

		return nil, out, nil
	})
}

// ---------------------------------------------------------------------------
// Write tools (disabled in read-only mode)
// ---------------------------------------------------------------------------

const (
	modeAuto    = "auto"
	modeAI      = "ai"
	modeUPS     = "ups"
	modePassive = "passive"
)

var validModes = []string{modeAuto, modeAI, modeUPS, modePassive}

func registerWriteTools(server *mcp.Server, cfg *config.Config, logger *slog.Logger) {
	// set_operating_mode: single tool for all mode switching
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_operating_mode",
		Description: "Switch the battery operating mode. Modes: auto (optimize based on solar/consumption), ai (AI-driven, requires cloud), ups (uninterruptible power supply), passive (external control with power setpoint and countdown). For passive mode, provide power (watts) and cd_time (seconds).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		Mode       string `json:"mode" jsonschema:"Operating mode to set: auto, ai, ups, or passive."`
		InstanceID int    `json:"instance_id,omitempty" jsonschema:"Energy system instance ID. Defaults to 0."`
		Power      *int   `json:"power,omitempty" jsonschema:"Power setpoint in watts (required for passive mode)."`
		CDTime     *int   `json:"cd_time,omitempty" jsonschema:"Countdown time in seconds (required for passive mode)."`
	}) (*mcp.CallToolResult, struct {
		Success bool   `json:"success"`
		Mode    string `json:"mode"`
	}, error) {
		mode := strings.ToLower(strings.TrimSpace(in.Mode))
		if !slices.Contains(validModes, mode) {
			return nil, struct {
				Success bool   `json:"success"`
				Mode    string `json:"mode"`
			}{}, fmt.Errorf("set_operating_mode: invalid mode %q, valid modes: %v", in.Mode, validModes)
		}

		client := newClient(cfg)
		var result *marstek.SetResult
		var err error

		switch mode {
		case modeAuto:
			result, err = client.SetAutoMode(in.InstanceID)
		case modeAI:
			result, err = client.SetAIMode(in.InstanceID)
		case modeUPS:
			result, err = client.SetUPSMode(in.InstanceID)
		case modePassive:
			power := 100
			cdTime := 300
			if in.Power != nil {
				power = *in.Power
			}
			if in.CDTime != nil {
				cdTime = *in.CDTime
			}
			result, err = client.SetPassiveMode(in.InstanceID, power, cdTime)
		}

		if err != nil {
			return nil, struct {
				Success bool   `json:"success"`
				Mode    string `json:"mode"`
			}{}, fmt.Errorf("set_operating_mode: %w", err)
		}

		return nil, struct {
			Success bool   `json:"success"`
			Mode    string `json:"mode"`
		}{Success: result.SetResult, Mode: mode}, nil
	})

	// set_led
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_led",
		Description: "Control the battery panel LED (on/off).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		On bool `json:"on" jsonschema:"Set to true to turn LED on, false to turn it off."`
	}) (*mcp.CallToolResult, struct {
		Success bool `json:"success"`
	}, error) {
		client := newClient(cfg)
		result, err := client.SetLED(in.On)
		if err != nil {
			return nil, struct {
				Success bool `json:"success"`
			}{}, fmt.Errorf("set_led: %w", err)
		}
		return nil, struct {
			Success bool `json:"success"`
		}{Success: result.SetResult}, nil
	})

	// set_dod
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_dod",
		Description: "Set the Depth of Discharge (DOD) limit for the battery. Value must be between 30 and 88 percent.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		Value int `json:"value" jsonschema:"Depth of discharge percentage. Must be between 30 and 88."`
	}) (*mcp.CallToolResult, struct {
		Success bool `json:"success"`
	}, error) {
		client := newClient(cfg)
		result, err := client.SetDOD(in.Value)
		if err != nil {
			return nil, struct {
				Success bool `json:"success"`
			}{}, fmt.Errorf("set_dod: %w", err)
		}
		return nil, struct {
			Success bool `json:"success"`
		}{Success: result.SetResult}, nil
	})

	// set_grid_export_limit
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_grid_export_limit",
		Description: "Set the maximum power that can be exported to the grid. Valid values: 800, 1200, 1500, 2200, or 2500 watts. This calls the Marstek Set.Ver API which is write-only: the current limit cannot be read back. The device confirms success via a set_result boolean. Use get_energy_status to see the actual live grid export power.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		Limit int `json:"limit" jsonschema:"Maximum grid export power in watts. Must be one of: 800, 1200, 1500, 2200, 2500."`
	}) (*mcp.CallToolResult, struct {
		Success bool `json:"success"`
	}, error) {
		client := newClient(cfg)
		result, err := client.SetGridExportLimit(in.Limit)
		if err != nil {
			return nil, struct {
				Success bool `json:"success"`
			}{}, fmt.Errorf("set_grid_export_limit: %w", err)
		}
		return nil, struct {
			Success bool `json:"success"`
		}{Success: result.SetResult}, nil
	})
}
