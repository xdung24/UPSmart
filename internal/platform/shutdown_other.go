//go:build !windows

package platform

import (
	"context"
	"errors"
)

func Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("Windows shutdown is not supported on this platform")
}
