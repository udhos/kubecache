package main

import (
	"context"
	"io"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

func fetch(c context.Context, client *http.Client, tracer trace.Tracer, method, uri string) ([]byte, int, error) {

	const me = "fetch"
	ctx, span := tracer.Start(c, me)
	defer span.End()

	req, errReq := http.NewRequestWithContext(ctx, method, uri, nil)
	if errReq != nil {
		return nil, 500, errReq
	}

	//req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, errDo := client.Do(req)
	if errDo != nil {
		return nil, 500, errDo
	}
	defer resp.Body.Close()

	body, errBody := io.ReadAll(resp.Body)
	if errBody != nil {
		return nil, 500, errBody
	}

	return body, 200, nil
}
