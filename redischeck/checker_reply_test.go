package redischeck

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestParseReply_RejectsContractViolations(t *testing.T) {
	cases := []struct {
		name  string
		reply interface{}
	}{
		{"negative remaining", []interface{}{int64(1), int64(-1), int64(0), int64(0)}},
		{"allowed different from 0 or 1", []interface{}{int64(2), int64(0), int64(0), int64(0)}},
		{"allowed with retry after", []interface{}{int64(1), int64(0), int64(0), int64(100)}},
		{"denied with negative retry after", []interface{}{int64(0), int64(0), int64(0), int64(-100)}},
		{"invalid reply type", "oops"},
		{"incomplete reply", []interface{}{int64(0), int64(0), int64(0)}},
		{"allowed with wrong type", []interface{}{"1", int64(0), int64(0), int64(0)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseReply(tc.reply)
			if err == nil {
				t.Errorf("parseReply(%v) = nil, want error", tc.reply)
			} else if status.Code(err) != codes.Internal {
				t.Errorf("parseReply(%v) = %v, want error code %v", tc.reply, err, codes.Internal)
			}

		})
	}
}
