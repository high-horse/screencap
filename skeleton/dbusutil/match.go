package dbusutil

import (
	"fmt"
	"screencap/config"
	"time"

	"github.com/godbus/dbus/v5"
)

type PortalResponse struct {
	Code    uint32
	Results map[string]dbus.Variant
}

func WaitResponse(
	conn *Conn,
	reqPath dbus.ObjectPath,
	timeout time.Duration,
) (*PortalResponse, error) {

	signalCh := make(chan *dbus.Signal, 10)
	conn.Signal(signalCh)
	defer conn.RemoveSignal(signalCh)

	deadline := time.After(timeout)

	response := &PortalResponse{} 
	for {
		select {

		case sig := <-signalCh:
			if sig == nil {
				continue
			}

			if sig.Name != config.PortalRequestResponse {
				continue
			}

			if string(sig.Path) != string(reqPath) {
				continue
			}

			if sig.Body == nil || len(sig.Body) < 2 {
				return nil, fmt.Errorf("invalid portal response")
			}

			if len(sig.Body) < 2 {
				return nil, fmt.Errorf("invalid portal response")
			}

			code, ok := sig.Body[0].(uint32)
			if !ok {
				return nil, fmt.Errorf("invalid response code type")
			}
			response.Code = code			

			if response.Code != 0 {
				return response, fmt.Errorf("portal error code %d", response.Code)
			}

			results, ok := sig.Body[1].(map[string]dbus.Variant)
			if !ok {
				return nil, fmt.Errorf("invalid response format")
			}

			response.Results = results
			return response, nil

		case <-deadline:
			return nil, fmt.Errorf("timeout waiting for portal response")
		}
	}
}

func AddMatch(conn *Conn, reqPath dbus.ObjectPath) (string, error) {
	rule := fmt.Sprintf(
		"type='signal',sender='%s',interface='org.freedesktop.portal.Request',member='Response',path='%s'",
		config.PortalBusName, reqPath,
	)

	if err := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule).Err; err != nil {
		return "", err
	}

	return rule, nil
}

func RemoveMatch(conn *Conn, rule string) {
	conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, rule)
}
