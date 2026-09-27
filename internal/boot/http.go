package boot

import (
	"context"
	"fmt"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"

	receiptsvc "github.com/sonalshailja/receipt-autoitemize/internal/services/receipt"
	healthv1 "github.com/sonalshailja/receipt-autoitemize/rpc/common/health/v1"
	receiptv1 "github.com/sonalshailja/receipt-autoitemize/rpc/receipt/v1"
)

// registerGatewayHandlers wires the REST routes: the generated translators for
// every gRPC service, plus the multipart upload the gateway cannot express.
func registerGatewayHandlers(
	ctx context.Context,
	mux *runtime.ServeMux,
	grpcAddress string,
	dialOpts []grpc.DialOption,
	deps *Dependencies,
) error {
	if err := healthv1.RegisterHealthServiceHandlerFromEndpoint(ctx, mux, grpcAddress, dialOpts); err != nil {
		return fmt.Errorf("health service: %w", err)
	}
	if err := receiptv1.RegisterReceiptServiceHandlerFromEndpoint(ctx, mux, grpcAddress, dialOpts); err != nil {
		return fmt.Errorf("receipt service: %w", err)
	}
	upload := receiptsvc.UploadHandler(deps.ReceiptService, deps.Config.Storage.MaxUploadBytes)
	if err := mux.HandlePath(http.MethodPost, "/receipts", upload); err != nil {
		return fmt.Errorf("receipt upload: %w", err)
	}
	return nil
}
