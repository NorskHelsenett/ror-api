package aclstore

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAvailabilityChecks(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "available", true: "unavailable"}[unavailable], func(t *testing.T) {
			var checks availabilityChecks
			var calls atomic.Int32
			var expected error
			if unavailable {
				expected = errors.New("database unavailable")
			}
			probe := func() error {
				calls.Add(1)
				return expected
			}
			var wg sync.WaitGroup
			for range 20 {
				wg.Go(func() {
					if err := checks.check("database-a", probe); !errors.Is(err, expected) {
						t.Errorf("got %v, want %v", err, expected)
					}
				})
			}
			wg.Wait()
			if got := calls.Load(); got != 1 {
				t.Fatalf("probed %d times, want 1", got)
			}
			if err := checks.check("database-b", probe); !errors.Is(err, expected) {
				t.Fatal(err)
			}
			if got := calls.Load(); got != 2 {
				t.Fatalf("different database did not get its own probe: %d", got)
			}
		})
	}
}
