package main

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// helperCommand re-execs the test binary as a child process, so these tests do not
// depend on a shell or on kubescape being installed.
func helperCommand(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = append(os.Environ(), "GO_HELPER_MODE="+mode)

	return cmd
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
		// long enough that the test drives the outcome, short enough not to wedge
		// CI if something goes wrong
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	os.Exit(99)
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

// A signal the plugin receives has to reach the child and be reported as
// 128+signum, the shell convention. Before this, exec.CommandContext's default
// Cancel killed the child instead, which both skipped kubescape's own shutdown and
// turned the status into -1, surfacing as 255.
func TestRunForwarding_ForwardsSignalAndReportsSignalStatus(t *testing.T) {
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

func TestRunForwarding_StartFailureReportsOne(t *testing.T) {
	cmd := exec.Command("")
	if got := runForwarding(cmd, "helper", make(chan os.Signal)); got != 1 {
		t.Fatalf("exit code = %d, want 1", got)
	}
}
