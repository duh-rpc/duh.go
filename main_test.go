package duh_test

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs the whole package under goleak: after all tests finish, any
// goroutine still running fails the suite. This is the suite-wide guard against
// leaks like HandleStream's heartbeat goroutine (ENG-107) — it catches a leak
// from any test that exercises a leaky path, not just the streaming tests below.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
