package model

import (
	"slices"
	"testing"
)

// TEC-409: the hub's unauthenticated /mcp/olex endpoint is not carried over;
// the app serves only the three OAuth-protected MCP endpoints.
func TestMCPResourcesHaveNoLegacyEndpoint(t *testing.T) {
	want := []string{"/mcp/dealer", "/mcp/customer", "/mcp/user"}
	if !slices.Equal(Resources, want) {
		t.Fatalf("Resources = %v, want %v", Resources, want)
	}
	if IsResource("/mcp/olex") {
		t.Fatal("/mcp/olex must not be an MCP endpoint")
	}
}
