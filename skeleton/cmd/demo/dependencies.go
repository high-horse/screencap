package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/godbus/dbus/v5"
)

func CheckDependencies() error {
	var errs []string
	var missingPackages []string

	// ─── Helper functions ───────────────────────────────────────────────

	checkBinary := func(name string, pkg string) {
		if _, err := exec.LookPath(name); err != nil {
			errs = append(errs, fmt.Sprintf("missing binary: %s", name))
			missingPackages = append(missingPackages, pkg)
		}
	}

	checkEnv := func(key string) {
		if os.Getenv(key) == "" {
			errs = append(errs, fmt.Sprintf("missing environment variable: %s", key))
		}
	}

	// checkEnvEquals := func(key, expected string) {
	// 	val := os.Getenv(key)
	// 	if val == "" {
	// 		errs = append(errs, fmt.Sprintf("missing environment variable: %s", key))
	// 		return
	// 	}
	// 	if val != expected {
	// 		errs = append(errs, fmt.Sprintf("%s must be '%s' (got '%s')", key, expected, val))
	// 	}
	// }

	checkGstPlugin := func(plugin, pkg string) {
		cmd := exec.Command("gst-inspect-1.0", plugin)
		if err := cmd.Run(); err != nil {
			errs = append(errs, fmt.Sprintf("missing GStreamer plugin: %s", plugin))
			missingPackages = append(missingPackages, pkg)
		}
	}

	checkDBusName := func(name string) {
		conn, err := dbus.SessionBus()
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to connect to D-Bus: %v", err))
			return
		}
		defer conn.Close()

		var names []string
		err = conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names)
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to list D-Bus names: %v", err))
			return
		}

		found := false
		for _, n := range names {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("D-Bus service not available: %s", name))
		}
	}

	// ─── Checks ─────────────────────────────────────────────────────────

	// 1. Required binaries
	checkBinary("gst-launch-1.0", "gstreamer1.0-tools / gstreamer1")
	checkBinary("gst-inspect-1.0", "gstreamer1.0-tools / gstreamer1")

	// 2. GStreamer plugins
	checkGstPlugin("pipewiresrc", "gstreamer1.0-plugins-good / gst-plugins-good")
	checkGstPlugin("x264enc", "gstreamer1.0-plugins-ugly / gst-plugins-ugly")

	// 3. Environment (Wayland + D-Bus)
	checkEnv("DBUS_SESSION_BUS_ADDRESS")
	// checkEnv("WAYLAND_DISPLAY")
	// checkEnvEquals("XDG_SESSION_TYPE", "wayland")

	// 4. D-Bus portal availability
	checkDBusName("org.freedesktop.portal.Desktop")

	// ─── Result ─────────────────────────────────────────────────────────

	if len(errs) > 0 {
		distro := detectDistro()
		installCmd := generateInstallCommand(distro, missingPackages)

		return fmt.Errorf(
			"dependency check failed:\n - %s\n\nTo install on %s:\n  %s",
			strings.Join(errs, "\n - "),
			distro,
			installCmd,
		)
		// return fmt.Errorf("dependency check failed:\n - %s", strings.Join(errs, "\n - "))
	}

	return nil
}

func detectDistro() string {
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		return "Debian/Ubuntu"
	}
	if _, err := os.Stat("/etc/fedora-release"); err == nil {
		return "Fedora"
	}
	if _, err := os.Stat("/etc/arch-release"); err == nil {
		return "Arch"
	}
	return "your distro"
}

func generateInstallCommand(distro string, pkgs []string) string {
	// Map generic package names to distro-specific ones
	debianMap := map[string]string{
		"gstreamer1.0-tools / gstreamer1":              "gstreamer1.0-tools",
		"gstreamer1.0-plugins-good / gst-plugins-good": "gstreamer1.0-plugins-good",
		"gstreamer1.0-plugins-ugly / gst-plugins-ugly": "gstreamer1.0-plugins-ugly",
	}

	switch distro {
	case "Debian/Ubuntu":
		var resolved []string
		for _, p := range pkgs {
			if r, ok := debianMap[p]; ok {
				resolved = append(resolved, r)
			}
		}
		return fmt.Sprintf("sudo apt install %s", strings.Join(resolved, " "))
	case "Fedora":
		return fmt.Sprintf("sudo dnf install %s", strings.Join(pkgs, " "))
	case "Arch":
		return fmt.Sprintf("sudo pacman -S %s", strings.Join(pkgs, " "))
	default:
		return "please install: " + strings.Join(pkgs, ", ")
	}
}
