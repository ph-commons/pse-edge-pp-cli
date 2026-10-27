// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestServerValidatesToolInputSchemas(t *testing.T) {
	s := newServer()
	called := false
	s.AddTool(mcp.NewTool("validation_probe", mcp.WithBoolean("enabled")), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("called"), nil
	})
	s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"validation_probe","arguments":{"enabled":"not-a-boolean"}}}`))
	if called {
		t.Fatal("handler ran with an input that violates its schema")
	}
}

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
