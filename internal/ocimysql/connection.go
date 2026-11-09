package ocimysql

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

const operationLimit = 10 * time.Second
const headerLimit = 16 << 10

func newTransport() *http.Transport {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	return &http.Transport{
		DialContext:     (&net.Dialer{Timeout: operationLimit}).DialContext,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		Protocols:       protocols, DisableKeepAlives: true, DisableCompression: true,
		MaxResponseHeaderBytes: headerLimit,
	}
}

// Each operation owns connection setup and cancellation. Fresh HTTP1 connections
// prevent transport replay; dial/TLS must not outlive the requesting operation.
func (c *Client) operationTransport(ctx context.Context) (*http.Transport, func()) {
	transport := c.transport.Clone()
	dial := transport.DialContext
	transport.DialTLSContext = func(_ context.Context, network, addr string) (net.Conn, error) {
		conn, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		config := transport.TLSClientConfig.Clone()
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		config.ServerName = host
		config.NextProtos = []string{"http/1.1"}
		secured := tls.Client(conn, config)
		if err = secured.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return secured, nil
	}
	return transport, transport.CloseIdleConnections
}
