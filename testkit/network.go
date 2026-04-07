package testkit

import "github.com/dpopsuev/mirage"

// NewNetworkIsolatorWithStub creates a NetworkIsolator using the StubNetOps.
// The returned stub can be inspected to verify operations without root.
func NewNetworkIsolatorWithStub(subnet, bridge string, mtu int) (*mirage.NetworkIsolator, *StubNetOps) {
	stub := NewStubNetOps()
	ni := mirage.NewNetworkIsolatorWithOps(subnet, bridge, mtu, stub)
	return ni, stub
}
