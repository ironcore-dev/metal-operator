// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	fieldOwner             = client.FieldOwner("metal.ironcore.dev/controller-manager")
	ServerBIOSVersionLabel = "metal.ironcore.dev/server-bios-version"
)

func ConvertBIOSVersionToLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func GetServerSelectorFromSetSelector(serverSelector *metav1.LabelSelector) (labels.Selector, error) {
	if serverSelector.MatchLabels != nil {
		if biosVersion, ok := serverSelector.MatchLabels[ServerBIOSVersionLabel]; ok {
			serverSelector.MatchLabels[ServerBIOSVersionLabel] = ConvertBIOSVersionToLabel(biosVersion)
		}
	}
	return metav1.LabelSelectorAsSelector(serverSelector)
}

// shouldIgnoreReconciliation checks if the object should be ignored during reconciliation.
func shouldIgnoreReconciliation(obj client.Object) bool {
	val, found := obj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return val == metalv1alpha1.OperationAnnotationIgnore
}

// shouldRetryReconciliation checks if the object should retry reconciliation from failed state.
func shouldRetryReconciliation(obj client.Object) bool {
	val, found := obj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return val == metalv1alpha1.OperationAnnotationRetry
}

// GenerateRandomPassword generates a random password of the given length.
func GenerateRandomPassword(length int) ([]byte, error) {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return nil, fmt.Errorf("failed to generate random password: %w", err)
		}
		result[i] = letters[n.Int64()]
	}
	return result, nil
}
