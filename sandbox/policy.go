// Package sandbox defines capability policy for go-mrbvm embeddings.
package sandbox

// Capability is a host resource that an embedding may allow.
type Capability string

const (
	IO        Capability = "io"
	FileRead  Capability = "file-read"
	FileWrite Capability = "file-write"
	Network   Capability = "network"
	Process   Capability = "process"
	OS        Capability = "os"
	Eval      Capability = "eval"
	Load      Capability = "load"
	Require   Capability = "require"
)

// Policy is a capability allow-list. A nil Policy denies everything.
type Policy struct {
	allowed map[Capability]bool
}

// New creates a policy that denies every capability.
func New() *Policy {
	return &Policy{allowed: make(map[Capability]bool)}
}

// AllowAll creates a policy that permits every known capability.
func AllowAll() *Policy {
	p := New()
	for _, capability := range []Capability{IO, FileRead, FileWrite, Network, Process, OS, Eval, Load, Require} {
		p.allowed[capability] = true
	}
	return p
}

// Permit adds capabilities to the allow-list.
func (p *Policy) Permit(capabilities ...Capability) *Policy {
	if p.allowed == nil {
		p.allowed = make(map[Capability]bool)
	}
	for _, capability := range capabilities {
		p.allowed[capability] = true
	}
	return p
}

// Deny removes capabilities from the allow-list.
func (p *Policy) Deny(capabilities ...Capability) *Policy {
	for _, capability := range capabilities {
		delete(p.allowed, capability)
	}
	return p
}

// Allowed reports whether the capability is permitted.
func (p *Policy) Allowed(capability Capability) bool {
	if p == nil {
		return false
	}
	return p.allowed[capability]
}
