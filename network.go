//go:build linux

package mirage

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// Sentinel errors for network operations.
var (
	ErrNetnsExists  = errors.New("mirage: network namespace already exists")
	ErrNetnsUnknown = errors.New("mirage: unknown network namespace")
)

// NetOps abstracts kernel network operations for testability.
// Implement this interface to stub out kernel calls in tests.
type NetOps interface {
	CreateNamespace(name string) error
	DeleteNamespace(name string) error
	CreateVethPair(host, container string, mtu int) error
	DeleteLink(name string) error
	MoveVethToNs(vethName, nsName string) error
	EnsureBridge(bridgeName string, gatewayIP net.IP, subnet *net.IPNet, mtu int) error
	AttachToBridge(vethName, bridgeName string) error
	BringUp(linkName string) error
	ConfigureContainerNs(nsName, vethName string, containerIP, gatewayIP net.IP, prefixLen int, policy NetworkPolicy) error
}

// isolatedNet tracks one network namespace's resources.
type isolatedNet struct {
	nsName   string
	vethHost string
	vethCont string
	ip       net.IP
}

// NetworkIsolator creates per-space network namespaces with iptables egress rules.
type NetworkIsolator struct {
	subnet string // CIDR, e.g. "10.88.0.0/24"
	bridge string // bridge name, e.g. "mirage0"
	mtu    int
	ops    NetOps // kernel operations (real or stub)

	mu     sync.Mutex
	nextIP uint32 // last octet counter for container IPs
	netns  map[string]*isolatedNet
}

// NewNetworkIsolator creates a network isolator with real kernel operations.
func NewNetworkIsolator(subnet, bridge string, mtu int) *NetworkIsolator {
	return NewNetworkIsolatorWithOps(subnet, bridge, mtu, &realNetOps{})
}

// NewNetworkIsolatorWithOps creates a NetworkIsolator with custom operations (for testing).
func NewNetworkIsolatorWithOps(subnet, bridge string, mtu int, ops NetOps) *NetworkIsolator {
	if mtu <= 0 {
		mtu = 1500
	}
	if bridge == "" {
		bridge = "mirage0"
	}
	return &NetworkIsolator{
		subnet: subnet,
		bridge: bridge,
		mtu:    mtu,
		ops:    ops,
		nextIP: 1, // .1 is gateway, containers start at .2
		netns:  make(map[string]*isolatedNet),
	}
}

// Setup creates a network namespace with iptables egress rules from the NetworkPolicy.
func (ni *NetworkIsolator) Setup(name string, policy NetworkPolicy) (string, error) {
	ni.mu.Lock()
	defer ni.mu.Unlock()

	if _, exists := ni.netns[name]; exists {
		return "", fmt.Errorf("%w: %s", ErrNetnsExists, name)
	}

	_, subnet, err := net.ParseCIDR(ni.subnet)
	if err != nil {
		return "", fmt.Errorf("mirage: invalid subnet %s: %w", ni.subnet, err)
	}

	// Allocate container IP.
	ipNum := atomic.AddUint32(&ni.nextIP, 1)
	containerIP := make(net.IP, len(subnet.IP))
	copy(containerIP, subnet.IP)
	containerIP[3] = byte(ipNum)

	gatewayIP := make(net.IP, len(subnet.IP))
	copy(gatewayIP, subnet.IP)
	gatewayIP[3] = 1

	nsName := "mirage-" + name
	vethHost := "vh-" + name
	vethCont := "vc-" + name
	if len(vethHost) > 15 {
		vethHost = vethHost[:15]
	}
	if len(vethCont) > 15 {
		vethCont = vethCont[:15]
	}

	// 1. Create named network namespace.
	if err := ni.ops.CreateNamespace(nsName); err != nil {
		return "", fmt.Errorf("mirage: create netns %s: %w", nsName, err)
	}

	// 2. Create veth pair.
	if err := ni.ops.CreateVethPair(vethHost, vethCont, ni.mtu); err != nil {
		ni.ops.DeleteNamespace(nsName) //nolint:errcheck
		return "", fmt.Errorf("mirage: create veth: %w", err)
	}

	// 3. Move container veth into namespace.
	if err := ni.ops.MoveVethToNs(vethCont, nsName); err != nil {
		ni.ops.DeleteLink(vethHost) //nolint:errcheck
		ni.ops.DeleteNamespace(nsName) //nolint:errcheck
		return "", fmt.Errorf("mirage: move veth to netns: %w", err)
	}

	// 4. Ensure bridge and attach host veth.
	ones, _ := subnet.Mask.Size()
	_ = ones
	if err := ni.ops.EnsureBridge(ni.bridge, gatewayIP, subnet, ni.mtu); err != nil {
		ni.ops.DeleteLink(vethHost) //nolint:errcheck
		ni.ops.DeleteNamespace(nsName) //nolint:errcheck
		return "", fmt.Errorf("mirage: setup bridge: %w", err)
	}
	if err := ni.ops.AttachToBridge(vethHost, ni.bridge); err != nil {
		ni.ops.DeleteNamespace(nsName) //nolint:errcheck
		return "", fmt.Errorf("mirage: attach veth to bridge: %w", err)
	}
	if err := ni.ops.BringUp(vethHost); err != nil {
		ni.ops.DeleteNamespace(nsName) //nolint:errcheck
		return "", fmt.Errorf("mirage: bring up host veth: %w", err)
	}

	// 5. Configure container namespace (IP, routes, iptables).
	prefixLen, _ := subnet.Mask.Size()
	if err := ni.ops.ConfigureContainerNs(nsName, vethCont, containerIP, gatewayIP, prefixLen, policy); err != nil {
		ni.ops.DeleteNamespace(nsName) //nolint:errcheck
		return "", fmt.Errorf("mirage: configure netns: %w", err)
	}

	ni.netns[name] = &isolatedNet{
		nsName:   nsName,
		vethHost: vethHost,
		vethCont: vethCont,
		ip:       containerIP,
	}

	slog.Info("mirage: network namespace created",
		slog.String("ns", nsName),
		slog.String("ip", containerIP.String()),
	)
	return nsName, nil
}

