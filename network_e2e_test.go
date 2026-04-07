//go:build e2e && netns

package mirage

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// requireRoot skips if not running as root.
func requireRoot(t *testing.T) {
	t.Helper()
	if os.Getuid() != 0 {
		t.Skip("requires root (run with: sudo go test -tags='e2e netns')")
	}
}

// requireIptables skips if iptables is not available.
func requireIptables(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("iptables"); err != nil {
		t.Skip("iptables not available")
	}
}

// cleanupBridge removes a bridge by name (best-effort, for test cleanup).
func cleanupBridge(t *testing.T, name string) {
	t.Helper()
	if br, err := netlink.LinkByName(name); err == nil {
		_ = netlink.LinkDel(br)
	}
}

// cleanupStaleNs removes a namespace if it exists from a previous test run.
func cleanupStaleNs(names ...string) {
	for _, name := range names {
		_ = netns.DeleteNamed(name)
	}
}

func TestNetwork_E2E_CreateAndTeardown(t *testing.T) {
	requireRoot(t)
	cleanupStaleNs("mirage-e2e-test")
	bridge := "miraget1"
	t.Cleanup(func() { cleanupBridge(t, bridge) })

	ni := NewNetworkIsolator("10.99.0.0/24", bridge, 1500)

	nsName, err := ni.Setup("e2e-test", NetworkPolicy{
		Mode:        NetIsolated,
		AllowEgress: []string{"93.184.216.34:443"},
		DNS:         []string{"10.99.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if nsName != "mirage-e2e-test" {
		t.Errorf("nsName = %q, want mirage-e2e-test", nsName)
	}

	// Verify namespace exists.
	out, err := exec.Command("ip", "netns", "list").CombinedOutput()
	if err != nil {
		t.Fatalf("ip netns list: %s: %v", out, err)
	}
	if !strings.Contains(string(out), "mirage-e2e-test") {
		t.Errorf("namespace not found in ip netns list: %s", out)
	}

	// Teardown.
	if err := ni.Teardown("e2e-test"); err != nil {
		t.Fatal(err)
	}

	// Verify namespace is gone.
	out, _ = exec.Command("ip", "netns", "list").CombinedOutput()
	if strings.Contains(string(out), "mirage-e2e-test") {
		t.Errorf("namespace should be gone after teardown: %s", out)
	}
}

func TestNetwork_E2E_IptablesRules(t *testing.T) {
	requireRoot(t)
	requireIptables(t)
	cleanupStaleNs("mirage-iptables-test")
	bridge := "miraget2"
	t.Cleanup(func() { cleanupBridge(t, bridge) })

	ni := NewNetworkIsolator("10.98.0.0/24", bridge, 1500)

	_, err := ni.Setup("iptables-test", NetworkPolicy{
		Mode:        NetIsolated,
		AllowEgress: []string{"1.2.3.4:443", "5.6.7.8:80"},
		DNS:         []string{"10.98.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ni.Teardown("iptables-test") //nolint:errcheck

	// Check iptables rules inside the namespace.
	out, err := exec.Command("ip", "netns", "exec", "mirage-iptables-test",
		"iptables", "-L", "OUTPUT", "-n").CombinedOutput()
	if err != nil {
		t.Fatalf("iptables -L: %s: %v", out, err)
	}

	rules := string(out)
	if !strings.Contains(rules, "1.2.3.4") {
		t.Errorf("expected iptables rule for 1.2.3.4: %s", rules)
	}
	if !strings.Contains(rules, "5.6.7.8") {
		t.Errorf("expected iptables rule for 5.6.7.8: %s", rules)
	}
	if !strings.Contains(rules, "DROP") {
		t.Errorf("expected DROP rule: %s", rules)
	}
}

func TestNetwork_E2E_TeardownAll(t *testing.T) {
	requireRoot(t)
	cleanupStaleNs("mirage-bulk-a", "mirage-bulk-b", "mirage-bulk-c")
	bridge := "miraget3"
	t.Cleanup(func() { cleanupBridge(t, bridge) })

	ni := NewNetworkIsolator("10.97.0.0/24", bridge, 1500)

	for _, name := range []string{"bulk-a", "bulk-b", "bulk-c"} {
		_, err := ni.Setup(name, NetworkPolicy{
			Mode: NetIsolated,
			DNS:  []string{"10.97.0.1"},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	ni.TeardownAll()

	out, _ := exec.Command("ip", "netns", "list").CombinedOutput()
	for _, name := range []string{"mirage-bulk-a", "mirage-bulk-b", "mirage-bulk-c"} {
		if strings.Contains(string(out), name) {
			t.Errorf("namespace %s should be gone after TeardownAll: %s", name, out)
		}
	}
}

func TestNetwork_E2E_DuplicateSetup(t *testing.T) {
	requireRoot(t)
	cleanupStaleNs("mirage-dup-test")
	bridge := "miraget4"
	t.Cleanup(func() { cleanupBridge(t, bridge) })

	ni := NewNetworkIsolator("10.96.0.0/24", bridge, 1500)

	_, err := ni.Setup("dup-test", NetworkPolicy{Mode: NetIsolated, DNS: []string{"10.96.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer ni.Teardown("dup-test") //nolint:errcheck

	// Second setup with same name should fail.
	_, err = ni.Setup("dup-test", NetworkPolicy{Mode: NetIsolated, DNS: []string{"10.96.0.1"}})
	if err == nil {
		t.Fatal("expected error on duplicate setup")
	}
}
