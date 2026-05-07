package portal

import (
	"fmt"
	"log"
	"screencap/config"
	"screencap/dbusutil"
	"screencap/internal/util"

	"github.com/godbus/dbus/v5"
)

// CreateSession creates a new screen cast session and returns its path.
func (p *ScreenCastPortal) CreateSession() (dbus.ObjectPath, error) {
	reqToken := util.NewTOken()
	sessToken := util.NewTOken()

	reqPath := p.requestPath(reqToken)

	// CRITICAL: AddMatch must include sender='org.freedesktop.portal.Desktop'
	rule, err := dbusutil.AddMatch(p.conn, reqPath)
	if err != nil {
		log.Println("DBUS addmatch error ", err)
	}
	defer dbusutil.RemoveMatch(p.conn, rule)

	// Setup signal channel BEFORE making the call
	signalCh := make(chan *dbus.Signal, 10)
	p.conn.Signal(signalCh)
	defer p.conn.RemoveSignal(signalCh)

	// Make the call
	obj := p.conn.Object(config.PortalBusName, config.PortalObjectPath)
	call := obj.Call(config.PortalInterface+".CreateSession", 0, map[string]any{
		"session_handle_token": sessToken,
		"handle_token":         reqToken,
	})
	if call.Err != nil {
		return "", fmt.Errorf("CreateSession call failed: %w", call.Err)
	}

	// Debug: print the returned handle
	fmt.Printf("DEBUG: CreateSession returned handle: %v\n", call.Body)

	resp, err := dbusutil.WaitResponse(p.conn, reqPath, config.DefaultTImeout)
	if err != nil {
		return "", err
	}

	sessionHandle := resp.Results["session_handle"].Value().(string)
	return dbus.ObjectPath(sessionHandle), nil

}

// SelectSources configures what to capture (monitors, windows, cursor).
func (p *ScreenCastPortal) SelectSources(sessionPath dbus.ObjectPath) error {
	reqToken := util.NewTOken()
	reqPath := p.requestPath(reqToken)

	rule, err := dbusutil.AddMatch(p.conn, reqPath)
	if err != nil {
		log.Println("err in addmatch ", err)
	}
	defer dbusutil.RemoveMatch(p.conn, rule)

	signalCh := make(chan *dbus.Signal, 10)
	p.conn.Signal(signalCh)
	defer p.conn.RemoveSignal(signalCh)

	obj := p.conn.Object(config.PortalBusName, config.PortalObjectPath)
	call := obj.Call(config.PortalInterface+".SelectSources", 0, sessionPath, map[string]any{
		"handle_token": reqToken,
		"types":        uint32(1 | 2), // Monitor + Window
		"multiple":     false,
		"cursor_mode":  uint32(2), // Embedded
	})
	if call.Err != nil {
		return fmt.Errorf("SelectSources call failed: %w", call.Err)
	}

	_, err = dbusutil.WaitResponse(p.conn, reqPath, config.DefaultTImeout)
	if err != nil {
		return err
	}
	return nil

}

// Start triggers the GUI dialog for user to pick a screen/window.
// Returns the PipeWire node ID and stream properties.
func (p *ScreenCastPortal) Start(sessionPath dbus.ObjectPath) (uint32, map[string]dbus.Variant, error) {
	reqToken := util.NewTOken()
	reqPath := p.requestPath(reqToken)

	rule, err := dbusutil.AddMatch(p.conn, reqPath)
	if err != nil {
		log.Println("Error in addmatch ", err)
	}
	defer dbusutil.RemoveMatch(p.conn, rule)

	signalCh := make(chan *dbus.Signal, 10)
	p.conn.Signal(signalCh)
	defer p.conn.RemoveSignal(signalCh)

	fmt.Println("  Opening screen sharing dialog...")
	fmt.Println("  Please select a monitor or window to share.")

	obj := p.conn.Object(config.PortalBusName, config.PortalObjectPath)
	call := obj.Call(config.PortalInterface+".Start", 0, sessionPath, "", map[string]any{
		"handle_token": reqToken,
	})
	if call.Err != nil {
		return 0, nil, fmt.Errorf("Start call failed: %w", call.Err)
	}

	resp, err := dbusutil.WaitResponse(p.conn, reqPath, config.UserSelectTimeout)
	if err != nil {
		return 0, nil, err
	}
	streams := resp.Results["streams"]
	return p.parseStreams(map[string]dbus.Variant{
		"streams": streams,
	})
}


func (p *ScreenCastPortal) Capture() (*CaptureSession, error) {

	// 1. Create portal session
	sessionPath, err := p.CreateSession()
	if err != nil {
		return nil, err
	}

	// 2. Ask user to select monitor/window
	if err := p.SelectSources(sessionPath); err != nil {
		return nil, err
	}

	// 3. Start stream
	nodeID, props, err := p.Start(sessionPath)
	if err != nil {
		return nil, err
	}

	// 4. Open PipeWire remote
	pwFD, err := p.OpenPipeWireRemote(sessionPath)
	if err != nil {
		return nil, err
	}

	return &CaptureSession{
		SessionPath: sessionPath,
		NodeID:      nodeID,
		PipeWireFD:  pwFD,
		Props:       props,
	}, nil
}