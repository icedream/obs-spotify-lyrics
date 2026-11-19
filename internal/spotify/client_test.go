package spotify

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fixedClockClient returns a client whose clock is controlled by the test.
func fixedClockClient(t *testing.T) (*Client, *time.Time) {
	t.Helper()

	c := NewClient("sp_dc", "device")
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	return c, &now
}

func TestDealerConnIDCacheTTL(t *testing.T) {
	c, now := fixedClockClient(t)

	require.Empty(t, c.cachedDealerConnID(), "a fresh client has no connection ID")

	c.cacheDealerConnID("conn-1")
	require.Equal(t, now.Add(dealerConnIDTTL), c.dealerConnIDExpiry, "expiry is TTL ahead of the clock")

	*now = now.Add(dealerConnIDTTL - time.Millisecond)
	require.Equal(t, "conn-1", c.cachedDealerConnID(), "still inside the TTL window")

	// The boundary itself counts as expired.
	*now = now.Add(time.Millisecond)
	require.Empty(t, c.cachedDealerConnID(), "expired at the TTL boundary")
}

func TestDealerConnIDCacheInvalidation(t *testing.T) {
	c, _ := fixedClockClient(t)

	c.cacheDealerConnID("conn-1")
	require.Equal(t, "conn-1", c.cachedDealerConnID())

	c.invalidateDealerConnID()
	require.Empty(t, c.cachedDealerConnID(), "invalidated inside the TTL window")
	require.Empty(t, c.dealerConnID)
	require.True(t, c.dealerConnIDExpiry.IsZero())

	// A dialled ID takes over from the dropped one.
	c.cacheDealerConnID("conn-2")
	require.Equal(t, "conn-2", c.cachedDealerConnID())
}

func TestDealerConnIDCacheOverwrite(t *testing.T) {
	c, now := fixedClockClient(t)

	c.cacheDealerConnID("conn-1")
	*now = now.Add(time.Minute)
	c.cacheDealerConnID("conn-2")

	require.Equal(t, "conn-2", c.cachedDealerConnID())
	require.Equal(t, now.Add(dealerConnIDTTL), c.dealerConnIDExpiry, "a dial re-arms the TTL")
}
