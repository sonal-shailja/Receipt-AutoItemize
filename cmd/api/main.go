// Command api runs the service: gRPC, REST and internal listeners.
package main

import (
	"context"
	"log"

	"github.com/sonalshailja/receipt-autoitemize/internal/boot"
)

func main() {
	if err := boot.Initialize(context.Background()); err != nil {
		log.Fatalf("service failed: %v", err)
	}
}
