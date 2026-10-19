package transport

import (
	"net/url"
	"testing"
)

func TestURLHost(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		host string
		port uint16
		want string
	}{
		{name: "IPv6 default port", host: "::1", want: "[::1]"},
		{name: "IPv6 explicit default port", host: "2001:db8::1", port: 443, want: "[2001:db8::1]"},
		{name: "IPv6 non default port", host: "::1", port: 8443, want: "[::1]:8443"},
		{name: "IPv6 zone", host: "fe80::1%eth0", want: "[fe80::1%eth0]"},
		{name: "IPv6 zone non default port", host: "fe80::1%eth0", port: 8443, want: "[fe80::1%eth0]:8443"},
		{name: "IPv6 already bracketed", host: "[::1]", want: "[::1]"},
		{name: "IPv6 already bracketed non default port", host: "[::1]", port: 8443, want: "[::1]:8443"},
		{name: "IPv6 bracketed with port", host: "[2001:db8::1]:8443", want: "[2001:db8::1]:8443"},
		{name: "IPv4", host: "127.0.0.1", want: "127.0.0.1"},
		{name: "IPv4 non default port", host: "127.0.0.1", port: 8443, want: "127.0.0.1:8443"},
		{name: "domain", host: "dns.example.com", want: "dns.example.com"},
		{name: "domain non default port", host: "dns.example.com", port: 8443, want: "dns.example.com:8443"},
		{name: "host with port", host: "dns.example.com:8443", want: "dns.example.com:8443"},
		{name: "host with port replaced", host: "dns.example.com:8443", port: 9443, want: "dns.example.com:9443"},
		{name: "host with port default port", host: "dns.example.com:8443", port: 443, want: "dns.example.com:8443"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got := URLHost(testCase.host, testCase.port)
			if got != testCase.want {
				t.Fatalf("URLHost(%q, %d) = %q, want %q", testCase.host, testCase.port, got, testCase.want)
			}
			// The transports build a url.URL from this host and then parse its
			// string form when creating the request, so use the same round trip.
			destination := url.URL{Scheme: "https", Host: got, Path: "/dns-query"}
			_, err := url.Parse(destination.String())
			if err != nil {
				t.Fatalf("url.Parse rejected URL host %q: %v", got, err)
			}
		})
	}
}
