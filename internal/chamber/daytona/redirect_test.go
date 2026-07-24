package daytona

import (
	"slices"
	"testing"

	"github.com/reactor-sec/reactor/internal/chamber"
)

// A shell redirect into a missing directory does not fail like a bad command
// does: the shell never starts the process, so there is no exit status, no
// output, and — because the redirect target is the file that would have
// explained it — no log either. Backgrounded, that is completely silent, and it
// is how the egress sink appeared to start and then not be listening. The
// driver renders those redirects, so the driver makes the directories.
func TestRedirectDirsCoversEveryFileTheCommandWillOpen(t *testing.T) {
	c := &Chamber{home: "/home/daytona"}

	got := c.redirectDirs(chamber.ExecOpts{
		StdoutPath: "logs/sink.out",
		StderrPath: "logs/sink.err",
	})
	if !slices.Equal(got, []string{"/home/daytona/logs"}) {
		t.Fatalf("redirect dirs = %v, want the one logs dir (deduplicated)", got)
	}

	// A traced command opens a third file, and it is just as fatal.
	got = c.redirectDirs(chamber.ExecOpts{
		StdoutPath: "run/out.txt",
		Trace:      true,
		TracePath:  "traces/strace.1.log",
	})
	want := []string{"/home/daytona/run", "/home/daytona/traces"}
	if !slices.Equal(got, want) {
		t.Fatalf("redirect dirs = %v, want %v", got, want)
	}

	// Trace with no path named still writes somewhere, and that somewhere has
	// to be the same place buildCmd will send it.
	got = c.redirectDirs(chamber.ExecOpts{Trace: true})
	if !slices.Equal(got, []string{"/home/daytona/logs"}) {
		t.Fatalf("default trace path dir = %v, want /home/daytona/logs", got)
	}

	// Nothing to redirect: no round trip to the sandbox at all.
	if got := c.redirectDirs(chamber.ExecOpts{Cmd: []string{"true"}}); len(got) != 0 {
		t.Fatalf("redirect dirs = %v, want none", got)
	}
}
