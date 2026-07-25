package server

import "testing"

func TestConsistentMachineIDIsStable(t *testing.T) {
	first := consistentMachineID()
	if len(first) != 16 {
		t.Fatalf("machine id length=%d", len(first))
	}
	if first != consistentMachineID() {
		t.Fatal("machine id is not stable")
	}
}