// Teardown removes the network namespace for a space.
func (ni *NetworkIsolator) Teardown(name string) error {
	ni.mu.Lock()
	iso, ok := ni.netns[name]
	if !ok {
		ni.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNetnsUnknown, name)
	}
	delete(ni.netns, name)
	ni.mu.Unlock()

	ni.ops.DeleteLink(iso.vethHost) //nolint:errcheck
	ni.ops.DeleteNamespace(iso.nsName) //nolint:errcheck
	slog.Info("mirage: network namespace removed", slog.String("ns", iso.nsName))
	return nil
}

// TeardownAll removes all network namespaces.
func (ni *NetworkIsolator) TeardownAll() {
	ni.mu.Lock()
	snapshot := make(map[string]*isolatedNet, len(ni.netns))
	for k, v := range ni.netns {
		snapshot[k] = v
	}
	ni.netns = make(map[string]*isolatedNet)
	ni.mu.Unlock()

	for _, iso := range snapshot {
		ni.ops.DeleteLink(iso.vethHost) //nolint:errcheck
		ni.ops.DeleteNamespace(iso.nsName) //nolint:errcheck
	}
}

// NsName returns the namespace name for a space, or empty string.
func (ni *NetworkIsolator) NsName(name string) string {
	ni.mu.Lock()
	defer ni.mu.Unlock()
	if iso, ok := ni.netns[name]; ok {
		return iso.nsName
	}
	return ""
}

// BuildIptablesRules generates iptables rule arguments from a NetworkPolicy.
// Exported for testability. Each element is an argument list for `iptables`.
func BuildIptablesRules(policy NetworkPolicy, gatewayIP net.IP) [][]string {
	rules := [][]string{
		{"-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT"},
	}

	for _, target := range policy.AllowEgress {
		host, port, err := net.SplitHostPort(target)
		if err != nil {
			continue
		}
		rules = append(rules,
			[]string{"-A", "OUTPUT", "-d", host, "-p", "tcp", "--dport", port, "-j", "ACCEPT"},
		)
	}

	rules = append(rules,
		[]string{"-A", "OUTPUT", "-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
	)

	for _, dnsServer := range policy.DNS {
		rules = append(rules,
			[]string{"-A", "OUTPUT", "-d", dnsServer, "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
			[]string{"-A", "OUTPUT", "-d", dnsServer, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"},
		)
	}
	rules = append(rules,
		[]string{"-A", "OUTPUT", "-d", gatewayIP.String(), "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
		[]string{"-A", "OUTPUT", "-d", gatewayIP.String(), "-p", "tcp", "--dport", "53", "-j", "ACCEPT"},
	)

	rules = append(rules, []string{"-A", "OUTPUT", "-j", "DROP"})
	return rules
}

// --- Real kernel implementation ---

type realNetOps struct{}

func (r *realNetOps) CreateNamespace(name string) error {
	ns, err := netns.NewNamed(name)
	if err != nil {
		return err
	}
	return ns.Close()
}

func (r *realNetOps) DeleteNamespace(name string) error {
	return netns.DeleteNamed(name)
}

func (r *realNetOps) CreateVethPair(host, container string, mtu int) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: host, MTU: mtu},
		PeerName:  container,
	}
	return netlink.LinkAdd(veth)
}

func (r *realNetOps) DeleteLink(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return err
	}
	return netlink.LinkDel(link)
}

