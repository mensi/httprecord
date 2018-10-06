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
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/mholt/caddy"
	"reflect"
	"testing"
)

func TestHTTPRecordParse(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
		output    HTTPRecord
	}{
		{
			`httprecord {
				A example.com. https://example.com
				fallthrough
			}`,
			false,
			HTTPRecord{
				Records: []Record{{
					Type: "A",
					Name: "example.com.",
					URI:  "https://example.com",
				}},
				Fall: fall.Root,
			},
		},
		{
			`httprecord {
				A example.com.
			}`,
			true, // Because we do not have a URI anywhere.
			HTTPRecord{},
		},
		{
			`httprecord http://example.com {
				URGBLAH
			}`,
			true, // Because URGBLAH is not a valid record type.
			HTTPRecord{},
		},
		{
			`httprecord example.com {
				A example.com.
			}`,
			true, // Because we do not have a URI anywhere.
			HTTPRecord{},
		},
		{
			`httprecord example.com example.org {
				A relative https://example.com
			}`,
			false,
			HTTPRecord{
				Records: []Record{{
					Type: "A",
					Name: "relative.example.com.",
					URI:  "https://example.com",
				}, {
					Type: "A",
					Name: "relative.example.org.",
					URI:  "https://example.com",
				}},
			},
		},
		{
			`httprecord example.com https://example.com`,
			false,
			HTTPRecord{
				Zones: []Zone{{
					Origin: "example.com.",
					URI:    "https://example.com",
				}},
			},
		},
		{
			`httprecord example.com example.org https://example.com {
				A relative
			}`,
			false,
			HTTPRecord{
				Zones: []Zone{{
					Origin: "example.com.",
					URI:    "https://example.com",
				}, {
					Origin: "example.org.",
					URI:    "https://example.com",
				}},
				Records: []Record{{
					Type: "A",
					Name: "relative.example.com.",
					URI:  "https://example.com",
				}, {
					Type: "A",
					Name: "relative.example.org.",
					URI:  "https://example.com",
				}},
			},
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		h, err := parseConfig(c)

		if err == nil && test.shouldErr {
			t.Fatalf("Test %d expected errors, but got no error", i)
		} else if err != nil && !test.shouldErr {
			t.Fatalf("Test %d expected no errors, but got '%v'", i, err)
		} else if !reflect.DeepEqual(h, test.output) {
			t.Fatalf("Test %d expected %v, got %v", i, test.output, h)
		}
	}
}
