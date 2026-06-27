package identity

import "testing"

func TestForChannelThread_Deterministic(t *testing.T) {
	a := ForChannelThread("acme", "T1", "C1", "1700.1", true)
	b := ForChannelThread("acme", "T1", "C1", "1700.1", true)
	if a != b {
		t.Fatalf("not deterministic: %+v vs %+v", a, b)
	}
	if a.UserID != "opentag:org:acme:T1:C1" {
		t.Errorf("service identity = %q", a.UserID)
	}
}

func TestForChannelThread_SharedMemoryAcrossThreads(t *testing.T) {
	// Shared (default): same channel identity across threads → cross-thread memory;
	// distinct sessions per thread.
	t1 := ForChannelThread("acme", "T1", "C1", "100.1", true)
	t2 := ForChannelThread("acme", "T1", "C1", "200.2", true)
	if t1.UserID != t2.UserID {
		t.Errorf("shared memory: channel identity should be the same across threads")
	}
	if t1.SessionID == t2.SessionID {
		t.Errorf("different threads should get different sessions")
	}
}

func TestForChannelThread_IsolatedMemory(t *testing.T) {
	// Not shared: each thread gets its own identity → memory does not cross threads.
	t1 := ForChannelThread("acme", "T1", "C1", "100.1", false)
	t2 := ForChannelThread("acme", "T1", "C1", "200.2", false)
	if t1.UserID == t2.UserID {
		t.Errorf("isolated memory: threads should get different identities")
	}
}

func TestForDM_PersonalIdentity(t *testing.T) {
	d := ForDM("T1", "U9", "")
	if d.UserID != "opentag:user:T1:U9" {
		t.Errorf("DM identity = %q", d.UserID)
	}
	ch := ForChannelThread("acme", "T1", "C1", "1.1", true)
	if d.UserID == ch.UserID {
		t.Errorf("DM and channel identities must differ")
	}
}
