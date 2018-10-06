// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package httprecord

import (
	"github.com/miekg/dns"
	"net"
	"strconv"
	"strings"
)

type recordLine string

func (r recordLine) Type() string {
	p := strings.Split(string(r), " ")
	if len(p) >= 2 && isType(p[0]) {
		return p[0]
	} else {
		return ""
	}
}

func (r recordLine) TTL() uint32 {
	p := strings.Split(string(r), " ")
	if len(p) >= 3 && isType(p[0]) {
		ttl, _ := strconv.Atoi(p[1])
		return uint32(ttl)
	} else {
		return 0
	}
}

func (r recordLine) Payload() string {
	p := strings.Split(string(r), " ")

	if len(p) > 1 && isType(p[0]) {
		p = p[1:]

		if len(p) > 1 {
			_, err := strconv.Atoi(p[0])
			if err == nil {
				p = p[1:]
			}
		}
	}

	return strings.Join(p, " ")
}

func isType(t string) bool {
	_, ok := dns.StringToType[t]
	return ok
}

func parseLines(response string) []recordLine {
	var result []recordLine

	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		result = append(result, recordLine(line))
	}

	return result
}

func parseTXT(name string, ttl uint32, response string) ([]dns.RR, error) {
	var rrs []dns.RR

	for _, l := range parseLines(response) {
		t := l.Type()
		rttl := l.TTL()
		if rttl == 0 || rttl > ttl {
			rttl = ttl
		}

		if t == "" || t == "TXT" {
			rr := new(dns.TXT)
			rr.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeTXT,
				Class: dns.ClassINET, Ttl: rttl}
			rr.Txt = []string{l.Payload()}

			rrs = append(rrs, rr)
		}
	}

	return rrs, nil
}

func parseA(name string, ttl uint32, response string) ([]dns.RR, error) {
	var rrs []dns.RR

	for _, l := range parseLines(response) {
		t := l.Type()
		rttl := l.TTL()
		if rttl == 0 || rttl > ttl {
			rttl = ttl
		}

		if t == "" || t == "A" {
			ip := net.ParseIP(l.Payload())
			if t == "" && ip.To4() == nil {
				// If the record type was unspecified and this is not a v4 address, ignore it.
				continue
			}

			rr := new(dns.A)
			rr.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeA,
				Class: dns.ClassINET, Ttl: rttl}
			rr.A = ip

			rrs = append(rrs, rr)
		}
	}

	return rrs, nil
}

func parseAAAA(name string, ttl uint32, response string) ([]dns.RR, error) {
	var rrs []dns.RR

	for _, l := range parseLines(response) {
		t := l.Type()
		rttl := l.TTL()
		if rttl == 0 || rttl > ttl {
			rttl = ttl
		}

		if t == "" || t == "AAAA" {
			ip := net.ParseIP(l.Payload())
			if t == "" && ip.To4() != nil {
				// If the record type was unspecified and this is a v4 address, ignore it.
				continue
			}

			rr := new(dns.AAAA)
			rr.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeAAAA,
				Class: dns.ClassINET, Ttl: rttl}
			rr.AAAA = ip

			rrs = append(rrs, rr)
		}
	}

	return rrs, nil
}
