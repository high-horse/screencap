package portal

import(
	"fmt"
	"sync/atomic"

	"github.com/godbus/dbus/v5"
)

const (
	portalDest      = "org.freedesktop.portal.Desktop"
	portalPath      = "/org/freedesktop/portal/desktop"
	screencastIface = "org.freedesktop.portal.ScreenCast"
	requestIface    = "org.freedesktop.portal.Request"
)

var tokenCounter uint64

func nextToken() string {
	n := atomic.AddUint64(&tokenCounter, 1)
	return fmt.Sprintf("screencap%d", n)
}

// Response is what every portal async call resolves to.
type Response struct {
	Code    uint32
	Results map[string]dbus.Variant
}


// AwaitResponse subscribes to the Request object path and blocks until
// the portal fires the Response signal. Every portal call returns a
// request path; pass it here to get the actual result.
func AwaitResponse(conn *dbus.Conn, requestPath dbus.ObjectPath) (*Response, error) {
	matchRule := fmt.Sprintf(
		"type='signal',interface='%s',member='Response',path='%s'",
		requestIface, requestPath,
	)
	if err := conn.BusObject().Call(
		"org.freedesktop.DBus.AddMatch", 0, matchRule,
	).Err; err != nil {
		return nil, fmt.Errorf("AddMatch: %w", err)
	}
	defer conn.BusObject().Call(
		"org.freedesktop.DBus.RemoveMatch", 0, matchRule,
	)

	ch := make(chan *dbus.Signal, 1)
	conn.Signal(ch)
	defer conn.RemoveSignal(ch)

	sig := <-ch

	if len(sig.Body) < 2 {
		return nil, fmt.Errorf("unexpected signal body length: %d", len(sig.Body))
	}
	code, ok := sig.Body[0].(uint32)
	if !ok {
		return nil, fmt.Errorf("response code not uint32")
	}
	results, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return nil, fmt.Errorf("response results not map")
	}
	if code != 0 {
		return nil, fmt.Errorf("portal response code %d (1=cancelled, 2=ended)", code)
	}
	return &Response{Code: code, Results: results}, nil
}