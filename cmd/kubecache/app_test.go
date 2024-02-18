package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestApp(t *testing.T) {

	expected := "hello"

	const slowServerDelay = 100 * time.Millisecond
	const fastResponse = 5 * time.Millisecond

	var serverHits int

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHits++
		time.Sleep(slowServerDelay)
		t.Logf("server replying: %s", expected)
		fmt.Fprint(w, expected)
	}))
	defer s.Close()

	os.Setenv("BACKEND_URL", s.URL)

	app := newApplication("test")
	defer app.stop()
	go app.run()

	time.Sleep(100 * time.Millisecond) // give time for the application to start

	u := "http://localhost:9000/test"

	if serverHits != 0 {
		t.Errorf("non-zero server hits: %d", serverHits)
	}

	// hit server
	{
		elap, err := query(expected, u)
		if err != nil {
			t.Errorf(err.Error())
		}
		if elap < slowServerDelay {
			t.Errorf("response too fast for server %v < %v", elap, slowServerDelay)
		}
	}
	if serverHits != 1 {
		t.Errorf("non-unitary server hits: %d", serverHits)
	}

	// hit cache
	{
		elap, err := query(expected, u)
		if err != nil {
			t.Errorf(err.Error())
		}
		if elap >= slowServerDelay {
			t.Errorf("response too slow for server %v > %v", elap, fastResponse)
		}
	}
	if serverHits != 1 {
		t.Errorf("non-unitary server hits: %d", serverHits)
	}

	// hit cache AGAIN
	{
		elap, err := query(expected, u)
		if err != nil {
			t.Errorf(err.Error())
		}
		if elap >= slowServerDelay {
			t.Errorf("response too slow for server %v > %v", elap, fastResponse)
		}
	}
	if serverHits != 1 {
		t.Errorf("non-unitary server hits: %d", serverHits)
	}
}

func query(expected, u string) (time.Duration, error) {
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
		return elap, fmt.Errorf("expected='%s' got='%s'", expected, str)
	}

	return elap, nil
}
