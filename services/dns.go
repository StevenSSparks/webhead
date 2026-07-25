package services

import (
	"net"

	"github.com/miekg/dns"
	"github.com/stevenssparks/roost/device"
)

// NewDNSHandler answers every A/ANY query for any name with answerIP (TTL 30)
// and records the lookup, mirroring the board's wildcard DNS funnel.
func NewDNSHandler(st *device.State, answerIP string) dns.Handler {
	ip := net.ParseIP(answerIP)
	return dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(req)
		m.Authoritative = true
		for _, q := range req.Question {
			st.NoteDNS(q.Name, answerIP)
			if q.Qtype == dns.TypeA || q.Qtype == dns.TypeANY {
				m.Answer = append(m.Answer, &dns.A{
					Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
					A:   ip,
				})
			}
		}
		_ = w.WriteMsg(m)
	})
}

// StartDNS starts a UDP DNS server on addr and returns once it is bound. The
// caller keeps the server for shutdown.
func StartDNS(st *device.State, addr, answerIP string) (*dns.Server, error) {
	srv := &dns.Server{Addr: addr, Net: "udp", Handler: NewDNSHandler(st, answerIP)}
	errc := make(chan error, 1)
	srv.NotifyStartedFunc = func() { errc <- nil }
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			errc <- err
		}
	}()
	return srv, <-errc
}
