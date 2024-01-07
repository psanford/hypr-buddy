package config

import (
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
