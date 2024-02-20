package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestApp(t *testing.T) {
	base := "http://localhost:8080"

	test2(t, "1", base+"/test/123")
	test2(t, "2", base+"/test/123?a=b")
	test2(t, "3", base+"/test/123 456")
	test2(t, "4", base+"/test/123?a=b&c=d")
}

func query(name, expected, u string) (time.Duration, error) {
	var elap time.Duration

	begin := time.Now()

	resp, errGet := http.Get(u)
	if errGet != nil {
		return elap, errGet
	}

	elap = time.Since(begin)

	body, errBody := io.ReadAll(resp.Body)
	if errBody != nil {
		return elap, errBody
	}

	str := string(body)

	if str != expected {
		return elap, fmt.Errorf("%s: %s: expected='%s' got='%s'", name, u, expected, str)
	}

	return elap, nil
}

func test2(t *testing.T, name, full string) {

	expected := "hello"

	const slowServerDelay = 100 * time.Millisecond
	const fastResponse = 5 * time.Millisecond

	var serverHits int

	var expectedURL string

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHits++
		time.Sleep(slowServerDelay)
		t.Logf("%s: server url=%s path=%s", name, r.URL, r.URL.Path)
		for k, v := range r.URL.Query() {
			for _, vv := range v {
				t.Logf("%s: server query: '%s'='%s'", name, k, vv)
			}
		}
		t.Logf("%s: server replying: %s", name, expected)
		if r.URL.String() != expectedURL {
			t.Errorf("%s: server: url: got=%s expected=%s", name, r.URL, expectedURL)
			return
		}
		fmt.Fprint(w, expected)
	}))
	defer s.Close()

	os.Setenv("BACKEND_URL", s.URL)

	app := newApplication("test")
	defer app.stop()
	go app.run()

	time.Sleep(100 * time.Millisecond) // give time for the application to start

	uu, errParse := url.Parse(full)
	if errParse != nil {
		t.Errorf("parse: %v", errParse)
		return
	}

	u := uu.String()

	t.Logf("%s: client: %s", name, u)

	uu.Scheme = ""
	uu.Host = ""

	expectedURL = uu.String()

	if serverHits != 0 {
		t.Errorf("non-zero server hits: %d", serverHits)
		return
	}

	// hit server
	{
		elap, err := query(name, expected, u)
		if err != nil {
			t.Errorf(err.Error())
			return
		}
		if elap < slowServerDelay {
			t.Errorf("%s: response too fast for server %v < %v", name, elap, slowServerDelay)
			return
		}
	}
	if serverHits != 1 {
		t.Errorf("%s: 1st: non-unitary server hits: %d", name, serverHits)
		return
	}

	// hit cache
	{
		elap, err := query(name, expected, u)
		if err != nil {
			t.Errorf(err.Error())
			return
		}
		if elap >= fastResponse {
			t.Errorf("response too slow for cache %v > %v", elap, fastResponse)
			return
		}
	}
	if serverHits != 1 {
		t.Errorf("%s: 2nd: non-unitary server hits: %d", name, serverHits)
		return
	}

	// hit cache AGAIN
	{
		elap, err := query(name, expected, u)
		if err != nil {
			t.Errorf(err.Error())
			return
		}
		if elap >= fastResponse {
			t.Errorf("response too slow for cache %v > %v", elap, fastResponse)
			return
		}
	}
	if serverHits != 1 {
		t.Errorf("%s: 3rd: non-unitary server hits: %d", name, serverHits)
		return
	}
}
