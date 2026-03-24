// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// bmcRefField is the field indexer key for looking up servers by BMC reference
	bmcRefField = "spec.bmcRef.name"

	// CriticalEventConditionType is the condition type for critical events
	CriticalEventConditionType = "CriticalEventReceived"

	// taintKeyPrefix is the prefix for critical event taint keys (used in commented code until PR #672)
	taintKeyPrefix = "metal.ironcore.dev/critical-event-" //nolint:unused

	// maxTaintKeyLength is the maximum allowed length for a Kubernetes taint key (used in commented code until PR #672)
	maxTaintKeyLength = 63 //nolint:unused
)

var (
	// invalidTaintKeyCharsRE matches characters not allowed in taint keys (used in commented code until PR #672)
	invalidTaintKeyCharsRE = regexp.MustCompile(`[^A-Za-z0-9_.-]`) //nolint:unused

	// multiDashRE matches sequences of multiple dashes (used in commented code until PR #672)
	multiDashRE = regexp.MustCompile(`-+`) //nolint:unused

	// Blank identifier to satisfy unused import check until PR #672 is merged
	_ = corev1.Taint{}
)

// sanitizeEventID normalizes an event ID to be valid as part of a Kubernetes taint key.
// It replaces invalid characters with dashes, collapses multiple dashes, trims non-alphanumerics,
// and truncates to ensure the full key (with prefix) doesn't exceed maxTaintKeyLength.
// This function will be used when PR #672 is merged and the taint code is uncommented.
func sanitizeEventID(eventID string) string { //nolint:unused
	// Replace invalid characters with dashes
	sanitized := invalidTaintKeyCharsRE.ReplaceAllString(eventID, "-")

	// Collapse multiple dashes into one
	sanitized = multiDashRE.ReplaceAllString(sanitized, "-")

	// Trim leading/trailing non-alphanumeric characters
	sanitized = strings.Trim(sanitized, "-._")

	// Calculate max suffix length to keep total key <= maxTaintKeyLength
	maxSuffixLen := maxTaintKeyLength - len(taintKeyPrefix)
	if len(sanitized) > maxSuffixLen {
		sanitized = sanitized[:maxSuffixLen]
		// Trim any trailing non-alphanumeric after truncation
		sanitized = strings.TrimRight(sanitized, "-._")
	}

	return sanitized
}

// CreateCriticalEventHandler creates a handler that taints servers when critical events are received
func CreateCriticalEventHandler(k8sClient client.Client, log logr.Logger) CriticalEventHandler {
	return func(ctx context.Context, bmcName string, event Event) {
		log.Info("Handling critical event for server tainting",
			"bmcName", bmcName,
			"eventID", event.EventID,
			"component", event.OriginOfCondition,
			"message", event.Message,
			"timestamp", event.EventTimestamp)

		// List all servers associated with this BMC
		serverList := &metalv1alpha1.ServerList{}
		if err := k8sClient.List(ctx, serverList, client.MatchingFields{bmcRefField: bmcName}); err != nil {
			log.Error(err, "Failed to list servers for BMC", "bmcName", bmcName)
			return
		}

		if len(serverList.Items) == 0 {
			log.Info("No servers found for BMC", "bmcName", bmcName)
			return
		}

		// Taint each server associated with the BMC
		for i := range serverList.Items {
			server := &serverList.Items[i]
			if err := taintServer(ctx, k8sClient, server, event); err != nil {
				log.Error(err, "Failed to taint server", "server", server.Name, "bmcName", bmcName)
				continue
			}
			log.Info("Successfully tainted server", "server", server.Name, "bmcName", bmcName)
		}
	}
}

// taintServer adds a taint to the server based on the critical event
// This implementation uses two approaches:
// 1. Adds a Kubernetes condition to mark the critical event (works immediately)
// 2. Adds a taint to ServerSpec.Taints (requires PR #672 to be merged)
func taintServer(ctx context.Context, k8sClient client.Client, server *metalv1alpha1.Server, event Event) error {
	log := ctrl.LoggerFrom(ctx)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(server), server); err != nil {
		return fmt.Errorf("failed to re-fetch server before patching: %w", err)
	}
	serverBase := server.DeepCopy()
	criticalEventCondition := metav1.Condition{
		Type:               CriticalEventConditionType,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: server.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "CriticalEventReceived",
		Message:            fmt.Sprintf("Critical Redfish event received: %s (Component: %s)", event.Message, event.OriginOfCondition),
	}
	apimeta.SetStatusCondition(&server.Status.Conditions, criticalEventCondition)

	if err := k8sClient.Status().Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch server status with critical event condition: %w", err)
	}

	log.Info("Added critical event condition to server",
		"server", server.Name,
		"eventID", event.EventID,
		"component", event.OriginOfCondition,
		"eventMessage", event.Message)

	// Uncomment the following code after PR #672 is merged:
	/*
		serverBase = server.DeepCopy()
		sanitizedEventID := sanitizeEventID(event.EventID)
		taintKey := taintKeyPrefix + sanitizedEventID
		taint := corev1.Taint{
			Key:    taintKey,
			Value:  event.OriginOfCondition,
			Effect: "NoClaim",
		}

		taintExists := false
		for _, existingTaint := range server.Spec.Taints {
			if existingTaint.Key == taint.Key && existingTaint.Effect == taint.Effect {
				taintExists = true
				log.V(1).Info("Taint already exists on server", "server", server.Name, "taintKey", taint.Key)
				break
			}
		}

		if !taintExists {
			server.Spec.Taints = append(server.Spec.Taints, taint)

			if err := k8sClient.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
				return fmt.Errorf("failed to patch server spec with taint: %w", err)
			}

			log.Info("Added taint to server spec",
				"server", server.Name,
				"taintKey", taint.Key,
				"taintValue", taint.Value)
		}
	*/
	return nil
}
