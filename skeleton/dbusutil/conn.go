package dbusutil

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)


type Conn struct {
	*dbus.Conn
}

func NewSessionConn() (*Conn, error) {
	bus, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %w", err)
	}

	return &Conn{Conn: bus}, nil
}

func (c *Conn) Close() error {
	if c.Conn != nil {
		return c.Conn.Close()
	}
	return nil
}

func (c *Conn) Object(busName string, path dbus.ObjectPath) dbus.BusObject {
	return c.Conn.Object(busName, path)
}

func (c *Conn) Names() []string {
	return c.Conn.Names()
}

func (c *Conn) UniqueName() (string, error) {
	for _, n := range c.Names() {
		if len(n) > 0 && n[0] == ':' {
			return n, nil
		}
	}
	return "", fmt.Errorf("no unique bus name found")
}