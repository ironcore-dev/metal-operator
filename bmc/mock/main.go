// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/ironcore-dev/metal-operator/bmc/mock/server"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := zap.Options{
		Development: true,
	}

	log := ctrl.Log.WithName("RedfishMockServer")
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	srv := server.NewMockServer(log, ":8000")

	if err := srv.Start(ctx); err != nil {
		log.Error(err, "Failed to start mock server")
		return
	}

	log.Info("Mock server stopped")
}
