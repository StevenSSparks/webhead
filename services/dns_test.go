package services

import (
	"net"
	"testing"
	"testing/fstest"

	"github.com/miekg/dns"
	"github.com/stevenssparks/roost/device"
)

type captureWriter struct {
	dns.ResponseWriter
	msg *dns.Msg
}

func (c *captureWriter) WriteMsg(m *dns.Msg) error { c.msg = m; return nil }
func (c *captureWriter) LocalAddr() net.Addr       { return &net.UDPAddr{} }
func (c *captureWriter) RemoteAddr() net.Addr      { return &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4)} }

func TestDNSAnswersEverythingWithLocalhost(t *testing.T) {
	st := device.New(fstest.MapFS{})
	h := NewDNSHandler(st, "127.0.0.1")

	req := new(dns.Msg)
	req.SetQuestion("captive.apple.com.", dns.TypeA)
	cw := &captureWriter{}
	h.ServeDNS(cw, req)

	if cw.msg == nil || len(cw.msg.Answer) != 1 {
		t.Fatalf("no answer: %+v", cw.msg)
	}
	a, ok := cw.msg.Answer[0].(*dns.A)
	if !ok || a.A.String() != "127.0.0.1" {
		t.Fatalf("answer=%v", cw.msg.Answer[0])
	}
	if got := st.RecentDNS(1); len(got) != 1 || got[0].Name != "captive.apple.com." {
		t.Fatalf("dns not logged: %+v", got)
	}
}
