package internal

import (
	"bytes"
	"net/http"

	"github.com/anyinone/go-zero/rest/httpx"
	"github.com/anyinone/go-zero/rest/pathvar"
	"github.com/anyinone/jsoniter"
	"github.com/fullstorydev/grpcurl"
	"github.com/golang/protobuf/jsonpb"
)

// NewRequestParser creates a new request parser from the given http.Request and resolver.
func NewRequestParser(r *http.Request, resolver jsonpb.AnyResolver) (grpcurl.RequestParser, error) {
	vars := pathvar.Vars(r)
	params, err := httpx.GetFormValues(r)
	if err != nil {
		return nil, err
	}

	for k, v := range vars {
		params[k] = v
	}
	if len(params) == 0 {
		return grpcurl.NewJSONRequestParser(r.Body, resolver), nil
	}

	if r.ContentLength == 0 {
		return buildJsonRequestParser(params, resolver)
	}

	m := make(map[string]interface{})
	if err := jsoniter.NewDecoder(r.Body).Decode(&m); err != nil {
		return nil, err
	}

	for k, v := range params {
		m[k] = v
	}

	return buildJsonRequestParser(m, resolver)
}

func buildJsonRequestParser(m map[string]interface{}, resolver jsonpb.AnyResolver) (
	grpcurl.RequestParser, error) {
	var buf bytes.Buffer
	if err := jsoniter.NewEncoder(&buf).Encode(m); err != nil {
		return nil, err
	}

	return grpcurl.NewJSONRequestParser(&buf, resolver), nil
}
