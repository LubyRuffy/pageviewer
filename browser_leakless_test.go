package pageviewer

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveLeaklessEnabledKeepsDisabledWhenNotRequested(t *testing.T) {
	assert.False(t, resolveLeaklessEnabled(false, 0, 50*time.Millisecond, 10*time.Millisecond))
}

func TestResolveLeaklessEnabledFallsBackWhenLockPortIsBusy(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	start := time.Now()
	enabled := resolveLeaklessEnabled(true, port, 50*time.Millisecond, 10*time.Millisecond)
	assert.False(t, enabled)
	assert.Less(t, time.Since(start), 200*time.Millisecond)
}

func TestResolveLeaklessEnabledStaysEnabledWhenLockPortIsFree(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())

	assert.True(t, resolveLeaklessEnabled(true, port, 50*time.Millisecond, 10*time.Millisecond))
}
