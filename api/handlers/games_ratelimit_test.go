package handlers

import (
	"testing"
	"time"
)

func TestKeyedLimiterAllowAndBlock(t *testing.T) {
	l := newKeyedLimiter(3, time.Hour)
	for i := 0; i < 3; i++ {
		if !l.Allow("alice") {
			t.Fatalf("hit %d: expected allow, got block", i+1)
		}
	}
	if l.Allow("alice") {
		t.Fatalf("hit 4 for alice: expected block, got allow")
	}
	// Other keys must have independent quota.
	if !l.Allow("bob") {
		t.Fatalf("bob should not be affected by alice's quota")
	}
}

func TestKeyedLimiterWindowReset(t *testing.T) {
	l := newKeyedLimiter(1, 10*time.Millisecond)
	if !l.Allow("k") {
		t.Fatalf("first allow failed")
	}
	if l.Allow("k") {
		t.Fatalf("second within window should block")
	}
	time.Sleep(20 * time.Millisecond)
	if !l.Allow("k") {
		t.Fatalf("after window reset should allow")
	}
}
