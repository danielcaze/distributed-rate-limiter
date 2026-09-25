// Package httpgateway translates HTTP JSON into calls to the generated gRPC client.
package httpgateway

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	limiterv1 "github.com/danielcaze/distributed-rate-limiter/gen/limiter/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

func New(client limiterv1.LimiterClient, timeout time.Duration, ready func(context.Context) error) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if err := ready(ctx); err != nil {
			http.Error(w, "dependency unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("dependencies ready; admission not implemented\n"))
	})
	mux.HandleFunc("POST /v1/check", func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaErr != nil || mediaType != "application/json" {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		var request limiterv1.CheckRequest
		if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(body, &request); err != nil {
			http.Error(w, "invalid request JSON", http.StatusBadRequest)
			return
		}
		if request.GetKey() == "" {
			http.Error(w, "key is required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		response, err := client.Check(ctx, &request)
		if err != nil {
			writeGRPCError(w, err)
			return
		}
		data, err := (protojson.MarshalOptions{EmitUnpopulated: true}).Marshal(response)
		if err != nil {
			http.Error(w, "invalid admission response", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !response.GetAllowed() {
			w.WriteHeader(http.StatusTooManyRequests)
		}
		_, _ = w.Write(data)
	})
	return mux
}

func writeGRPCError(w http.ResponseWriter, err error) {
	code := status.Code(err)
	if errors.Is(err, context.DeadlineExceeded) {
		code = codes.DeadlineExceeded
	}
	var httpCode int
	switch code {
	case codes.InvalidArgument:
		httpCode = http.StatusBadRequest
	case codes.Unavailable:
		httpCode = http.StatusServiceUnavailable
	case codes.DeadlineExceeded:
		httpCode = http.StatusGatewayTimeout
	case codes.Unimplemented:
		httpCode = http.StatusNotImplemented
	default:
		httpCode = http.StatusInternalServerError
	}
	http.Error(w, code.String(), httpCode)
}
