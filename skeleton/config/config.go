package config

import "time"

const (
	PortalBusName    = "org.freedesktop.portal.Desktop"
	PortalObjectPath = "/org/freedesktop/portal/desktop"
	PortalInterface  = "org.freedesktop.portal.ScreenCast"

	PortalRequestResponse = "org.freedesktop.portal.Request.Response"

	DefaultTImeout = 30 * time.Second
	UserSelectTimeout = 60 * time.Second
)