func (r *realNetOps) MoveVethToNs(vethName, nsName string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	nsHandle, err := netns.GetFromName(nsName)
	if err != nil {
		return err
	}
	defer nsHandle.Close()

	link, err := netlink.LinkByName(vethName)
	if err != nil {
		return err
	}
	return netlink.LinkSetNsFd(link, int(nsHandle))
}

func (r *realNetOps) EnsureBridge(bridgeName string, gatewayIP net.IP, subnet *net.IPNet, mtu int) error {
	br, err := netlink.LinkByName(bridgeName)
	if err != nil {
		bridge := &netlink.Bridge{
			LinkAttrs: netlink.LinkAttrs{Name: bridgeName, MTU: mtu},
		}
		if err := netlink.LinkAdd(bridge); err != nil {
			return fmt.Errorf("create bridge: %w", err)
		}
		br = bridge
	}

	addrs, _ := netlink.AddrList(br, netlink.FAMILY_V4)
	hasAddr := false
	for _, a := range addrs {
		if a.IP.Equal(gatewayIP) {
			hasAddr = true
			break
		}
	}
	if !hasAddr {
		addr := &netlink.Addr{IPNet: &net.IPNet{IP: gatewayIP, Mask: subnet.Mask}}
		if err := netlink.AddrAdd(br, addr); err != nil {
			return fmt.Errorf("add gateway IP: %w", err)
		}
	}
	return netlink.LinkSetUp(br)
}

func (r *realNetOps) AttachToBridge(vethName, bridgeName string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hostLink, err := netlink.LinkByName(vethName)
	if err != nil {
		return err
	}
	br, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return err
	}
	return netlink.LinkSetMaster(hostLink, br.(*netlink.Bridge))
}

func (r *realNetOps) BringUp(linkName string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return err
	}
	return netlink.LinkSetUp(link)
}

func (r *realNetOps) ConfigureContainerNs(nsName, vethName string, containerIP, gatewayIP net.IP, prefixLen int, policy NetworkPolicy) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	origNs, err := netns.Get()
	if err != nil {
		return fmt.Errorf("get current netns: %w", err)
	}
	defer origNs.Close()

	targetNs, err := netns.GetFromName(nsName)
	if err != nil {
		return fmt.Errorf("get target netns: %w", err)
	}
	defer targetNs.Close()

	if err := netns.Set(targetNs); err != nil {
		return fmt.Errorf("switch to netns: %w", err)
	}
	defer func() { _ = netns.Set(origNs) }()

	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("find loopback: %w", err)
	}
	if err := netlink.LinkSetUp(lo); err != nil {
		return fmt.Errorf("bring up loopback: %w", err)
	}

	link, err := netlink.LinkByName(vethName)
	if err != nil {
		return fmt.Errorf("find container veth: %w", err)
	}
	addr := &netlink.Addr{IPNet: &net.IPNet{IP: containerIP, Mask: net.CIDRMask(prefixLen, 32)}}
	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("add IP to veth: %w", err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("bring up veth: %w", err)
	}

	if err := netlink.RouteAdd(&netlink.Route{Gw: gatewayIP}); err != nil {
		return fmt.Errorf("add default route: %w", err)
	}

	rules := BuildIptablesRules(policy, gatewayIP)
	for _, rule := range rules {
		cmd := exec.Command("iptables", rule...) //nolint:gosec
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("iptables %v: %s: %w", rule, strings.TrimSpace(string(out)), err)
		}
	}

	return nil
}
