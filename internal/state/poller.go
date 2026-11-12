package state

import (
	"context"
	"log"
	"time"

	"upsmart/internal/ups"
)

type Reader func(context.Context) (ups.Result, error)

func Poll(ctx context.Context, store *Store, read Reader, interval, timeout time.Duration) {
	backoff := time.Second
	lastError := ""
	var lastLogged time.Time
	for {
		if ctx.Err() != nil {
			return
		}
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		result, err := read(callCtx)
		cancel()
		if ctx.Err() != nil {
			return
		}
		now := time.Now()
		err = store.Apply(now, result, err)
		wait := interval
		if err != nil {
			if err.Error() != lastError || now.Sub(lastLogged) >= 30*time.Second {
				log.Printf("UPS read failed: %v", err)
				lastLogged = now
			}
			lastError = err.Error()
			wait = backoff
			backoff = min(backoff*2, 30*time.Second)
		} else {
			if lastError != "" {
				log.Print("UPS communication recovered")
			}
			lastError = ""
			backoff = time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
