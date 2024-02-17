package main

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

func fetch(c context.Context, client *http.Client, tracer trace.Tracer,
	method, uri string, reqBody []byte, h http.Header) ([]byte, int, error) {

	const me = "fetch"
	ctx, span := tracer.Start(c, me)
	defer span.End()

	req, errReq := http.NewRequestWithContext(ctx, method, uri,
		bytes.NewBuffer(reqBody))
	if errReq != nil {
		return nil, 500, errReq
	}

	// copy headers
	for k, v := range h {
		for _, vv := range v {
			req.Header.Add(k, vv)
		}
	}

	resp, errDo := client.Do(req)
	if errDo != nil {
		return nil, 500, errDo
	}
	defer resp.Body.Close()

	reqBody, errBody := io.ReadAll(resp.Body)
	if errBody != nil {
		return nil, 500, errBody
	}

	return reqBody, 200, nil
}
