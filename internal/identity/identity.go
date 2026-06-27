// Package identity derives the two pieces of identity OpenTag needs for every
// interaction, deterministically:
//
//   - UserID    — the value sent to kagent as X-User-ID (who the work runs as)
//   - SessionID — the kagent session id, which is also the A2A contextID
//
// A channel mention runs under the org's *service identity* (shared by everyone
// in the channel), while the working conversation is a per-thread session. A DM
// runs under the user's *personal identity*. Determinism matters: the same
// thread must always map to the same session so anyone can pick up where another
// left off.
package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Identity is the resolved (UserID, SessionID) pair for one interaction.
type Identity struct {
	// UserID is sent to kagent as X-User-ID.
	UserID string
	// SessionID is the kagent session id / A2A contextID for the thread.
	SessionID string
}

// short returns a stable 12-hex-char digest of the joined parts.
func short(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// ForChannelThread resolves identity for a channel mention working in a thread.
// The session is always unique per thread (its own conversation). The UserID —
// which keys kagent's memory — is shared across the whole channel when
// sharedMemory is true (so the agent's memory spans threads), or isolated per
// thread when false.
func ForChannelThread(orgID, team, channel, threadTS string, sharedMemory bool) Identity {
	userID := fmt.Sprintf("opentag:org:%s:%s:%s", orgID, team, channel)
	if !sharedMemory {
		userID += ":t-" + short(threadTS)
	}
	return Identity{
		UserID:    userID,
		SessionID: "thread-" + short(team, channel, threadTS),
	}
}

// ForDM resolves identity for a direct message. DMs run under the user's own
// identity, not the org service account.
func ForDM(team, slackUserID, threadTS string) Identity {
	root := threadTS
	if root == "" {
		// A DM without a thread groups under the user's root conversation.
		root = slackUserID
	}
	return Identity{
		UserID:    fmt.Sprintf("opentag:user:%s:%s", team, slackUserID),
		SessionID: "dm-" + short(team, slackUserID, root),
	}
}
