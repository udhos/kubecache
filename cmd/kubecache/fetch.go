package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
)

func doFetch(c context.Context, tracer trace.Tracer, httpClient *http.Client,
	backendURL *url.URL, key string) (response, bool, error) {

	const me = "doFetch"
	ctx, span := tracer.Start(c, me)
	defer span.End()

	resp := response{Header: http.Header{}}
	var isErrorStatus bool

	method, u, errKey := parseKey(me, backendURL, key)
	if errKey != nil {
		return resp, isErrorStatus, errKey
	}

	begin := time.Now()

	body, respHeaders, status, errFetch := fetch(ctx, httpClient, tracer,
		method, u)

	elap := time.Since(begin)

	isErrorStatus = isHTTPError(status)

	//
	// log fetch status
	//
	traceID := span.SpanContext().TraceID().String()
	if errFetch == nil {
		if isErrorStatus {
			//
			// http error
			//
			bodyStr := string(body)
			log.Error().Str("traceID", traceID).Str("method", method).Str("url", u).Int("response_status", status).Dur("elapsed", elap).Str("response_body", bodyStr).Msgf("getter: traceID=%s method=%s url=%s response_status=%d elapsed=%v response_body:%s", traceID, method, u, status, elap, bodyStr)
		} else {
			//
			// http success
			//
			log.Debug().Str("traceID", traceID).Str("method", method).Str("url", u).Int("response_status", status).Dur("elapsed", elap).Msgf("getter: traceID=%s method=%s url=%s response_status=%d elapsed=%v", traceID, method, u, status, elap)
		}
	} else {
		log.Error().Str("traceID", traceID).Str("method", method).Str("url", u).Int("response_status", status).Str("response_error", errFetch.Error()).Dur("elapsed", elap).Msgf("getter: traceID=%s method=%s url=%s response_status=%d elapsed=%v response_error:%v", traceID, method, u, status, elap, errFetch)
	}

	span.SetAttributes(
		traceMethod.String(method),
		traceURI.String(u),
		traceResponseStatus.Int(resp.Status),
		traceElapsed.String(elap.String()),
	)
	if errFetch != nil {
		span.SetAttributes(traceResponseError.String(errFetch.Error()))
	}

	if errFetch != nil {
		return resp, isErrorStatus, errFetch
	}

	resp = response{
		Body:   body,
		Status: status,
		Header: respHeaders,
	}

	return resp, isErrorStatus, nil
}

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
