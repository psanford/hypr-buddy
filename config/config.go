package config

import (
	"fmt"
	"os"
	"path/filepath"
)

func SocketPath() string {
	sockPath := os.Getenv("HYPRBUDDY_SOCKET")
	if sockPath != "" {
		return sockPath
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		panic(err)
	}
	dir := filepath.Join(cacheDir, "hypr-buddy")
	os.MkdirAll(dir, 0755)

	return filepath.Join(dir, "hypr-buddy.control.sock")
}

func HyprRuntimeDir() string {
	sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if sig == "" {
		panic(fmt.Sprintf("HYPRLAND_INSTANCE_SIGNATURE not set"))
	}

	xdgDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgDir == "" {
		xdgDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}

	return fmt.Sprintf("%s/hypr/%s/", xdgDir, sig)
}
