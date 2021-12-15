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
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/cache"
	"github.com/miekg/dns"
	"log"
	"strings"
	"time"
)

func init() { plugin.Register("httprecord", setup) }

func setup(c *caddy.Controller) error {
	httprecord, err := parseConfig(c)
	if err != nil {
		return plugin.Error("httprecord", err)
	}

	log.Printf("Parsed config: %v", httprecord)

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		httprecord.Next = next
		return httprecord
	})

	return nil
}

func parseConfig(c *caddy.Controller) (HTTPRecord, error) {
	var h = HTTPRecord{}

	serverBlockOrigins := make([]string, len(c.ServerBlockKeys))
	for i := range serverBlockOrigins {
		serverBlockOrigins[i] = plugin.Host(c.ServerBlockKeys[i]).NormalizeExact()[0]
	}

	for c.Next() {
		args := c.RemainingArgs()

		if len(args) == 0 {
			// Format: httprecord { block }
			if err := parseConfigBlock(c, &h, serverBlockOrigins, ""); err != nil {
				return h, err
			}
		} else {
			// Format: httprecord [ORIGIN...] [ORIGIN_OR_URI] { block }
			uri := ""

			if strings.ToLower(args[len(args)-1])[:len("http")] == "http" {
				uri = args[len(args)-1]
				args = args[:len(args)-1]
			}

			// The rest of the args now are origins -> normalize them.
			for i, origin := range args {
				args[i] = plugin.Name(origin).Normalize()
			}

			if uri != "" {
				for _, origin := range args {
					h.Zones = append(h.Zones, Zone{
						Origin: origin,
						URI:    uri,
					})
				}
			}

			if len(args) == 0 {
				if err := parseConfigBlock(c, &h, serverBlockOrigins, uri); err != nil {
					return h, err
				}
			} else {
				if err := parseConfigBlock(c, &h, args, uri); err != nil {
					return h, err
				}
			}
		}
	}

	return h, nil
}

func parseConfigBlock(c *caddy.Controller, h *HTTPRecord, origins []string, blockuri string) error {
	for c.NextBlock() {
		switch c.Val() {
		case "onerror":
			args := c.RemainingArgs()

			if len(args) != 1 || (args[0] != "servfail" && args[0] != "cached") {
				return c.Err("unknown value for onerror. Expected one of: servfail, cached")
			}

			h.ReturnCachedOnError = args[0] == "cached"
			if h.ReturnCachedOnError {
				h.Cache = cache.New(100)
			}
		case "timeout":
			args := c.RemainingArgs()

			if len(args) != 1 {
				return c.Err("unknown value for timeout. Expected a duration")
			}

			if timeout, err := time.ParseDuration(args[0]); err != nil {
				return c.Err("unable to parse timeout: " + err.Error())
			} else {
				h.Timeout = timeout
			}
		case "fallthrough":
			h.Fall.SetZonesFromArgs(c.RemainingArgs())
		default:
			rtype := strings.ToUpper(c.Val())
			args := c.RemainingArgs()

			if !isType(rtype) {
				return c.Errf("unknown record type: %s", rtype)
			}

			if len(args) == 2 || (len(args) == 1 && blockuri != "") {
				name := strings.ToLower(args[0])

				uri := blockuri
				if len(args) == 2 {
					uri = args[1]
				}

				if dns.IsFqdn(name) {
					h.Records = append(h.Records, Record{
						Type: rtype,
						Name: name,
						URI:  uri,
					})
				} else {
					for _, origin := range origins {
						h.Records = append(h.Records, Record{
							Type: rtype,
							Name: name + "." + origin,
							URI:  uri,
						})
					}
				}
			} else {
				return c.ArgErr()
			}
		}
	}

	return nil
}
