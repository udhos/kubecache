package main

import (
	"context"
	"io"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

func fetch(c context.Context, client *http.Client, tracer trace.Tracer,
	method, uri string) ([]byte, http.Header, int, error) {

	const me = "fetch"
	ctx, span := tracer.Start(c, me)
	defer span.End()

	req, errReq := http.NewRequestWithContext(ctx, method, uri, nil)
	if errReq != nil {
		return nil, nil, 500, errReq
	}

	//req.Header.Add("key", "value")

	resp, errDo := client.Do(req)
	if errDo != nil {
		return nil, nil, 500, errDo
	}
	defer resp.Body.Close()

	reqBody, errBody := io.ReadAll(resp.Body)
	if errBody != nil {
		return nil, nil, 500, errBody
	}

	return reqBody, resp.Header, resp.StatusCode, nil
}
