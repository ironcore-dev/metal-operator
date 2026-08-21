// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"crypto/rand"
	"fmt"
	"math/big"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	fieldOwner = client.FieldOwner("metal.ironcore.dev/controller-manager")
)

// shouldIgnoreReconciliation checks if the object should be ignored during reconciliation.
func shouldIgnoreReconciliation(obj client.Object) bool {
	op, found := obj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return op == metalv1alpha1.OperationAnnotationIgnore
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
	case metalv1alpha1.ServerStateAvailable, metalv1alpha1.ServerStateReserved:
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
