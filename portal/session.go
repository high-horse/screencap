package portal

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

// Session holds the D-Bus connection and the session handle returned
// by the portal after CreateSession succeeds.
type Session struct {
	Conn   *dbus.Conn
	Handle dbus.ObjectPath
}

// NewSession connects to the session D-Bus and calls
// org.freedesktop.portal.ScreenCast.CreateSession.
func NewSession() (*Session, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect session bus: %w", err)
	}

	handleToken   := nextToken()
	sessionToken  := nextToken()

	obj := conn.Object(portalDest, portalPath)

	var requestPath dbus.ObjectPath
	err = obj.Call(
		screencastIface+".CreateSession", 0,
		map[string]dbus.Variant{
			"handle_token":         dbus.MakeVariant(handleToken),
			"session_handle_token": dbus.MakeVariant(sessionToken),
		},
	).Store(&requestPath)
	if err != nil {
		return nil, fmt.Errorf("CreateSession call: %w", err)
	}

	resp, err := AwaitResponse(conn, requestPath)
	if err != nil {
		return nil, fmt.Errorf("CreateSession response: %w", err)
	}

	sessionHandle, ok := resp.Results["session_handle"]
	if !ok {
		return nil, fmt.Errorf("no session_handle in response")
	}

	return &Session{
		Conn:   conn,
		Handle: dbus.ObjectPath(sessionHandle.Value().(string)),
	}, nil
}

// Close sends Close on the session object so the compositor tears it down.
func (s *Session) Close() error {
	obj := s.Conn.Object(portalDest, s.Handle)
	return obj.Call("org.freedesktop.portal.Session.Close", 0).Err
}