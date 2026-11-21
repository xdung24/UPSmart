package ups

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
)

func HandleHelper(args []string, output io.Writer) (bool, error) {
	if len(args) == 2 && args[0] == "__hid" && args[1] == "session" {
		return true, handleSession(os.Stdin, output)
	}
	return handleHelper(args, output, NativeOperation)
}

func handleSession(input io.Reader, output io.Writer) error {
	session, err := openNativeSession()
	if err != nil {
		return err
	}
	defer session.Close()
	scanner := bufio.NewScanner(input)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		if scanner.Text() != "status" {
			return errors.New("unsupported HID session request")
		}
		result, requestErr := session.Status()
		if requestErr != nil {
			result.Error = requestErr.Error()
		}
		if err := encoder.Encode(result); err != nil {
			return err
		}
	}
	return scanner.Err()
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
