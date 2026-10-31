// Package buildinfo holds values stamped at link time.
package buildinfo

import (
	"fmt"
	"os"
)

// Version is set with -ldflags "-X base-admin/server/internal/platform/buildinfo.Version=v1.2.3".
var Version = "dev"

// InstanceID identifies this process among replicas: hostname plus PID.
func InstanceID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}
