package sandbox_test

import (
	"testing"

	"github.com/onohiroki/go-mrbvm/sandbox"
)

func TestPolicy(t *testing.T) {
	policy := sandbox.New()
	if policy.Allowed(sandbox.Network) {
		t.Fatal("new policy should deny Network")
	}
	policy.Permit(sandbox.Network, sandbox.FileRead)
	if !policy.Allowed(sandbox.Network) || !policy.Allowed(sandbox.FileRead) {
		t.Fatal("permitted capabilities should be allowed")
	}
	policy.Deny(sandbox.Network)
	if policy.Allowed(sandbox.Network) {
		t.Fatal("denied capability should not be allowed")
	}
	if !sandbox.AllowAll().Allowed(sandbox.Process) {
		t.Fatal("AllowAll should permit Process")
	}
}
