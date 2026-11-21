//go:build !windows

package ups

import "errors"

func AcquireOwner() (func(), error) {
	return nil, errors.New("native monitoring requires Windows")
}

func CheckLegacy() error {
	return errors.New("native monitoring requires Windows")
}

func NativeOperation(bool) (Result, error) {
	return Result{}, errors.New("native HID discovery requires Windows")
}

type nativeSession struct{}

func openNativeSession() (*nativeSession, error) {
	return nil, errors.New("native HID discovery requires Windows")
}

func (*nativeSession) Status() (Result, error) {
	return Result{}, errors.New("native HID discovery requires Windows")
}

func (*nativeSession) Close() error { return nil }
