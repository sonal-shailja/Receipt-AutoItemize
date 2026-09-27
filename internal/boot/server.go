package boot

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/textproto"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sonalshailja/receipt-autoitemize/internal/interceptors/logging"
	"github.com/sonalshailja/receipt-autoitemize/internal/interceptors/recovery"
	"github.com/sonalshailja/receipt-autoitemize/internal/interceptors/requestid"
)

// Server runs the two listeners: gRPC, and the grpc-gateway REST proxy in
// front of it. One handler serves both protocols; the gateway translates JSON
// to protobuf and dials the gRPC port.
type Server struct {
	deps *Dependencies

	grpcServer *grpc.Server
	httpServer *http.Server
}

// NewServer constructs the servers without starting them.
func NewServer(ctx context.Context, deps *Dependencies) (*Server, error) {
	grpcServer := newGRPCServer(deps)

	httpServer, err := newGatewayServer(ctx, deps)
	if err != nil {
		return nil, err
	}

	return &Server{
		deps:       deps,
		grpcServer: grpcServer,
		httpServer: httpServer,
	}, nil
}

// newGRPCServer builds the gRPC server and registers every service on it.
// Interceptors run outermost first: recovery catches panics from everything
// inside it, requestid assigns the id logging then reads.
func newGRPCServer(deps *Dependencies) *grpc.Server {
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			recovery.UnaryServerInterceptor(),
			requestid.UnaryServerInterceptor(),
			logging.UnaryServerInterceptor(deps.Logger),
		),
	)

	registerGRPCServices(server, deps)

	// Reflection lets grpcurl discover the API without a copy of the .proto.
	reflection.Register(server)

	return server
}

// newGatewayServer builds the REST listener.
func newGatewayServer(ctx context.Context, deps *Dependencies) (*http.Server, error) {
	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				// snake_case field names, and zero values emitted.
				UseProtoNames:   true,
				EmitUnpopulated: true,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		}),
		runtime.WithIncomingHeaderMatcher(incomingHeaderMatcher),
		runtime.WithErrorHandler(gatewayErrorHandler),
	)

	// The gateway dials the gRPC server over loopback, so no TLS.
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	if err := registerGatewayHandlers(ctx, mux, deps.Config.Server.GRPC, dialOpts, deps); err != nil {
		return nil, fmt.Errorf("boot: registering gateway handlers: %w", err)
	}

	return &http.Server{
		Addr:              deps.Config.Server.HTTP,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}, nil
}

// incomingHeaderMatcher decides which HTTP headers cross into gRPC metadata.
// The default drops x-request-id, which would break correlation.
func incomingHeaderMatcher(key string) (string, bool) {
	if key == textproto.CanonicalMIMEHeaderKey(requestid.HeaderKey) {
		return requestid.HeaderKey, true
	}
	return runtime.DefaultHeaderMatcher(key)
}

// Run starts both listeners and blocks until ctx is cancelled or a
// listener fails.
func (s *Server) Run(ctx context.Context) error {
	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		listener, err := net.Listen("tcp", s.deps.Config.Server.GRPC)
		if err != nil {
			return fmt.Errorf("boot: listening on grpc %s: %w", s.deps.Config.Server.GRPC, err)
		}
		s.deps.Logger.Info("grpc server listening", "address", s.deps.Config.Server.GRPC)
		return s.grpcServer.Serve(listener)
	})

	group.Go(func() error {
		s.deps.Logger.Info("http server listening", "address", s.deps.Config.Server.HTTP)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("boot: http server: %w", err)
		}
		return nil
	})

	// Shut down when the context is cancelled or a listener fails.
	group.Go(func() error {
		<-groupCtx.Done()
		return s.Shutdown()
	})

	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// Shutdown lets in-flight requests finish, then stops. The timeout bounds how
// long a slow request may delay the shutdown.
func (s *Server) Shutdown() error {
	timeout := s.deps.Config.Server.ShutdownTimeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	s.deps.Logger.Info("shutting down", "timeout", timeout.String())

	stopped := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(stopped)
	}()

	var shutdownErr error
	if err := s.httpServer.Shutdown(ctx); err != nil {
		shutdownErr = err
	}

	select {
	case <-stopped:
	case <-ctx.Done():
		s.deps.Logger.Warn("graceful shutdown timed out, forcing stop")
		s.grpcServer.Stop()
	}

	return shutdownErr
}
