//go:build !windows

package enroll

import "runtime"

func osVersion() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
