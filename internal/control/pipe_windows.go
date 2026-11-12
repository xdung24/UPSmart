//go:build windows

package control

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func CurrentUserSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func Listen(options Options) (net.Listener, error) {
	return listen(PipeName, options)
}

func listen(path string, options Options) (net.Listener, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	backend, err := CurrentUserSID()
	if err != nil {
		return nil, err
	}
	user := options.UserSID
	if user == "" {
		if backend == "S-1-5-19" || backend == "S-1-5-20" || backend == "S-1-5-18" {
			return nil, errors.New("service desktop IPC requires an explicit desktop.user_sid")
		}
		user = backend
	}
	if _, err := windows.StringToSid(user); err != nil {
		return nil, errors.New("desktop.user_sid is not a valid Windows SID")
	}
	// Omit FILE_CREATE_PIPE_INSTANCE from desktop rights. go-winio rejects
	// remote clients and exclusively creates the first pipe instance.
	sddl := fmt.Sprintf("D:P(A;;GA;;;SY)(A;;GA;;;%s)(A;;0x12019b;;;%s)", backend, user)
	return winio.ListenPipe(path, &winio.PipeConfig{
		SecurityDescriptor: sddl, InputBufferSize: 64 << 10, OutputBufferSize: 64 << 10,
	})
}

func NewClient() *Client {
	return newClient(PipeName)
}

func newClient(path string) *Client {
	return &Client{http: &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return winio.DialPipeAccess(ctx, path, 0x12019b)
			},
			MaxConnsPerHost: 2, MaxIdleConnsPerHost: 2, IdleConnTimeout: 10 * time.Second,
		},
	}}
}
