//go:build !windows

package service

import "errors"

func Run(Application) error {
	return errors.New("Windows services require Windows")
}

func Manage(string, string) error {
	return errors.New("Windows services require Windows")
}
