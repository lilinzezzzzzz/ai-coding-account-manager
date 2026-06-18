package security_test

import (
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
)

func TestManagerWithWildcardBindAddrAllowsLoopbackHosts(t *testing.T) {
	manager := newManager(t, "0.0.0.0:43127")

	for _, host := range []string{"127.0.0.1:43127", "localhost:43127"} {
		if !manager.ValidateHost(host) {
			t.Fatalf("ValidateHost(%q) = false, want true", host)
		}
	}
}

func TestManagerWithWildcardBindAddrRejectsWildcardAndRemoteHosts(t *testing.T) {
	manager := newManager(t, "0.0.0.0:43127")

	for _, host := range []string{"0.0.0.0:43127", "192.168.1.10:43127"} {
		if manager.ValidateHost(host) {
			t.Fatalf("ValidateHost(%q) = true, want false", host)
		}
	}
}

func TestManagerWithWildcardBindAddrRejectsWildcardOrigin(t *testing.T) {
	manager := newManager(t, "0.0.0.0:43127")

	if manager.ValidateOrigin("http://0.0.0.0:43127") {
		t.Fatal("ValidateOrigin(http://0.0.0.0:43127) = true, want false")
	}
}

func newManager(t *testing.T, bindAddr string) *security.Manager {
	t.Helper()

	manager, err := security.NewManager(security.Config{BindAddr: bindAddr})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager
}
