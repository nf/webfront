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

func TestParseRules(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(testHandler))
	defer target.Close()

	ruleFile, err := writeRules([]*Rule{
		{Host: "example.com", Forward: target.Listener.Addr().String()},
		{Host: "example.org", Serve: "testdata"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(ruleFile)

	s, err := NewServer(ruleFile, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	testRequest(t, s, "http://example.com/", "OK")
	testRequest(t, s, "http://example.org/", "contents of index.html\n")
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("OK"))
}

func testRequest(t *testing.T, h http.Handler, url string, wantBody string) {
	rw := httptest.NewRecorder()
	rw.Body = new(bytes.Buffer)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.ServeHTTP(rw, req)
	if g, w := rw.Code, http.StatusOK; g != w {
		t.Errorf("GET %s StatusCode = %d, want %d", url, g, w)
	}
	if g, w := rw.Body.String(), wantBody; g != w {
		t.Errorf("GET %s Body = %q, want %q", url, g, w)
	}
}

func writeRules(rules []*Rule) (name string, err error) {
	f, err := ioutil.TempFile("", "webfront-rules")
	if err != nil {
		return
	}
	defer f.Close()
	err = json.NewEncoder(f).Encode(rules)
	if err != nil {
		return
	}
	return f.Name(), nil
}
