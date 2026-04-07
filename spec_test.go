package mirage

import (
	"testing"
)

func TestMountSpec_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mount   MountSpec
		wantErr bool
	}{
		{"valid bind", MountSpec{Type: MountBind, Source: "/src", Destination: "/dst"}, false},
		{"valid tmpfs", MountSpec{Type: MountTmpfs, Destination: "/tmp"}, false},
		{"empty destination", MountSpec{Type: MountBind, Source: "/src"}, true},
		{"bind without source", MountSpec{Type: MountBind, Destination: "/dst"}, true},
		{"unknown type", MountSpec{Type: "magic", Destination: "/dst"}, true},
		{"tmpfs no source needed", MountSpec{Type: MountTmpfs, Destination: "/tmp"}, false},
		{"proc no source needed", MountSpec{Type: MountProc, Destination: "/proc"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mount.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNetworkPolicy_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		policy  NetworkPolicy
		wantErr bool
	}{
		{"valid none", NetworkPolicy{Mode: NetNone}, false},
		{"valid host", NetworkPolicy{Mode: NetHost}, false},
		{"valid isolated", NetworkPolicy{Mode: NetIsolated, AllowEgress: []string{"api.example.com:443"}}, false},
		{"empty mode", NetworkPolicy{}, true},
		{"unknown mode", NetworkPolicy{Mode: "magic"}, true},
		{"isolated without egress", NetworkPolicy{Mode: NetIsolated}, false}, // valid — blocks everything
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResourceLimits_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		limits  ResourceLimits
		wantErr bool
	}{
		{"valid memory only", ResourceLimits{Memory: "512MB"}, false},
		{"valid all", ResourceLimits{Memory: "2GB", CPUWeight: 100, IOWeight: 50}, false},
		{"bad memory format", ResourceLimits{Memory: "abc"}, true},
		{"cpu too low", ResourceLimits{CPUWeight: -1}, true},
		{"cpu too high", ResourceLimits{CPUWeight: 10001}, true},
		{"io too high", ResourceLimits{IOWeight: 10001}, true},
		{"empty is valid", ResourceLimits{}, false}, // no limits = unlimited
		{"valid KB", ResourceLimits{Memory: "1024KB"}, false},
		{"valid GB", ResourceLimits{Memory: "4GB"}, false},
		{"valid bytes", ResourceLimits{Memory: "1073741824"}, false},
		{"valid pid_max", ResourceLimits{PIDMax: 100}, false},
		{"negative pid_max", ResourceLimits{PIDMax: -1}, true},
		{"zero pid_max", ResourceLimits{PIDMax: 0}, false},
		{"valid cpu_max", ResourceLimits{CPUMax: "200000 100000"}, false},
		{"cpu_max with max quota", ResourceLimits{CPUMax: "max 100000"}, false},
		{"cpu_max missing period", ResourceLimits{CPUMax: "200000"}, true},
		{"cpu_max bad quota", ResourceLimits{CPUMax: "abc 100000"}, true},
		{"cpu_max bad period", ResourceLimits{CPUMax: "200000 abc"}, true},
		{"cpu_max zero period", ResourceLimits{CPUMax: "200000 0"}, true},
		{"cpu_max negative quota", ResourceLimits{CPUMax: "-1 100000"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.limits.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseMemory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  int64
		err   bool
	}{
		{"512KB", 512 * 1024, false},
		{"512MB", 512 * 1024 * 1024, false},
		{"2GB", 2 * 1024 * 1024 * 1024, false},
		{"1073741824", 1073741824, false},
		{"0", 0, false},
		{"abc", 0, true},
		{"-1MB", 0, true},
		{"", 0, false}, // empty = no limit
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMemory(tt.input)
			if (err != nil) != tt.err {
				t.Errorf("ParseMemory(%q) error = %v, wantErr %v", tt.input, err, tt.err)
			}
			if got != tt.want {
				t.Errorf("ParseMemory(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestNamespaceConfig_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		ns      NamespaceConfig
		wantErr bool
	}{
		{"valid minimal", NamespaceConfig{User: true, Mount: true}, false},
		{"network without user", NamespaceConfig{Mount: true, Network: true}, true},
		{"pid without user", NamespaceConfig{Mount: true, PID: true}, true},
		{"empty is valid", NamespaceConfig{}, false}, // no namespaces = overlay only
		{"full", NamespaceConfig{User: true, Mount: true, PID: true, Network: true, IPC: true, UTS: true, Cgroup: true}, false},
		{"cgroup without user", NamespaceConfig{Mount: true, Cgroup: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ns.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
