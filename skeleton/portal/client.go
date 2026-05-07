package portal

import (
	"fmt"
	"screencap/config"
	"screencap/dbusutil"
	"strings"

	"github.com/godbus/dbus/v5"
)

// NewScreenCastPortal connects to the session D-Bus and returns a portal client.
func NewScreenCastPortal() (*ScreenCastPortal, error) {
	conn, err := dbusutil.NewSessionConn()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %w", err)
	}

	unique, err := conn.UniqueName()
	if err != nil {
		return nil, fmt.Errorf("failed to get unique bus name: %w", err)
	}
	// sender = unique name without leading ':', dots replaced with underscores
	sender := strings.TrimPrefix(unique, ":")
	sender = strings.ReplaceAll(sender, ".", "_")

	return &ScreenCastPortal{
		conn:   conn,
		sender: sender,
	}, nil
}

// Close cleans up the D-Bus connection.
func (p *ScreenCastPortal) Close() {
	p.conn.Close()
}

// GetAvailableSourceTypes returns what the portal can capture (monitor/window/virtual).
func (p *ScreenCastPortal) GetAvailableSourceTypes() (uint32, error) {
	obj := p.conn.Object(config.PortalBusName, config.PortalObjectPath)
	var sources uint32
	err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, config.PortalInterface, "AvailableSourceTypes").Store(&sources)
	return sources, err
}

// ─── Internal helpers ─────────────────────────────────────────────────────

func (p *ScreenCastPortal) requestPath(token string) dbus.ObjectPath {
	return dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", p.sender, token))
}

// parseStreams extracts nodeID and properties from the streams data.
func (p *ScreenCastPortal) parseStreams(results map[string]dbus.Variant) (uint32, map[string]dbus.Variant, error) {
	streamsValue, hasStreams := results["streams"]
	if !hasStreams {
		return 0, nil, fmt.Errorf("no 'streams' key in results")
	}

	streamsData := streamsValue.Value()

	// Helper to extract from a tuple [nodeID, props]
	extract := func(tuple []any) (uint32, map[string]dbus.Variant, error) {
		if len(tuple) < 2 {
			return 0, nil, fmt.Errorf("stream tuple has %d elements, expected at least 2", len(tuple))
		}

		var nodeID uint32
		switch id := tuple[0].(type) {
		case uint32:
			nodeID = id
		case int:
			nodeID = uint32(id)
		case int32:
			nodeID = uint32(id)
		default:
			return 0, nil, fmt.Errorf("node ID is not uint32, got %T", tuple[0])
		}

		props, ok := tuple[1].(map[string]dbus.Variant)
		if !ok {
			return 0, nil, fmt.Errorf("stream properties is not map, got %T", tuple[1])
		}

		return nodeID, props, nil
	}

	switch v := streamsData.(type) {
	case []any:
		if len(v) == 0 {
			return 0, nil, fmt.Errorf("no streams returned - user probably cancelled")
		}
		if firstStream, ok := v[0].([]any); ok {
			return extract(firstStream)
		}
		return extract(v)

	case [][]any:
		if len(v) == 0 {
			return 0, nil, fmt.Errorf("no streams returned - user probably cancelled")
		}
		tuple := make([]any, len(v[0]))
		for i, x := range v[0] {
			tuple[i] = x
		}
		return extract(tuple)

	default:
		return 0, nil, fmt.Errorf("unexpected streams data type: %T", streamsData)
	}
}
