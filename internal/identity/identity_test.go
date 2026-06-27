package identity

import "testing"

func TestForChannelThread_Deterministic(t *testing.T) {
	a := ForChannelThread("acme", "T1", "C1", "1700.1")
	b := ForChannelThread("acme", "T1", "C1", "1700.1")
	if a != b {
		t.Fatalf("not deterministic: %+v vs %+v", a, b)
	}
	if a.UserID != "opentag:org:acme:T1:C1" {
		t.Errorf("service identity = %q", a.UserID)
	}
}

func TestForChannelThread_SharedAcrossThreadsSameChannel(t *testing.T) {
	// Same channel, different threads: same service identity, different sessions.
	t1 := ForChannelThread("acme", "T1", "C1", "100.1")
	t2 := ForChannelThread("acme", "T1", "C1", "200.2")
	if t1.UserID != t2.UserID {
		t.Errorf("channel identity should be shared across threads")
	}
	if t1.SessionID == t2.SessionID {
		t.Errorf("different threads should get different sessions")
	}
}

func TestForDM_PersonalIdentity(t *testing.T) {
	d := ForDM("T1", "U9", "")
	if d.UserID != "opentag:user:T1:U9" {
		t.Errorf("DM identity = %q", d.UserID)
	}
	ch := ForChannelThread("acme", "T1", "C1", "1.1")
	if d.UserID == ch.UserID {
		t.Errorf("DM and channel identities must differ")
	}
}
