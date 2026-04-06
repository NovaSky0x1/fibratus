//go:build !windows

package bootstrap

import log "github.com/sirupsen/logrus"

func restartService() {
	log.Warn("fleet: service restart not supported on this platform")
}
