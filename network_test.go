//go:build linux

package mirage

import (
	"net"
	"testing"
)

// stubOps is a minimal in-package stub for unit testing NetworkIsolator logic.
type stubOps struct {
	namespaces map[string]bool
	veths      map[string]string
	configured int
}

func newStubOps() *stubOps {
	return &stubOps{namespaces: map[string]bool{}, veths: map[string]string{}}
}

func (s *stubOps) CreateNamespace(name string) error   { s.namespaces[name] = true; return nil }
func (s *stubOps) DeleteNamespace(name string) error    { delete(s.namespaces, name); return nil }
func (s *stubOps) CreateVethPair(h, c string, _ int) error { s.veths[h] = c; return nil }
func (s *stubOps) DeleteLink(name string) error         { delete(s.veths, name); return nil }
func (s *stubOps) MoveVethToNs(_, _ string) error       { return nil }
func (s *stubOps) EnsureBridge(_ string, _ net.IP, _ *net.IPNet, _ int) error { return nil }
func (s *stubOps) AttachToBridge(_, _ string) error     { return nil }
func (s *stubOps) BringUp(_ string) error               { return nil }
func (s *stubOps) ConfigureContainerNs(_ string, _ string, _ net.IP, _ net.IP, _ int, _ NetworkPolicy) error {
	s.configured++
	return nil
}

func TestNetworkIsolator_IPAllocation(t *testing.T) {
	t.Parallel()
	ops := newStubOps()
	ni := NewNetworkIsolatorWithOps("10.88.0.0/24", "test0", 1500, ops)

	// First space gets .2, second gets .3, third gets .4.
	for i, name := range []string{"agent-a", "agent-b", "agent-c"} {
		_, err := ni.Setup(name, NetworkPolicy{Mode: NetIsolated})
		if err != nil {
			t.Fatal(err)
		}
		wantOctet := byte(i + 2) // .2, .3, .4
		got := ni.netns[name].ip[3]
		if got != wantOctet {
			t.Errorf("%s IP last octet = %d, want %d", name, got, wantOctet)
		}
	}
}

func TestNetworkIsolator_NsNaming(t *testing.T) {
	t.Parallel()
	ops := newStubOps()
	ni := NewNetworkIsolatorWithOps("10.88.0.0/24", "test0", 1500, ops)

	nsName, err := ni.Setup("my-agent", NetworkPolicy{Mode: NetIsolated})
	if err != nil {
		t.Fatal(err)
	}
	if nsName != "mirage-my-agent" {
		t.Errorf("nsName = %q, want mirage-my-agent", nsName)
	}
}

func TestNetworkIsolator_VethNaming(t *testing.T) {
	t.Parallel()
	ops := newStubOps()
	ni := NewNetworkIsolatorWithOps("10.88.0.0/24", "test0", 1500, ops)

	ni.Setup("short", NetworkPolicy{Mode: NetIsolated}) //nolint:errcheck

	iso := ni.netns["short"]
	if iso.vethHost != "vh-short" {
		t.Errorf("vethHost = %q, want vh-short", iso.vethHost)
	}
	if iso.vethCont != "vc-short" {
		t.Errorf("vethCont = %q, want vc-short", iso.vethCont)
	}
}

func TestNetworkIsolator_VethTruncation(t *testing.T) {
	t.Parallel()
	ops := newStubOps()
	ni := NewNetworkIsolatorWithOps("10.88.0.0/24", "test0", 1500, ops)

	ni.Setup("very-long-agent-name-here", NetworkPolicy{Mode: NetIsolated}) //nolint:errcheck

	iso := ni.netns["very-long-agent-name-here"]
	if len(iso.vethHost) > 15 {
		t.Errorf("vethHost len = %d, want <= 15: %q", len(iso.vethHost), iso.vethHost)
	}
	if len(iso.vethCont) > 15 {
		t.Errorf("vethCont len = %d, want <= 15: %q", len(iso.vethCont), iso.vethCont)
	}
}

func TestNetworkIsolator_DuplicateSetup(t *testing.T) {
	t.Parallel()
	ops := newStubOps()
	ni := NewNetworkIsolatorWithOps("10.88.0.0/24", "test0", 1500, ops)

	_, err := ni.Setup("dup", NetworkPolicy{Mode: NetIsolated})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ni.Setup("dup", NetworkPolicy{Mode: NetIsolated})
	if err == nil {
		t.Fatal("expected error on duplicate setup")
	}
}

