package notifications

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"upsmart/internal/notifications/relaypb"
)

func validateRelayAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	number, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || number < 1 || number > 65535 ||
		host == "" || len(address) > 253 ||
		strings.ContainsAny(host, `/\?#@%`) || strings.ContainsFunc(host, unicode.IsSpace) {
		return errors.New("notifications.relay_address must be an explicit host:port without a URL scheme or path")
	}
	return nil
}

func openRelay(address string) (*grpc.ClientConn, error) {
	return grpc.NewClient("passthrough:///"+address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithNoProxy(),
		grpc.WithDisableRetry(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxBodyBytes), grpc.MaxCallSendMsgSize(maxBodyBytes)),
	)
}

func requestRelay(ctx context.Context, conn *grpc.ClientConn, text string) (accepted, permanent, retry bool, delay time.Duration, reason string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var response relaypb.NotifyResponse
	err := conn.Invoke(ctx, "/notify.Notifier/SendMessage",
		&relaypb.NotifyRequest{Message: relayMarkdown(text)}, &response)
	if err != nil {
		switch status.Code(err) {
		case codes.Unavailable, codes.DeadlineExceeded, codes.Aborted, codes.Internal, codes.Unknown:
			return false, false, true, 0, "Notification relay request failed; acceptance is unknown"
		case codes.Canceled:
			return false, false, false, 0, "Notification relay request cancelled; acceptance is unknown"
		case codes.ResourceExhausted:
			return false, false, false, 0, "Notification relay limit exceeded; event dropped"
		default:
			return false, true, false, 0, "Notification relay rejected the request; check relay setup"
		}
	}
	if !response.Sent || response.Error != "" {
		// The relay's raw error can include the Telegram credential-bearing URL.
		return false, false, false, 0, "Notification relay did not confirm sending; check rdpwatcher"
	}
	return true, false, false, 0, ""
}

// rdpwatcher always sends with legacy Telegram Markdown, unlike direct mode.
func relayMarkdown(text string) string {
	var out strings.Builder
	count := 0
	for _, char := range text {
		escape := strings.ContainsRune("\\_*`[", char)
		width := 1
		if escape {
			width++
		}
		if count+width > maxTextRunes {
			break
		}
		if escape {
			out.WriteByte('\\')
		}
		out.WriteRune(char)
		count += width
	}
	return out.String()
}
