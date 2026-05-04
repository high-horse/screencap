package portal

import (
	"fmt"
	"os"

	"github.com/godbus/dbus/v5"
)

// SourceType mirrors the portal enum.
type SourceType uint32

const (
	SourceMonitor SourceType = 1
	SourceWindow  SourceType = 2
	SourceVirtual SourceType = 4
)

// SelectSources asks the portal which source types to allow.
// The user will later pick the actual source in the Start dialog.
func (s *Session) SelectSources(sourceType SourceType, multiple bool) error {
	obj := s.Conn.Object(portalDest, portalPath)

	var requestPath dbus.ObjectPath
	err := obj.Call(
		screencastIface+".SelectSources", 0,
		s.Handle,
		map[string]dbus.Variant{
			"handle_token": dbus.MakeVariant(nextToken()),
			"types":        dbus.MakeVariant(uint32(sourceType)),
			"multiple":     dbus.MakeVariant(multiple),
			"cursor_mode":  dbus.MakeVariant(uint32(2)), // 2 = embedded cursor
		},
	).Store(&requestPath)
	if err != nil {
		return fmt.Errorf("SelectSources call: %w", err)
	}

	if _, err = AwaitResponse(s.Conn, requestPath); err != nil {
		return fmt.Errorf("SelectSources response: %w", err)
	}
	return nil
}

// Start triggers the compositor's source-picker dialog.
// Returns the list of selected stream node IDs.
func (s *Session) Start() ([]uint32, error) {
	obj := s.Conn.Object(portalDest, portalPath)

	var requestPath dbus.ObjectPath
	err := obj.Call(
		screencastIface+".Start", 0,
		s.Handle,
		"", // parent window handle (empty = no parent)
		map[string]dbus.Variant{
			"handle_token": dbus.MakeVariant(nextToken()),
		},
	).Store(&requestPath)
	if err != nil {
		return nil, fmt.Errorf("Start call: %w", err)
	}

	resp, err := AwaitResponse(s.Conn, requestPath)
	if err != nil {
		return nil, fmt.Errorf("Start response: %w", err)
	}

	// streams is []struct{ node_id uint32; properties map[string]dbus.Variant }
	streams, ok := resp.Results["streams"]
	if !ok {
		return nil, fmt.Errorf("no streams in Start response")
	}

	// dbus returns this as [][]interface{}
	raw, ok := streams.Value().([][]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected streams type: %T", streams.Value())
	}

	var nodeIDs []uint32
	for _, s := range raw {
		if len(s) < 1 {
			continue
		}
		id, ok := s[0].(uint32)
		if !ok {
			continue
		}
		nodeIDs = append(nodeIDs, id)
	}
	return nodeIDs, nil
}

// OpenPipeWireRemote returns an *os.File wrapping the PipeWire socket fd.
// Pass this fd to pw_context_connect_fd (or GStreamer's pipewiresrc).
func (s *Session) OpenPipeWireRemote() (*os.File, error) {
	obj := s.Conn.Object(portalDest, portalPath)

	var fd dbus.UnixFD
	err := obj.Call(
		screencastIface+".OpenPipeWireRemote", 0,
		s.Handle,
		map[string]dbus.Variant{},
	).Store(&fd)
	if err != nil {
		return nil, fmt.Errorf("OpenPipeWireRemote: %w", err)
	}

	return os.NewFile(uintptr(fd), "pipewire-remote"), nil
}