//go:build !windows

package control

import (
	"errors"
	"net"
)

func Listen(Options) (net.Listener, error) {
	return nil, errors.New("desktop control transport requires Windows")
}
