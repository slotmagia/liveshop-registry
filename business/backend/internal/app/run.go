package app

import (
	"context"
	"errors"
	"time"

	"github.com/lvtuopen-ai/kernel-go/logctx"
)

const shutdownTimeout = 10 * time.Second

func Run(ctx context.Context) error {
	current, err := bootstrap(ctx)
	if err != nil {
		return err
	}
	defer current.deps.Close()

	if err := current.httpServer.Start(); err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = current.grpcServer.Stop(stopCtx)
		return err
	}
	grpcErrors := make(chan error, 1)
	go func() {
		grpcErrors <- current.grpcServer.Serve()
	}()
	logctx.FromContext(ctx).Info("registry listening", "address", current.httpAddress)
	logctx.FromContext(ctx).Info("registry gRPC listening", "address", current.grpcServer.Address())

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-grpcErrors:
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return errors.Join(serveErr, current.grpcServer.Stop(stopCtx), current.httpServer.Shutdown())
}
