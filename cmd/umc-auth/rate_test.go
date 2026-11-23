package main

import (
	"testing"
	"time"
)

func TestSlidingWindow(t *testing.T) {
	now := time.Now()
	window := newSlidingWindow(2, time.Hour)
	if !window.Check("a", now) {
		t.Fatal("first request denied")
	}
	window.Record("a", now)
	window.Record("a", now)
	if window.Check("a", now) {
		t.Fatal("limit was not enforced")
	}
	if !window.Check("b", now) {
		t.Fatal("unrelated key was limited")
	}
	if !window.Check("a", now.Add(time.Hour+time.Second)) {
		t.Fatal("window did not expire")
	}
}

func TestRateKeyAggregatesIPv6Subnets(t *testing.T) {
	first := rateKey("2001:db8:1234:5678::1")
	second := rateKey("2001:db8:1234:5678::2")
	if first != second {
		t.Fatalf("IPv6 addresses in one /64 were not aggregated: %q != %q", first, second)
	}
	if rateKey("192.0.2.1") == rateKey("192.0.2.2") {
		t.Fatal("IPv4 addresses must remain separate")
	}
}
