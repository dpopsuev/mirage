package testkit

import (
	"fmt"
	"net"
	"sync"

	"github.com/dpopsuev/mirage"
)

// StubNetOps records all network operations without touching the kernel.
// Use for unit testing NetworkIsolator logic (IP allocation, naming,
// duplicate detection, teardown tracking) without root.
type StubNetOps struct {
	mu sync.Mutex

	// Recorded state.
	Namespaces map[string]bool            // created namespaces
	VethPairs  map[string]string          // host → container
	Bridges    map[string]net.IP          // bridge → gateway IP
	Attached   map[string]string          // veth → bridge
	BroughtUp  []string                   // links brought up
	Configured []StubContainerNsCall      // configureContainerNs calls

	// Configurable errors — set before calling Setup to inject failures.
	CreateNamespaceErr      error
	CreateVethPairErr       error
	MoveVethToNsErr         error
	EnsureBridgeErr         error
	AttachToBridgeErr       error
	BringUpErr              error
	ConfigureContainerNsErr error
}

// StubContainerNsCall records one ConfigureContainerNs invocation.
type StubContainerNsCall struct {
	NsName      string
	VethName    string
	ContainerIP net.IP
	GatewayIP   net.IP
	PrefixLen   int
	Policy      mirage.NetworkPolicy
}

// NewStubNetOps creates a fresh stub with empty state.
func NewStubNetOps() *StubNetOps {
	return &StubNetOps{
		Namespaces: make(map[string]bool),
		VethPairs:  make(map[string]string),
		Bridges:    make(map[string]net.IP),
		Attached:   make(map[string]string),
	}
}

func (s *StubNetOps) CreateNamespace(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.CreateNamespaceErr != nil {
		return s.CreateNamespaceErr
	}
	if s.Namespaces[name] {
		return fmt.Errorf("namespace %s already exists", name)
	}
	s.Namespaces[name] = true
	return nil
}

func (s *StubNetOps) DeleteNamespace(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Namespaces, name)
	return nil
}

func (s *StubNetOps) CreateVethPair(host, container string, mtu int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.CreateVethPairErr != nil {
		return s.CreateVethPairErr
	}
	s.VethPairs[host] = container
	return nil
}

func (s *StubNetOps) DeleteLink(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.VethPairs, name)
	delete(s.Attached, name)
	return nil
}

func (s *StubNetOps) MoveVethToNs(vethName, nsName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.MoveVethToNsErr != nil {
		return s.MoveVethToNsErr
	}
	return nil
}

func (s *StubNetOps) EnsureBridge(bridgeName string, gatewayIP net.IP, subnet *net.IPNet, mtu int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.EnsureBridgeErr != nil {
		return s.EnsureBridgeErr
	}
	s.Bridges[bridgeName] = gatewayIP
	return nil
}

func (s *StubNetOps) AttachToBridge(vethName, bridgeName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.AttachToBridgeErr != nil {
		return s.AttachToBridgeErr
	}
	s.Attached[vethName] = bridgeName
	return nil
}

func (s *StubNetOps) BringUp(linkName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.BringUpErr != nil {
		return s.BringUpErr
	}
	s.BroughtUp = append(s.BroughtUp, linkName)
	return nil
}

func (s *StubNetOps) ConfigureContainerNs(nsName, vethName string, containerIP, gatewayIP net.IP, prefixLen int, policy mirage.NetworkPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ConfigureContainerNsErr != nil {
		return s.ConfigureContainerNsErr
	}
	s.Configured = append(s.Configured, StubContainerNsCall{
		NsName:      nsName,
		VethName:    vethName,
		ContainerIP: containerIP,
		GatewayIP:   gatewayIP,
		PrefixLen:   prefixLen,
		Policy:      policy,
	})
	return nil
}
