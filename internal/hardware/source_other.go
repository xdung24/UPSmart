//go:build !windows

package hardware

import (
	"context"
	"errors"
)

func platformSources() sources {
	return sources{
		cpu: func(context.Context) (cpuTimes, error) {
			return cpuTimes{}, errors.New("PC hardware monitoring is supported only on Windows")
		},
		query: func(context.Context) ([]byte, error) {
			return nil, errors.New("PC hardware monitoring is supported only on Windows")
		},
	}
}
