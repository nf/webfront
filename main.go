package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
)

var (
	httpAddr = flag.String("http", ":80", "http listen address")
	ruleFile = flag.String("rules", "", "file that contains the rule definitions")
)

func main() {
	flag.Parse()
	s := new(Server)
	if err := s.loadRules(); err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe(*httpAddr, s))
}

type Server struct {
	rules []Rule
}

type Rule struct {
	Host    string
	Forward string
	Static  string

	proxy http.Handler
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
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

func (s *Server) loadRules() error {
	f, err := os.Open(*ruleFile)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(&s.rules)
}
