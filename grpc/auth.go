package grpc

import (
	"context"

	"github.com/signalfx/golib/v3/sfxclient"
	"google.golang.org/grpc/credentials"
)

// SignalFxTokenAuth is a credentials.PerRPCCredentials object that sets an auth token on each gRPC request
// as expected by our ingest service.
type SignalFxTokenAuth struct {
	Token string
	DisableTransportSecurity bool
}

var _ credentials.PerRPCCredentials = new(SignalFxTokenAuth)

// GetRequestMetadata returns the metadata with the auth token
func (a *SignalFxTokenAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		sfxclient.TokenHeaderName: a.Token,
	}, nil
}

// RequireTransportSecurity determines whether TLS is required or not.  This will return `true`
// unless DisableTokenSecurity has been overridden to `true`.
func (a *SignalFxTokenAuth) RequireTransportSecurity() bool {
	return !a.DisableTransportSecurity
}
