package portal

import (
	"fmt"
	"screencap/config"

	"github.com/godbus/dbus/v5"
)

// OpenPipeWireRemote gets a file descriptor to connect to PipeWire.
func (p *ScreenCastPortal) OpenPipeWireRemote(sessionPath dbus.ObjectPath) (int, error) {
	obj := p.conn.Object(config.PortalBusName, config.PortalObjectPath)
	var fd dbus.UnixFD
	err := obj.Call(config.PortalInterface+".OpenPipeWireRemote", 0, sessionPath, map[string]any{}).Store(&fd)
	if err != nil {
		return -1, fmt.Errorf("OpenPipeWireRemote failed: %w", err)
	}
	return int(fd), nil
}
