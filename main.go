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
	httpAddr     = flag.String("http", ":80", "http listen address")
	ruleFile     = flag.String("rules", "", "file that contains the rule definitions")
	pollInterval = flag.Duration("poll", time.Second*10, "rule file poll interval")
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
	rules []Rule
}

type Rule struct {
	Host    string
	Forward string
	Static  string

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
	defer s.mu.RUnlock()
	for _, r := range s.rules {
		if !(req.Host == r.Host || strings.HasSuffix(req.Host, "."+r.Host)) {
			continue
		}
		if h := r.Forward; h != "" && r.proxy == nil {
			dir := func(req *http.Request) {
				req.URL.Scheme = "http"
				req.URL.Host = h
			}
			s.mu.RUnlock()
			s.mu.Lock()
			r.proxy = &httputil.ReverseProxy{Director: dir}
			s.mu.Unlock()
			s.mu.RLock()
		}
		if d := r.Static; d != "" && r.proxy == nil {
			s.mu.RUnlock()
			s.mu.Lock()
			r.proxy = http.FileServer(http.Dir(d))
			s.mu.Unlock()
			s.mu.RLock()
		}
		if r.proxy != nil {
			r.proxy.ServeHTTP(w, req)
			return
		}
	}
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
	var rules []Rule
	err = json.NewDecoder(f).Decode(&rules)
	if err != nil {
		return err
	}
	s.last = mtime
	s.rules = rules
	return nil
}
