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
	"fmt"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/cache"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"hash/fnv"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type HTTPRecord struct {
	Next                plugin.Handler
	Records             []Record
	Zones               []Zone
	Timeout             time.Duration
	MaxTTL              uint32
	ReturnCachedOnError bool
	Cache               *cache.Cache
	Fall                fall.F
}

type Zone struct {
	Origin string
	URI    string
}

type Record struct {
	Name string
	Type string
	URI  string
}

type BackendIndicatedError struct {
	HTTPResponseCode int
	DNSResponseCode  int
}

type cacheItem struct {
	Payload string
	TTL     uint32
}

func (e BackendIndicatedError) Error() string {
	return fmt.Sprintf("dns error: %d from http error %d", e.DNSResponseCode, e.HTTPResponseCode)
}

const MaxHTTPBodySize = 4096

var cacheControlRegex = regexp.MustCompile(`max-age:[\s]*([\d]+)`)
var responseToRR = map[string]func(name string, ttl uint32, response string) ([]dns.RR, error){
	"TXT":  parseTXT,
	"A":    parseA,
	"AAAA": parseAAAA,
}

func (h HTTPRecord) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	log.Debugf("Lookup type %s for %s", state.Type(), state.Name())

	if _, ok := responseToRR[state.Type()]; !ok {
		// As this type is not something we support, there is not going to be a result anyways.
		if h.Fall.Through(state.Name()) {
			return plugin.NextOrFailure(state.Name(), h.Next, ctx, w, r)
		}
		return nodata(w, r)
	}

	// First, let's see if we can find an exact match for the name being queried.
	for _, record := range h.Records {
		if record.Name == state.Name() && record.Type == state.Type() {
			return h.fetchAndWrite(w, r, state.Type(), state.Name(), record.URI)
		}
	}

	// Let's find a zone for this name.
	var origins []string
	for _, zone := range h.Zones {
		origins = append(origins, zone.Origin)
	}
	zone := plugin.Zones(origins).Matches(state.Name())
	if zone != "" {
		log.Debugf("Found matching zone: %s", zone)
		for _, zone := range h.Zones {
			return h.fetchAndWrite(w, r, state.Type(), state.Name(), zone.URI)
		}
	}

	if h.Fall.Through(state.Name()) {
		return plugin.NextOrFailure(state.Name(), h.Next, ctx, w, r)
	}

	// At this point, we don't have anything to return - but we don't know that it is NXDOMAIN as other records might
	// exist. As such, we will do a NODATA response
	return nodata(w, r)
}

func (h HTTPRecord) Name() string {
	return "httprecord"
}

func nodata(w dns.ResponseWriter, r *dns.Msg) (int, error) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable = true, true
	m.Answer = []dns.RR{}

	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

func (h HTTPRecord) fetch(name string, uri string) (string, uint32, error) {
	uri = strings.Replace(uri, "%(fqdn)", name, -1)

	timeout := h.Timeout
	if timeout == 0 {
		// A timeout of 0 means infinite - let's restrict it to avoid having undying HTTP clients.
		timeout = time.Second * 5
	}
	client := &http.Client{
		Timeout: timeout,
	}

	log.Debugf("Fetching: %s with a timeout of %s", uri, timeout)
	response, err := client.Get(uri)
	if err != nil {
		return "", 0, err
	}

	// Deliberately do not read all. A broken upstream could give us a lot of data that we could not return to the
	// client anyways. As such, just read part of it and discard the rest.
	body := make([]byte, MaxHTTPBodySize)
	read, err := response.Body.Read(body)
	if err != nil && err != io.EOF {
		response.Body.Close()
		return "", 0, err
	}
	response.Body.Close()

	if read == MaxHTTPBodySize {
		return string(body), 0, fmt.Errorf("backend returned a body longer than %d bytes", MaxHTTPBodySize-1)
	}

	ttl := h.extractTTL(response.Header)

	switch {
	case response.StatusCode == 200:
		return string(body[:read]), ttl, nil
	case response.StatusCode == 404:
		return "", 0, BackendIndicatedError{
			HTTPResponseCode: response.StatusCode,
			DNSResponseCode:  dns.RcodeNameError}
	case response.StatusCode >= 500:
		return "", 0, BackendIndicatedError{
			HTTPResponseCode: response.StatusCode,
			DNSResponseCode:  dns.RcodeServerFailure}
	default:
		return "", 0, fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}
}

func (h HTTPRecord) maybeFetchCached(name string, uri string) (string, uint32, error) {
	if !h.ReturnCachedOnError {
		return h.fetch(name, uri)
	}

	hasher := fnv.New64()
	hasher.Write([]byte(name))
	hasher.Write([]byte(uri))
	cachekey := hasher.Sum64()

	payload, ttl, err := h.fetch(name, uri)
	if err == nil {
		h.Cache.Add(cachekey, cacheItem{payload, ttl})
		return payload, ttl, err
	}

	if entry, ok := h.Cache.Get(cachekey); ok {
		if item, ok := entry.(cacheItem); ok {
			return item.Payload, item.TTL, nil
		}
	}
	return payload, ttl, err
}

func (h HTTPRecord) extractTTL(hdr http.Header) uint32 {
	var ttl uint32 = 0

	cc := hdr.Get("Cache-Control")
	m := cacheControlRegex.FindStringSubmatch(cc)
	if len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			ttl = uint32(n)
		}
	}
	if cc != "" && ttl == 0 {
		log.Warningf("Unable to parse Cache-Control header: %s", cc)
	}

	switch {
	case ttl > 0 && (h.MaxTTL == 0 || h.MaxTTL > ttl):
		return ttl
	case h.MaxTTL > 0:
		return h.MaxTTL
	default:
		return 3600
	}
}

func (h HTTPRecord) fetchAndWrite(w dns.ResponseWriter, r *dns.Msg, rtype string, name string, uri string) (int, error) {
	payload, ttl, err := h.maybeFetchCached(name, uri)
	if err != nil {
		if bie, ok := err.(BackendIndicatedError); ok {
			return bie.DNSResponseCode, err
		}
		return dns.RcodeServerFailure, err
	}

	parser, ok := responseToRR[rtype]
	if !ok {
		return dns.RcodeServerFailure, fmt.Errorf("unable to find response parser for: %s", rtype)
	}

	rrs, err := parser(name, ttl, payload)
	if err != nil {
		return dns.RcodeServerFailure, err
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable = true, true
	m.Answer = rrs

	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}
