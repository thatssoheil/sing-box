//go:build linux

package tlsspoof

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

const loopbackInterface = "lo"

// TestRawSpooferBindsLocalAddress asserts that the raw socket is bound to the
// connection's local address. Left unbound, the kernel resolves the forged
// segment's route as 0.0.0.0, and auto_route's "from 0.0.0.0 iif lo lookup
// <table>" policy rule sends it into the tun device instead of the wire.
func TestRawSpooferBindsLocalAddress(t *testing.T) {
	requireRoot(t)
	families := []struct {
		name string
		dial func(*testing.T) (net.Conn, uint16)
	}{
		{"IPv4", dialLocalEchoServer},
		{"IPv6", dialLocalEchoServerIPv6},
	}
	for _, family := range families {
		t.Run(family.name, func(t *testing.T) {
			client, _ := family.dial(t)
			spoofer, err := newRawSpoofer(client, MethodWrongAcknowledgment)
			require.NoError(t, err)
			defer spoofer.Close()
			bound, err := unix.Getsockname(spoofer.(*linuxSpoofer).rawFD)
			require.NoError(t, err)
			localAddress := client.LocalAddr().(*net.TCPAddr).AddrPort().Addr().Unmap()
			switch sockaddr := bound.(type) {
			case *unix.SockaddrInet4:
				require.Equal(t, localAddress.As4(), sockaddr.Addr)
			case *unix.SockaddrInet6:
				require.Equal(t, localAddress.As16(), sockaddr.Addr)
			default:
				t.Fatalf("unexpected raw socket address type: %T", bound)
			}
		})
	}
}
