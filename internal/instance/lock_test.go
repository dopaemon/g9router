package instance

import (
	"net"
	"testing"
)

func TestAcquireAllowsOnlyOneInstancePerPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	release, err := Acquire(port, port+1)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := Acquire(port); err == nil {
		t.Fatal("expected second instance to be rejected")
	}
	if _, err := Acquire(port + 1); err == nil {
		t.Fatal("expected second instance to be rejected on the second port")
	}
}
