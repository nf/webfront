package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	httpAddr     = flag.String("http", ":80", "http listen address")
	ruleFile     = flag.String("rules", "", "file that contains the rule definitions")
	pollInterval = flag.Duration("poll", time.Second*10, "rule file poll interval")
)

func main() {
	flag.Parse()
	s := NewServer(*ruleFile, *pollInterval)
	log.Fatal(http.ListenAndServe(*httpAddr, s))
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
		h := req.Header.Get("Host")
		if !(h == r.Host || strings.HasPrefix(h, "."+r.Host)) {
			continue
		}
		if h := r.Forward; h != "" && r.proxy == nil {
			dir := func(req *http.Request) {
				req.URL.Host = h
			}
			r.proxy = &httputil.ReverseProxy{Director: dir}
		}
		if d := r.Static; d != "" && r.proxy == nil {
			r.proxy = http.FileServer(http.Dir(d))
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
	err = json.NewDecoder(f).Decode(&s.rules)
	if err != nil {
		return err
	}
	s.last = mtime
	return nil
}
