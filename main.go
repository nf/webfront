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

It reads a JSON-formatted rule file like this:

[
	{"Host": "example.com", "Serve": "/var/www"},
	{"Host": "example.org", "Forward": "localhost:8080"}
]

For all requests to the host example.com (or any name ending in
".example.com") it serves files from the /var/www directory.

For requests to example.org, it forwards the request to the HTTP
server listening on localhost port 8080.

Usage of webfront:
  -http=":80": HTTP listen address
  -https="": HTTPS listen address (leave empty to disable)
  -https_cert="": HTTPS certificate file
  -https_key="": HTTPS key file
  -poll=10s: file poll interval
  -rules="": rule definition file

webfront was written by Andrew Gerrand <adg@golang.org>
*/
package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	httpAddr     = flag.String("http", ":80", "HTTP listen address")
	httpsAddr    = flag.String("https", "", "HTTPS listen address (leave empty to disable)")
	certFile     = flag.String("https_cert", "", "HTTPS certificate file")
	keyFile      = flag.String("https_key", "", "HTTPS key file")
	ruleFile     = flag.String("rules", "", "rule definition file")
	pollInterval = flag.Duration("poll", time.Second*10, "file poll interval")
)

func main() {
	flag.Parse()
	s := NewServer(*ruleFile, *pollInterval)
	httpFD, _ := strconv.Atoi(os.Getenv("RUNSIT_PORTFD_http"))
	httpsFD, _ := strconv.Atoi(os.Getenv("RUNSIT_PORTFD_https"))
	if httpsFD >= 3 || *httpsAddr != "" {
		cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
		if err != nil {
			log.Fatal(err)
		}
		c := &tls.Config{Certificates: []tls.Certificate{cert}}
		l := tls.NewListener(listen(httpsFD, *httpsAddr), c)
		go func() {
			log.Fatal(http.Serve(l, s))
		}()
	}
	log.Fatal(http.Serve(listen(httpFD, *httpAddr), s))
}

func listen(fd int, addr string) net.Listener {
	var l net.Listener
	var err error
	if fd >= 3 {
		l, err = net.FileListener(os.NewFile(uintptr(fd), "http"))
	} else {
		l, err = net.Listen("tcp", addr)
	}
	if err != nil {
		log.Fatal(err)
	}
	return l
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
	for _, r := range rules {
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
	s.mu.Lock()
	s.last = mtime
	s.rules = rules
	s.mu.Unlock()
	return nil
}
