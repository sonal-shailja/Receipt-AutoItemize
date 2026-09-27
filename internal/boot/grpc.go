package boot

import (
	"google.golang.org/grpc"

	healthv1 "github.com/sonalshailja/receipt-autoitemize/rpc/common/health/v1"
	receiptv1 "github.com/sonalshailja/receipt-autoitemize/rpc/receipt/v1"
)

// registerGRPCServices attaches every service implementation to the gRPC
// server. New services also need a gateway handler registered in http.go.
func registerGRPCServices(server *grpc.Server, deps *Dependencies) {
	healthv1.RegisterHealthServiceServer(server, deps.Health)
	receiptv1.RegisterReceiptServiceServer(server, deps.Receipt)
}
