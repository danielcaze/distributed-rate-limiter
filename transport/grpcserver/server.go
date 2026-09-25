package grpcserver

import (
	"context"
	"errors"

	limiterv1 "github.com/danielcaze/distributed-rate-limiter/gen/limiter/v1"
	"github.com/danielcaze/distributed-rate-limiter/limiter"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server accepts an injectable Checker. Production supplies the Redis adapter.
type Server struct {
	limiterv1.UnimplementedLimiterServer
	Checker limiter.Checker
}

func (s Server) Check(ctx context.Context, request *limiterv1.CheckRequest) (*limiterv1.CheckResponse, error) {
	if request == nil || request.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	if s.Checker == nil {
		return nil, status.Error(codes.Unimplemented, "admission is not implemented")
	}
	decision, err := s.Checker.Check(ctx, request.GetKey())
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "admission deadline exceeded")
		}
		if errors.Is(err, context.Canceled) {
			return nil, status.Error(codes.Canceled, "admission canceled")
		}
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, status.Error(codes.Internal, "admission failed")
	}
	if decision.ResetAt.IsZero() || decision.RetryAfter < 0 {
		return nil, status.Error(codes.Internal, "invalid admission decision")
	}
	resetAt := timestamppb.New(decision.ResetAt.UTC())
	retryAfter := durationpb.New(decision.RetryAfter)
	if err := resetAt.CheckValid(); err != nil {
		return nil, status.Error(codes.Internal, "invalid reset time")
	}
	if err := retryAfter.CheckValid(); err != nil {
		return nil, status.Error(codes.Internal, "invalid retry interval")
	}
	return &limiterv1.CheckResponse{Allowed: decision.Allowed, Remaining: decision.Remaining, ResetAt: resetAt, RetryAfter: retryAfter}, nil
}
