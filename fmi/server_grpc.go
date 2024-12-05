// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package fmi

import (
	"context"
	"fmt"
	"net"
	"time"

	commonv1alpha1 "github.com/ironcore-dev/metal-operator/api/gen/common/v1alpha1"
	"github.com/ironcore-dev/metal-operator/api/gen/serverbios/v1alpha1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

// ServerGRPC implements the ServerBIOS gRPC service.
type ServerGRPC struct {
	port            int
	shutdownTimeout time.Duration
	taskRunner      TaskRunner
}

// NewServerGRPC creates a new ServerGRPC.
func NewServerGRPC(config ServerConfig, insecureBMC bool) (*ServerGRPC, error) {
	taskRunner, err := NewTaskRunner(config.TaskRunnerType, config.KubeClient, insecureBMC)
	if err != nil {
		return nil, err
	}
	return &ServerGRPC{
		port:            config.Port,
		shutdownTimeout: config.ShutdownTimeout,
		taskRunner:      taskRunner,
	}, nil
}

// Start starts the gRPC server.
func (s *ServerGRPC) Start(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return err
	}
	server := grpc.NewServer()
	v1alpha1.RegisterServerBIOSServiceServer(server, s)

	g.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		return s.Shutdown(shutdownCtx, server)
	})

	g.Go(func() error {
		return server.Serve(listener)
	})

	return g.Wait()
}

// Shutdown shuts down the gRPC server.
func (s *ServerGRPC) Shutdown(_ context.Context, server *grpc.Server) error {
	server.GracefulStop()
	// TODO add shutdown logic here: wait for task in progress and etc.
	return nil
}

// BIOSScan implements the BIOSScan gRPC service.
func (s *ServerGRPC) BIOSScan(
	ctx context.Context,
	request *v1alpha1.BIOSScanRequest,
) (*v1alpha1.BIOSScanResponse, error) {
	result, err := s.taskRunner.ExecuteScan(ctx, request.ServerBiosRef)
	if err != nil {
		return nil, err
	}
	return &v1alpha1.BIOSScanResponse{
		Result:   commonv1alpha1.RequestResult_REQUEST_RESULT_SUCCESS,
		Version:  result.Version,
		Settings: result.Settings,
	}, nil
}

// BIOSSettingsApply implements the BIOSSettingsApply gRPC service.
func (s *ServerGRPC) BIOSSettingsApply(
	ctx context.Context,
	request *v1alpha1.BIOSSettingsApplyRequest,
) (*v1alpha1.BIOSSettingsApplyResponse, error) {
	result, err := s.taskRunner.ExecuteSettingsApply(ctx, request.ServerBiosRef)
	if err != nil {
		return nil, err
	}
	return &v1alpha1.BIOSSettingsApplyResponse{
		Result:         commonv1alpha1.RequestResult_REQUEST_RESULT_SUCCESS,
		RebootRequired: result.RebootRequired,
	}, nil
}

// BIOSVersionUpdate implements the BIOSVersionUpdate gRPC service.
func (s *ServerGRPC) BIOSVersionUpdate(
	_ context.Context,
	_ *v1alpha1.BIOSVersionUpdateRequest,
) (*v1alpha1.BIOSVersionUpdateResponse, error) {
	return &v1alpha1.BIOSVersionUpdateResponse{
		Result: commonv1alpha1.RequestResult_REQUEST_RESULT_UNSPECIFIED,
	}, nil
}
