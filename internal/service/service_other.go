//go:build !windows

package service

import (
	"errors"

	"upsmart/internal/config"
)

func Run(string, Application) error {
	return errors.New("Windows services require Windows")
}

func Manage(string, string, config.Config) error {
	return errors.New("Windows services require Windows")
}

func LogError(err error) error { return err }
