package stack

import "testing"

func TestLeaderIdentityUsesPodName(t *testing.T) {
	t.Setenv("POD_NAME", "pod-1")
	if got := leaderIdentity(); got != "pod-1" {
		t.Fatalf("expected leader identity to use POD_NAME, got %q", got)
	}
}

func TestLeaderIdentityUsesEnvOverride(t *testing.T) {
	t.Setenv("LEADER_ELECTION_IDENTITY", "local-override")
	t.Setenv("POD_NAME", "pod-1")
	if got := leaderIdentity(); got != "local-override" {
		t.Fatalf("expected leader identity to use override, got %q", got)
	}
}

func TestLeaderIdentityFallbackUsesPID(t *testing.T) {
	t.Setenv("LEADER_ELECTION_IDENTITY", "")
	t.Setenv("POD_NAME", "")
	got := leaderIdentity()
	if got == "" {
		t.Fatalf("expected leader identity fallback to be non-empty")
	}
}
