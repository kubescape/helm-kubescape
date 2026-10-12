//go:build !windows

package main

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// TestRunForwarding_ChildReceivesTheSignalNotAKill is the test that tells a
// forward from a kill. The child installs a handler, records which signal arrived
// and exits on its own; a killed child never records anything, so replacing
// Process.Signal with Process.Kill fails this.
//
// Unix only: Windows has no signal delivery, so there is no forward to prove there.
func TestRunForwarding_ChildReceivesTheSignalNotAKill(t *testing.T) {
	tests := []struct {
		name string
		sig  syscall.Signal
		want string
	}{
		{name: "SIGTERM", sig: syscall.SIGTERM, want: "terminated"},
		{name: "SIGINT", sig: syscall.SIGINT, want: "interrupt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readyPath, recordPath := helperRecordPaths(t)
			cmd := helperCommand(t, "record",
				"GO_HELPER_READY="+readyPath, "GO_HELPER_RECORD="+recordPath)

			signals := make(chan os.Signal, 1)
			done := make(chan int, 1)
			go func() { done <- runForwarding(cmd, "helper", signals) }()

			waitForFile(t, readyPath)
			signals <- tt.sig

			select {
			case <-done:
			case <-time.After(20 * time.Second):
				t.Fatal("child never exited")
			}

			got, err := os.ReadFile(recordPath)
			if err != nil {
				t.Fatalf("child recorded nothing, so it was killed rather than signalled: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("child received %q, want %q", got, tt.want)
			}
		})
	}
}

// A child that ignores the first signal must still receive the second. Before the
// receiver looped, the second was swallowed and this child stayed alive.
func TestRunForwarding_ForwardsRepeatedSignals(t *testing.T) {
	readyPath, recordPath := helperRecordPaths(t)
	firstPath := recordPath + ".first"
	cmd := helperCommand(t, "count",
		"GO_HELPER_READY="+readyPath,
		"GO_HELPER_FIRST="+firstPath,
		"GO_HELPER_RECORD="+recordPath)

	signals := make(chan os.Signal, 2)
	done := make(chan int, 1)
	go func() { done <- runForwarding(cmd, "helper", signals) }()

	// Each signal is sent only once the child has acknowledged the previous one.
	// Repeated non-realtime signals are not queued, so sending both at once can
	// deliver a single SIGTERM and the child would never reach its second wait.
	waitForFile(t, readyPath)
	signals <- syscall.SIGTERM
	waitForFile(t, firstPath)
	signals <- syscall.SIGTERM

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("child never exited: the second signal was not forwarded")
	}

	if _, err := os.ReadFile(recordPath); err != nil {
		t.Fatalf("child did not record two signals: %v", err)
	}
}
