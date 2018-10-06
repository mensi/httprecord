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
	"context"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testCase struct {
	config    HTTPRecord
	handler   http.HandlerFunc
	tc        test.Case
	shouldErr bool
}

func TestHTTPRecord_ServeDNS(t *testing.T) {
	tests := []testCase{{
		config: HTTPRecord{
			Records: []Record{{
				URI:  "-replace-",
				Name: "example.com.",
				Type: "TXT",
			}},
		},
		handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Write([]byte("Hello"))
		}),
		tc: test.Case{
			Qname: "example.com.", Qtype: dns.TypeTXT,
			Answer: []dns.RR{
				test.TXT("example.com. 3600	IN	TXT Hello"),
			},
		},
	}, {
		config: HTTPRecord{
			Zones: []Zone{{
				URI:    "-replace-",
				Origin: "example.com.",
			}},
		},
		handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Write([]byte("A 1.2.3.4"))
		}),
		tc: test.Case{
			Qname: "foo.example.com.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("foo.example.com. 3600	IN	A 1.2.3.4"),
			},
		},
	}, {
		config: HTTPRecord{
			Zones: []Zone{{
				URI:    "-replace-",
				Origin: "example.com.",
			}},
		},
		handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Write([]byte("1.2.3.4\n::1"))
		}),
		tc: test.Case{
			Qname: "foo.example.com.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("foo.example.com. 3600	IN	A 1.2.3.4"),
			},
		},
	}, {
		config: HTTPRecord{
			Zones: []Zone{{
				URI:    "-replace-",
				Origin: "example.com.",
			}},
		},
		handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Write([]byte("A 1.2.3.4"))
		}),
		tc: test.Case{
			Qname: "foo.example.com.", Qtype: dns.TypeAFSDB,
			Answer: []dns.RR{},
		},
	}, {
		config: HTTPRecord{
			Zones: []Zone{{
				URI:    "-replace-",
				Origin: "example.com.",
			}},
		},
		handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(404)
		}),
		tc: test.Case{
			Qname: "foo.example.com.", Qtype: dns.TypeA,
			Answer: []dns.RR{},
		},
		shouldErr: true,
	}, {
		config: HTTPRecord{
			Zones: []Zone{{
				URI:    "-replace-",
				Origin: "example.com.",
			}},
		},
		handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Write([]byte("AAAA 1800 ::1"))
		}),
		tc: test.Case{
			Qname: "foo.example.com.", Qtype: dns.TypeAAAA,
			Answer: []dns.RR{
				test.AAAA("foo.example.com. 1800	IN	AAAA ::1"),
			},
		},
	}, {
		config: HTTPRecord{
			Zones: []Zone{{
				URI:    "-replace-",
				Origin: "example.com.",
			}},
		},
		handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Header().Set("Cache-Control", "public, max-age: 1800")
			rw.Write([]byte("AAAA 3600 ::1"))
		}),
		tc: test.Case{
			Qname: "foo.example.com.", Qtype: dns.TypeAAAA,
			Answer: []dns.RR{
				test.AAAA("foo.example.com. 1800	IN	AAAA ::1"),
			},
		},
	}, {
		config: HTTPRecord{
			Zones: []Zone{{
				URI:    "-replace-",
				Origin: "example.com.",
			}},
		},
		handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(503)
		}),
		tc: test.Case{
			Qname: "foo.example.com.", Qtype: dns.TypeA,
			Answer: []dns.RR{},
		},
		shouldErr: true,
	}}

	log.D = true
	for i, c := range tests {
		runTestCase(t, c, i)
	}
}

func runTestCase(t *testing.T, c testCase, testnum int) {
	server := httptest.NewServer(c.handler)
	defer server.Close()

	for i, r := range c.config.Records {
		c.config.Records[i].URI = strings.Replace(r.URI, "-replace-", server.URL, -1)
	}

	for i, z := range c.config.Zones {
		c.config.Zones[i].URI = strings.Replace(z.URI, "-replace-", server.URL, -1)
	}

	ctx := context.TODO()
	m := c.tc.Msg()

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	_, err := c.config.ServeDNS(ctx, rec, m)

	if err != nil && !c.shouldErr {
		t.Errorf("Test %d expected no error, got %v\n", testnum, err)
		return
	} else if err == nil && c.shouldErr {
		t.Errorf("Test %d expected an error but didn't get one\n", testnum)
		return
	}

	if err == nil {
		resp := rec.Msg
		test.SortAndCheck(t, resp, c.tc)
	}
}
