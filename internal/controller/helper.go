// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"slices"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	fieldOwner = client.FieldOwner("metal.ironcore.dev/controller-manager")
)

type BMCTaskFetchFailedError struct {
	TaskURI  string
	Resource string
	Err      error
}

func (e BMCTaskFetchFailedError) Error() string {
	return e.Err.Error()
}

type MultiErrorTracker struct {
	Identifier string
	Err        error
}

func (e MultiErrorTracker) Error() string {
	return e.Err.Error()
}

// GetServerMaintenanceForObjectReference returns a ServerMaintenance object for a given reference.
func GetServerMaintenanceForObjectReference(ctx context.Context, c client.Client, ref *metalv1alpha1.ObjectReference) (*metalv1alpha1.ServerMaintenance, error) {
	if ref == nil {
		return nil, fmt.Errorf("got nil reference")
	}
	maintenance := &metalv1alpha1.ServerMaintenance{}
	if err := c.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, maintenance); err != nil {
		return nil, fmt.Errorf("failed to get ServerMaintenance: %w", err)
	}

	return maintenance, nil
}

// GetServerByName returns a Server object by its name or an error in case the object can not be found.
func GetServerByName(ctx context.Context, c client.Client, serverName string) (*metalv1alpha1.Server, error) {
	server := &metalv1alpha1.Server{}
	if err := c.Get(ctx, client.ObjectKey{Name: serverName}, server); err != nil {
		return nil, err
	}
	return server, nil
}

// shouldIgnoreReconciliation checks if the object should be ignored during reconciliation.
func shouldIgnoreReconciliation(obj client.Object) bool {
	val, found := obj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return slices.Contains([]string{
		metalv1alpha1.OperationAnnotationIgnore,
	}, val)
}

func isServerParked(server *metalv1alpha1.Server) bool {
	_, ok := server.GetAnnotations()[metalv1alpha1.ParkedAnnotation]
	return ok
}

func isServerParkingOrParked(server *metalv1alpha1.Server) bool {
	if isServerParked(server) {
		return true
	}
	return server.GetAnnotations()[metalv1alpha1.OperationAnnotation] == metalv1alpha1.OperationAnnotationPark
}

func isResetAnnotation(operation string) bool {
	_, ok := metalv1alpha1.AnnotationToRedfishMapping[operation]
	return ok
}

func isParkableState(state metalv1alpha1.ServerState) bool {
	switch state {
	case metalv1alpha1.ServerStateAvailable, metalv1alpha1.ServerStateReserved, metalv1alpha1.ServerStateDiscovery:
		return true
	}
	return false
}

// GenerateRandomPassword generates a random password of the given length.
func GenerateRandomPassword(length int) ([]byte, error) {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range length {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return nil, fmt.Errorf("failed to generate random password: %w", err)
		}
		result[i] = letters[n.Int64()]
	}
	return result, nil
}
