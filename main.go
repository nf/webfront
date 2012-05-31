/*
Copyright 2011 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
webfront is an HTTP reverse-proxy.

It reads a JSON-formatted rule like this:

[
	{"Host": "example.com", "Serve": "/var/www"},
	{"Host": "example.org", "Forward": "localhost:8080"}
]

For all requests to the host example.com (or a host name ending in
".example.com") it serves files from the /var/www directory.

For requests to example.org, it forwards the request to the HTTP
server listening on localhost port 8080.

Usage of webfront:
  -fd=0: file descriptor to listen on
  -http=":80": HTTP listen address
  -poll=10s: file poll interval
  -rules="": rule definition file

webfront was written by Andrew Gerrand <adg@golang.org>
*/
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	fd           = flag.Int("fd", 0, "file descriptor to listen on")
	httpAddr     = flag.String("http", ":80", "HTTP listen address")
	ruleFile     = flag.String("rules", "", "rule definition file")
	pollInterval = flag.Duration("poll", time.Second*10, "file poll interval")
)

func main() {
	flag.Parse()

	var l net.Listener
	var err error
	if *fd >= 3 {
		l, err = net.FileListener(os.NewFile(uintptr(*fd), "http"))
	} else {
		l, err = net.Listen("tcp", *httpAddr)
	}
	if err != nil {
		log.Fatal(err)
	}

	s := NewServer(*ruleFile, *pollInterval)
	log.Fatal(http.Serve(l, s))
}

type Server struct {
	mu    sync.RWMutex
	last  time.Time
	rules []*Rule
}

type Rule struct {
	Host    string
	Forward string
	Serve   string

	proxy http.Handler
}

func NewServer(file string, poll time.Duration) *Server {
	s := new(Server)
	go func() {
		for {
			if err := s.loadRules(file); err != nil {
				log.Fatal(err)
			}
			time.Sleep(poll)
		}
	}()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.mu.RLock()
	h := req.Host
	if i := strings.Index(h, ":"); i >= 0 {
		h = h[:i]
	}
	for _, r := range s.rules {
		if !(h == r.Host || strings.HasSuffix(h, "."+r.Host)) {
			continue
		}
		if p := r.proxy; p != nil {
			s.mu.RUnlock()
			p.ServeHTTP(w, req)
			return
		}
		log.Printf("nil proxy: %#v", r)
		break
	}
	s.mu.RUnlock()
	http.Error(w, "Not found.", http.StatusNotFound)
}

func (s *Server) loadRules(file string) error {
	fi, err := os.Stat(file)
	if err != nil {
		return err
	}
	mtime := fi.ModTime()
	if mtime.Before(s.last) && s.rules != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	var rules []*Rule
	err = json.NewDecoder(f).Decode(&rules)
	if err != nil {
		return err
	}
	s.last = mtime
	s.rules = rules
	for _, r := range s.rules {
		if h := r.Forward; h != "" {
			r.proxy = &httputil.ReverseProxy{
				Director: func(req *http.Request) {
					req.URL.Scheme = "http"
					req.URL.Host = h
				},
			}
		}
		if d := r.Serve; d != "" {
			r.proxy = http.FileServer(http.Dir(d))
		}
	}
	return nil
}
