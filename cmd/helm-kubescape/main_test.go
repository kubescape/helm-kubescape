package main

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// helperCommand re-execs the test binary as a child process, so these tests need
// neither a shell nor kubescape on PATH.
func helperCommand(t *testing.T, mode string, env ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = append(os.Environ(), "GO_HELPER_MODE="+mode)
	cmd.Env = append(cmd.Env, env...)

	return cmd
}

// waitForFile blocks until path exists, so a test only signals a child that has
// finished installing its handler. Signalling before that races the default
// disposition, which would terminate the child and look exactly like a kill.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child never became ready: %s was not created", path)
}

// TestHelperProcess is not a real test: it is the child the tests above run.
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv("GO_HELPER_MODE")
	if mode == "" {
		return
	}
	switch mode {
	case "exit0":
		os.Exit(0)
	case "exit7":
		os.Exit(7)
	case "sleep":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "record":
		// Record which signal actually arrived. A killed child never gets here,
		// which is what makes this able to tell a forward from a kill.
		received := make(chan os.Signal, 1)
		signal.Notify(received, os.Interrupt, syscall.SIGTERM)
		ready(t)
		sig := <-received
		_ = os.WriteFile(os.Getenv("GO_HELPER_RECORD"), []byte(sig.String()), 0o600)
		os.Exit(0)
	case "count":
		// Ignore the first signal and only exit on the second, so a receiver that
		// stops after one forward leaves this child alive.
		//
		// The first signal is acknowledged through its own file so the test can
		// wait for it before sending the second. POSIX does not queue repeated
		// non-realtime signals, so two SIGTERMs sent back to back can arrive as
		// one and this child would wait forever.
		received := make(chan os.Signal, 2)
		signal.Notify(received, os.Interrupt, syscall.SIGTERM)
		ready(t)
		<-received
		_ = os.WriteFile(os.Getenv("GO_HELPER_FIRST"), []byte("one"), 0o600)
		<-received
		_ = os.WriteFile(os.Getenv("GO_HELPER_RECORD"), []byte("two"), 0o600)
		os.Exit(0)
	}
	os.Exit(99)
}

func ready(t *testing.T) {
	t.Helper()
	if p := os.Getenv("GO_HELPER_READY"); p != "" {
		_ = os.WriteFile(p, []byte("ready"), 0o600)
	}
}

func TestRunForwarding_PassesThroughExitCode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want int
	}{
		{name: "success", mode: "exit0", want: 0},
		{name: "non-zero exit is inherited so CI gates still work", mode: "exit7", want: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runForwarding(helperCommand(t, tt.mode), "helper", make(chan os.Signal))
			if got != tt.want {
				t.Fatalf("exit code = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRunForwarding_ReportsSignalStatus(t *testing.T) {
	tests := []struct {
		name string
		sig  syscall.Signal
		want int
	}{
		{name: "SIGINT", sig: syscall.SIGINT, want: 130},
		{name: "SIGTERM", sig: syscall.SIGTERM, want: 143},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := make(chan os.Signal, 1)
			signals <- tt.sig

			done := make(chan int, 1)
			go func() { done <- runForwarding(helperCommand(t, "sleep"), "helper", signals) }()

			select {
			case got := <-done:
				if got != tt.want {
					t.Fatalf("exit code = %d, want %d", got, tt.want)
				}
			case <-time.After(20 * time.Second):
				t.Fatal("child was not stopped: the signal never reached it")
			}
		})
	}
}

// The receiver has to keep reading for as long as the child runs. One that stopped
// after the first signal left signal.Notify registered with nobody listening, so a
// second SIGINT or SIGTERM was neither forwarded nor able to stop the wrapper.
func TestRunForwarding_KeepsReadingSignalsUntilChildExits(t *testing.T) {
	signals := make(chan os.Signal, 2)
	signals <- syscall.SIGTERM
	signals <- syscall.SIGINT

	done := make(chan int, 1)
	go func() { done <- runForwarding(helperCommand(t, "sleep"), "helper", signals) }()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("child was not stopped")
	}

	if left := len(signals); left != 0 {
		t.Fatalf("%d signal(s) never read: the receiver stopped before the child exited", left)
	}
}

func TestRunForwarding_StartFailureReportsOne(t *testing.T) {
	if got := runForwarding(exec.Command(""), "helper", make(chan os.Signal)); got != 1 {
		t.Fatalf("exit code = %d, want 1", got)
	}
}

// helperRecordPaths returns a ready-marker path and a record path under t.TempDir.
func helperRecordPaths(t *testing.T) (ready, record string) {
	t.Helper()
	dir := t.TempDir()

	return filepath.Join(dir, "ready"), filepath.Join(dir, "record")
}
