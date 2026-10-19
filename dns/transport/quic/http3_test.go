package quic

import (
	"context"
	"net/url"
	"testing"

	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"

	badoption "github.com/sagernet/sing/common/json/badoption"
)

func newHTTP3Probe(t *testing.T, options option.RemoteHTTPSDNSServerOptions) *HTTP3Transport {
	t.Helper()
	transport, err := NewHTTP3(context.Background(), log.NewNOPFactory().Logger(), "probe", options)
	if err != nil {
		t.Fatal(err)
	}
	return transport.(*HTTP3Transport)
}

func TestHTTP3DestinationHost(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		server      string
		serverPort  uint16
		hostHeader  string
		wantHost    string
		wantAddress string
	}{
		{
			name:        "bare IPv6 default port",
			server:      "::1",
			wantHost:    "[::1]",
			wantAddress: "https://[::1]/dns-query",
		},
		{
			name:        "bare IPv6 non default port",
			server:      "::1",
			serverPort:  8443,
			wantHost:    "[::1]:8443",
			wantAddress: "https://[::1]:8443/dns-query",
		},
		{
			name:        "bare IPv6 from Host header",
			server:      "2001:db8::1",
			hostHeader:  "::1",
			wantHost:    "[::1]",
			wantAddress: "https://[::1]/dns-query",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			headers := make(badoption.HTTPHeader)
			if testCase.hostHeader != "" {
				headers["Host"] = badoption.Listable[string]{testCase.hostHeader}
			}
			transport := newHTTP3Probe(t, option.RemoteHTTPSDNSServerOptions{
				RemoteTLSDNSServerOptions: option.RemoteTLSDNSServerOptions{
					RemoteDNSServerOptions: option.RemoteDNSServerOptions{
						DNSServerAddressOptions: option.DNSServerAddressOptions{
							Server:     testCase.server,
							ServerPort: testCase.serverPort,
						},
					},
					OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{
						TLS: &option.OutboundTLSOptions{Enabled: true},
					},
				},
				Headers: headers,
			})
			destination := transport.destination
			if destination.Host != testCase.wantHost {
				t.Fatalf("destination host = %q, want %q", destination.Host, testCase.wantHost)
			}
			if destination.String() != testCase.wantAddress {
				t.Fatalf("destination = %q, want %q", destination.String(), testCase.wantAddress)
			}
			if _, err := url.Parse(destination.String()); err != nil {
				t.Fatalf("url.Parse rejected destination %q: %v", destination.String(), err)
			}
		})
	}
}
