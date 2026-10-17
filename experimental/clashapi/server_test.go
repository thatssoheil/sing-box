package clashapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListenNetwork(t *testing.T) {
	t.Parallel()

	require.Equal(t, "tcp4", listenNetwork("0.0.0.0:9090"))
	require.Equal(t, "tcp", listenNetwork("127.0.0.1:9090"))
	require.Equal(t, "tcp", listenNetwork("[::]:9090"))
	require.Equal(t, "tcp", listenNetwork(":9090"))
	require.Equal(t, "tcp", listenNetwork("localhost:9090"))
	require.Equal(t, "tcp", listenNetwork(""))
}