func TestNetworkIsolator_Teardown(t *testing.T) {
	t.Parallel()
	ops := newStubOps()
	ni := NewNetworkIsolatorWithOps("10.88.0.0/24", "test0", 1500, ops)

	ni.Setup("agent", NetworkPolicy{Mode: NetIsolated}) //nolint:errcheck

	if ni.NsName("agent") == "" {
		t.Fatal("expected NsName to return namespace name")
	}

	if err := ni.Teardown("agent"); err != nil {
		t.Fatal(err)
	}
	if ni.NsName("agent") != "" {
		t.Fatal("expected NsName to return empty after teardown")
	}

	// Teardown unknown should error.
	if err := ni.Teardown("nonexistent"); err == nil {
		t.Fatal("expected error on unknown teardown")
	}

	// Namespace should be deleted from ops.
	if ops.namespaces["mirage-agent"] {
		t.Error("namespace should be deleted from ops")
	}
}

func TestNetworkIsolator_TeardownAll(t *testing.T) {
	t.Parallel()
	ops := newStubOps()
	ni := NewNetworkIsolatorWithOps("10.88.0.0/24", "test0", 1500, ops)

	for _, name := range []string{"a", "b", "c"} {
		ni.Setup(name, NetworkPolicy{Mode: NetIsolated}) //nolint:errcheck
	}

	ni.TeardownAll()

	for _, name := range []string{"a", "b", "c"} {
		if ni.NsName(name) != "" {
			t.Errorf("%s should be gone after TeardownAll", name)
		}
	}
	if len(ops.namespaces) != 0 {
		t.Errorf("expected 0 namespaces, got %d", len(ops.namespaces))
	}
	if len(ops.veths) != 0 {
		t.Errorf("expected 0 veths, got %d", len(ops.veths))
	}
}

func TestNetworkIsolator_InvalidSubnet(t *testing.T) {
	t.Parallel()
	ops := newStubOps()
	ni := NewNetworkIsolatorWithOps("not-a-cidr", "test0", 1500, ops)

	_, err := ni.Setup("agent", NetworkPolicy{Mode: NetIsolated})
	if err == nil {
		t.Fatal("expected error on invalid subnet")
	}
}

func TestNetworkIsolator_DefaultBridgeAndMTU(t *testing.T) {
	t.Parallel()
	ops := newStubOps()
	ni := NewNetworkIsolatorWithOps("10.88.0.0/24", "", 0, ops)

	if ni.bridge != "mirage0" {
		t.Errorf("bridge = %q, want mirage0", ni.bridge)
	}
	if ni.mtu != 1500 {
		t.Errorf("mtu = %d, want 1500", ni.mtu)
	}
}

func TestBuildIptablesRules_EgressList(t *testing.T) {
	t.Parallel()
	gw := net.ParseIP("10.88.0.1")

	rules := BuildIptablesRules(NetworkPolicy{
		Mode:        NetIsolated,
		AllowEgress: []string{"1.2.3.4:443", "5.6.7.8:80"},
		DNS:         []string{"8.8.8.8"},
	}, gw)

	// Should have: loopback + 2 egress + established + 2 DNS(custom) + 2 DNS(gateway) + DROP = 9
	if len(rules) != 9 {
		t.Fatalf("expected 9 rules, got %d: %v", len(rules), rules)
	}

	// First rule: loopback ACCEPT.
	if rules[0][5] != "ACCEPT" || rules[0][3] != "lo" {
		t.Errorf("first rule should be loopback ACCEPT: %v", rules[0])
	}

	// Egress rules for 1.2.3.4:443 and 5.6.7.8:80.
	if rules[1][3] != "1.2.3.4" || rules[1][7] != "443" {
		t.Errorf("rule[1] should be 1.2.3.4:443: %v", rules[1])
	}
	if rules[2][3] != "5.6.7.8" || rules[2][7] != "80" {
		t.Errorf("rule[2] should be 5.6.7.8:80: %v", rules[2])
	}

	// Last rule: DROP.
	last := rules[len(rules)-1]
	if last[3] != "DROP" {
		t.Errorf("last rule should be DROP: %v", last)
	}
}

func TestBuildIptablesRules_NoEgress(t *testing.T) {
	t.Parallel()
	gw := net.ParseIP("10.88.0.1")

	rules := BuildIptablesRules(NetworkPolicy{Mode: NetIsolated}, gw)

	// loopback + established + 2 DNS(gateway) + DROP = 5
	if len(rules) != 5 {
		t.Fatalf("expected 5 rules, got %d: %v", len(rules), rules)
	}
}

func TestBuildIptablesRules_InvalidTarget(t *testing.T) {
	t.Parallel()
	gw := net.ParseIP("10.88.0.1")

	rules := BuildIptablesRules(NetworkPolicy{
		Mode:        NetIsolated,
		AllowEgress: []string{"invalid-no-port"},
	}, gw)

	// Invalid target should be skipped: loopback + established + 2 DNS(gateway) + DROP = 5
	if len(rules) != 5 {
		t.Fatalf("expected 5 rules (invalid skipped), got %d: %v", len(rules), rules)
	}
}
