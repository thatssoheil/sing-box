//go:build linux

package redirect

import (
	"context"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/common/process"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/stretchr/testify/require"
)

type captureRouter struct {
	adapter.Router
	metadata adapter.InboundContext
}

func (r *captureRouter) RouteConnectionEx(_ context.Context, _ net.Conn, metadata adapter.InboundContext, _ N.CloseHandlerFunc) {
	r.metadata = metadata
}

// TestNewConnectionOriginalDestination redirects a local connection with
// iptables and checks that the inbound keeps the original destination in
// OriginDestination: process search looks up the originating socket by that
// address, and the accept address (the redirect listener) never matches.
func TestNewConnectionOriginalDestination(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root; re-run with `go test -exec sudo`")
	}
	iptables, err := exec.LookPath("iptables")
	if err != nil {
		t.Skip("iptables not available")
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	listenPort := M.AddrPortFromNet(listener.Addr()).Port()

	target := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.2"), listenPort)
	listenPortString := strconv.Itoa(int(listenPort))

	rule := []string{"-t", "nat", "-A", "OUTPUT", "-p", "tcp", "-d", "127.0.0.2/32", "--dport", listenPortString, "-j", "REDIRECT", "--to-port", listenPortString}
	require.NoError(t, exec.Command(iptables, rule...).Run())
	t.Cleanup(func() {
		_ = exec.Command(iptables, append([]string{"-t", "nat", "-D", "OUTPUT"}, rule[4:]...)...).Run()
	})

	clientConn, err := net.DialTimeout("tcp4", target.String(), 5*time.Second)
	require.NoError(t, err)
	defer clientConn.Close()

	require.NoError(t, listener.(*net.TCPListener).SetDeadline(time.Now().Add(5*time.Second)))
	serverConn, err := listener.Accept()
	require.NoError(t, err)
	defer serverConn.Close()

	capture := &captureRouter{}
	redirect := &Redirect{
		Adapter: inbound.NewAdapter(C.TypeRedirect, "test"),
		router:  capture,
		logger:  log.NewNOPFactory().Logger(),
	}
	metadata := adapter.InboundContext{
		Source:            M.SocksaddrFromNet(serverConn.RemoteAddr()).Unwrap(),
		OriginDestination: M.SocksaddrFromNet(serverConn.LocalAddr()).Unwrap(),
	}
	redirect.NewConnection(context.Background(), serverConn, metadata, nil)

	require.Equal(t, target, capture.metadata.Destination.AddrPort())
	require.Equal(t, target, capture.metadata.OriginDestination.AddrPort())

	searcher, err := process.NewSearcher(process.Config{Logger: log.NewNOPFactory().Logger()})
	require.NoError(t, err)
	defer searcher.Close()
	info, err := process.FindProcessInfo(searcher, context.Background(), N.NetworkTCP, M.AddrPortFromNet(serverConn.RemoteAddr()), capture.metadata.OriginDestination.AddrPort())
	require.NoError(t, err)
	require.Equal(t, int32(os.Getuid()), info.UserId)
}
