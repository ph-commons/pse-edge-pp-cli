// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package main

import "testing"

func TestIsLoopbackAddr(t *testing.T) {
	yes := []string{"127.0.0.1:7777", "localhost:7777", "[::1]:7777", "::1:7777"}
	no := []string{":7777", "0.0.0.0:7777", "192.168.1.1:7777", "example.com:7777"}
	for _, a := range yes {
		if !isLoopbackAddr(a) {
			t.Errorf("isLoopbackAddr(%q) = false, want true", a)
		}
	}
	for _, a := range no {
		if isLoopbackAddr(a) {
			t.Errorf("isLoopbackAddr(%q) = true, want false", a)
		}
	}
}
