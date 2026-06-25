package pkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"time"
)

func DebugJson(value interface{}) {
	fmt.Println(reflect.TypeOf(value).String())
	prettyJSON, _ := json.MarshalIndent(value, "", "    ")
	fmt.Printf("%s\n", string(prettyJSON))
}

func RetryWithBackoff(
	ctx context.Context,
	maxAttempts int,
	baseDelay, maxDelay time.Duration,
	isTransient func(error) bool,
	fn func() error) error {
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if !isTransient(err) {
			return err
		}
		if attempt == maxAttempts-1 {
			return err
		}
		delay := baseDelay * (1 << attempt)
		if delay > maxDelay {
			delay = maxDelay
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

func IsTransientNetwork(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "connection reset") ||
		strings.Contains(s, "EOF") ||
		strings.Contains(s, "broken pipe")
}
