//go:build windows

package hardware

import "testing"

func TestProviderOutputBound(t *testing.T) {
	buffer := &limitedBuffer{limit: 5}
	if _, err := buffer.Write([]byte("123")); err != nil {
		t.Fatal(err)
	}
	n, err := buffer.Write([]byte("456789"))
	if err == nil || n != 2 || !buffer.overflow || string(buffer.data) != "12345" {
		t.Fatalf("output must be bounded and rejected: %+v %v", buffer, err)
	}
	if n, err := buffer.Write([]byte("more")); err == nil || n != 0 || len(buffer.data) != 5 {
		t.Fatal("overflow write expanded buffer")
	}
}
