package ups

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestHelperDispatchValidatesBeforeNativeAccessAndPreservesProtocol(t *testing.T) {
	for _, args := range [][]string{nil, {"--smoke-test"}, {"__hid"}, {"__hid", "invalid"}, {"__hid", "status", "extra"}} {
		handled, err := handleHelper(args, new(bytes.Buffer), func(bool) (Result, error) {
			t.Fatal("invalid/non-helper arguments reached native USB")
			return Result{}, nil
		})
		if len(args) > 0 && args[0] == "__hid" {
			if !handled || err == nil {
				t.Fatal("invalid helper arguments were not rejected")
			}
		} else if handled || err != nil {
			t.Fatal("ordinary desktop arguments were consumed")
		}
	}
	for _, mode := range []string{"discover", "status"} {
		var output bytes.Buffer
		handled, err := handleHelper([]string{"__hid", mode}, &output, func(status bool) (Result, error) {
			if status != (mode == "status") {
				t.Fatal("wrong native operation")
			}
			return Result{}, errors.New("fake unavailable device")
		})
		if err != nil || !handled {
			t.Fatal(err)
		}
		var result Result
		if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.Error != "fake unavailable device" {
			t.Fatal("helper failed to preserve the worker JSON error contract:", err)
		}
	}
}
