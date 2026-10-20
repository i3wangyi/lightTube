package main

import (
	"os"
	"runtime/debug"
	"testing"
)

func TestMemoryPolicyHonorsEnvironment(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	original := debug.SetMemoryLimit(-1)
	defer debug.SetMemoryLimit(original)
	for _, variable := range []string{"GOGC", "GOMEMLIMIT"} {
		t.Run(variable, func(t *testing.T) {
			for _, name := range []string{"GOGC", "GOMEMLIMIT"} {
				t.Setenv(name, "")
				os.Unsetenv(name)
			}
			t.Setenv(variable, "")
			const custom = 128 << 20
			debug.SetMemoryLimit(custom)
			if err := run([]string{"--version"}); err != nil {
				t.Fatal(err)
			}
			if got := debug.SetMemoryLimit(-1); got != custom {
				t.Fatalf("%s was overridden: limit %d", variable, got)
			}
		})
	}
}
