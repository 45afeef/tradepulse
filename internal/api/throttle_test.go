package api

import (
	"testing"
	"time"
)

func TestThrottleLocksAfterMaxFailuresAndUnlocksLater(t *testing.T) {
	now := time.Now()
	th := newThrottle(3, time.Minute, 10*time.Minute)
	th.now = func() time.Time { return now }
	if th.fail("a") || th.fail("a") {
		t.Fatal("must not lock before the threshold")
	}
	if th.blocked("a") != 0 {
		t.Fatal("not locked yet")
	}
	if !th.fail("a") || th.blocked("a") <= 0 {
		t.Fatal("third failure must lock")
	}
	now = now.Add(5 * time.Minute)
	if th.blocked("a") <= 0 {
		t.Fatal("still locked mid-way")
	}
	now = now.Add(6 * time.Minute)
	if th.blocked("a") != 0 {
		t.Fatal("lock must expire")
	}
	if th.fail("a") || th.fail("a") {
		t.Fatal("counting must restart after the lock")
	}
}

func TestThrottleKeysWindowAndReset(t *testing.T) {
	now := time.Now()
	th := newThrottle(3, time.Minute, time.Minute)
	th.now = func() time.Time { return now }
	th.fail("a")
	th.fail("a")
	th.fail("b")
	if th.blocked("b") != 0 {
		t.Fatal("keys are independent")
	}
	now = now.Add(2 * time.Minute) // window passed: old failures are forgotten
	if th.fail("a") || th.blocked("a") != 0 {
		t.Fatal("failures outside the window must not accumulate")
	}
	th.fail("c")
	th.fail("c")
	th.reset("c")
	if th.fail("c") || th.fail("c") {
		t.Fatal("reset must clear the counter")
	}
}
