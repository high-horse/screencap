package portal

import (
	"screencap/dbusutil"
)

type ScreenCastPortal struct {
	conn   *dbusutil.Conn
	sender string
}
