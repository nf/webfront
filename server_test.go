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

package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestServer(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(testHandler))
	defer target.Close()

	redirectLocalTarget := httptest.NewServer(http.HandlerFunc(testRedirectLocalHandler))
	defer redirectLocalTarget.Close()

	redirectGlobalTarget := httptest.NewServer(http.HandlerFunc(testRedirectGlobalHandler))
	defer redirectGlobalTarget.Close()

	ruleFile := writeRules([]*Rule{
		{Host: "example.com", Forward: target.Listener.Addr().String()},
		{Host: "example.org", Serve: "testdata"},
		{Host: "example.localredirect", Forward: redirectLocalTarget.Listener.Addr().String()},
		{Host: "example.globalredirect", Forward: redirectGlobalTarget.Listener.Addr().String()},
	})
	defer os.Remove(ruleFile)

	s, err := NewServer(ruleFile, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	var tests = []struct {
		url  string
		code int
		body string
	}{
		{"http://example.com/", 200, "OK"},
		{"http://foo.example.com/", 200, "OK"},
		{"http://example.org/", 200, "contents of index.html\n"},
		{"http://example.net/", 404, "Not found.\n"},
		{"http://fooexample.com/", 404, "Not found.\n"},
	}

	for _, test := range tests {
		rw := httptest.NewRecorder()
		rw.Body = new(bytes.Buffer)
		req, _ := http.NewRequest("GET", test.url, nil)
		s.ServeHTTP(rw, req)
		if g, w := rw.Code, test.code; g != w {
			t.Errorf("%s: code = %d, want %d", test.url, g, w)
		}
		if g, w := rw.Body.String(), test.body; g != w {
			t.Errorf("%s: body = %q, want %q", test.url, g, w)
		}
	}

	var redirectTests = []struct {
		url      string
		code     int
		location string
	}{
		{"http://example.localredirect/", 302, "https://example.localredirect:443"},
		{"http://example.globalredirect/", 302, "https://global.example.globalredirect"},
	}

	for _, test := range redirectTests {
		rw := httptest.NewRecorder()
		rw.Body = new(bytes.Buffer)
		req, _ := http.NewRequest("GET", test.url, nil)
		s.ServeHTTP(rw, req)
		if g, w := rw.Code, test.code; g != w {
			t.Errorf("%s: code = %d, want %d", test.url, g, w)
		}
		if g, w := rw.Header().Get("Location"), test.location; g != w {
			t.Errorf("%s: location header = %q, want %q", test.url, g, w)
		}
	}
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("OK"))
}

func testRedirectLocalHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "http://a.local.adress", http.StatusFound)
}

func testRedirectGlobalHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://global."+r.Host, http.StatusFound)
}

func writeRules(rules []*Rule) (name string) {
	f, err := ioutil.TempFile("", "webfront-rules")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	err = json.NewEncoder(f).Encode(rules)
	if err != nil {
		panic(err)
	}
	return f.Name()
}
