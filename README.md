# httprecord

## Name

*httprecord* - enables serving records based on HTTP results.

## Description

The *httprecord* plugin allows you to configure individual records or entire zones to be served based on the responses
to HTTP(S) requests. The primary use case is to serve dynamic record data without touching your CoreDNS server and
instead fetching those from a simple HTTP endpoint.

## Expected HTTP response format

The HTTP endpoint is expected to respond to a GET request with the following format:

~~~
[TYPE [TTL]] DATA
[[TYPE [TTL]] DATA ...]
~~~

* **TYPE** An optional record type for this line. Currently only TXT, A and AAAA are supported.
* **TTL** An optional TTL to override the TTL for this particular line.
* **DATA** The record's data. The format depends on the type of record.

for example, to return a set of A and AAAA records, a response with explicit types could look like:

~~~
A 1.2.3.4
A 1.2.3.5
AAAA ::1
~~~

or without them:

~~~
1.2.3.4
1.2.3.5
::1
~~~

if types are left off, each line will be used in a response if it makes sense for the current context. A dotted IPv4
address would for example only be returned for TXT and A records but not for AAAA.

## Syntax

~~~
httprecord [ORIGIN...] [URI_OR_ORIGIN] {
    [[TYPE NAME [URI]]...]
    fallthrough [ZONES...]
}
~~~

* **ORIGIN** An origin to match for.
* **URI_OR_ORIGIN** The last parameter can either be an origin or a URI to make lookups against.
* **TYPE** The type of an individual record in the block.
* **NAME** The name of an individual record in the block. This can be both absolute or relative. A relative name will
  be expanded to all origins of the config directive.
* **URI** The URI to perform the lookup against for the record. If none is given, **URI_OR_ORIGIN** will be used.
* **ZONES** Zones to perform fallthrough for: Requests for these will go to the next plugin if necessary.

## Examples

Respond to A requests on foo.example.com. with the IP address stored at https://example.com/foo.txt

~~~ corefile
. {
    httprecord {
        A foo.example.com. https://example.com/foo.txt
    }
}
~~~

Respond to A requests on foo.example.com. and foo.example.org. with the IP address stored at https://example.com/foo.txt

~~~ corefile
. {
    httprecord example.com example.org {
        A foo https://example.com/foo.txt
    }
}
~~~

Respond to all requests on *.example.com. with the contents at https://example.com/**FQDN**txt. For example, a lookup
on foo.example.com. will be looked-up at https://example.com/**foo.example.com.**txt. Also respond to the individual
record bar.example.org. by performing the same lookup.

~~~ corefile
. {
    httprecord example.com. https://example.com/%{fqdn}.txt {
        TXT bar.example.org.
    }
}
~~~
