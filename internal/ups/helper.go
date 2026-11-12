package ups

import (
	"encoding/json"
	"errors"
	"io"
)

func HandleHelper(args []string, output io.Writer) (bool, error) {
	return handleHelper(args, output, NativeOperation)
}

func handleHelper(args []string, output io.Writer, operation func(bool) (Result, error)) (bool, error) {
	if len(args) == 0 || args[0] != "__hid" {
		return false, nil
	}
	if len(args) != 2 || (args[1] != "discover" && args[1] != "status") {
		return true, errors.New("unsupported helper operation")
	}
	result, err := operation(args[1] == "status")
	if err != nil {
		result.Error = err.Error()
	}
	return true, json.NewEncoder(output).Encode(result)
}
