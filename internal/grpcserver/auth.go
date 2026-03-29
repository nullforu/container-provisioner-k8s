package grpcserver

import (
	"context"
	"strings"

	"smctf/internal/config"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func APIKeyUnaryInterceptor(cfg config.APIKeyConfig) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !cfg.Enabled {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "api key is required")
		}

		key := firstHeader(md, "x-api-key")
		if key == "" {
			key = firstHeader(md, "api_key")
		}

		if strings.TrimSpace(key) == "" {
			return nil, status.Error(codes.Unauthenticated, "api key is required")
		}

		if key != cfg.Value {
			return nil, status.Error(codes.Unauthenticated, "invalid api key")
		}

		return handler(ctx, req)
	}
}

func firstHeader(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}

	return values[0]
}
