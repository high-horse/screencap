package portal

import (
	"screencap/dbusutil"

	"github.com/godbus/dbus/v5"
)

type ScreenCastPortal struct {
	conn   *dbusutil.Conn
	sender string
}

type CaptureSession struct {
	SessionPath dbus.ObjectPath
	NodeID      uint32
	PipeWireFD  int
	Props       map[string]dbus.Variant
}
