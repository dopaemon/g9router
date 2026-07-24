package headroom

import "testing"

func TestManagerInitiallyStopped(t *testing.T) {
	if pid := New("headroom").PID(); pid != 0 {
		t.Fatal(pid)
	}
}
