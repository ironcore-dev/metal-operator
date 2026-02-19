// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// bmcRefField is the field indexer key for looking up servers by BMC reference
	bmcRefField = "spec.bmcRef.name"

	// CriticalEventConditionType is the condition type for critical events
	CriticalEventConditionType = "CriticalEventReceived"
)

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
			if err := taintServer(ctx, k8sClient, server, event, log); err != nil {
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
func taintServer(ctx context.Context, k8sClient client.Client, server *metalv1alpha1.Server, event Event, log logr.Logger) error {
	serverBase := server.DeepCopy()
	// Approach 1: Add a condition to mark the critical event
	// This is a Kubernetes-native way to mark the server and works immediately
	criticalEventCondition := metav1.Condition{
		Type:               CriticalEventConditionType,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: server.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             fmt.Sprintf("CriticalEvent%s", event.EventID),
		Message:            fmt.Sprintf("Critical Redfish event received: %s (Component: %s)", event.Message, event.OriginOfCondition),
	}

	// Check if condition already exists and is still True
	conditionExists := false
	for i, existingCondition := range server.Status.Conditions {
		if existingCondition.Type == CriticalEventConditionType {
			conditionExists = true
			// Update the condition with the latest event
			server.Status.Conditions[i] = criticalEventCondition
			break
		}
	}

	if !conditionExists {
		server.Status.Conditions = append(server.Status.Conditions, criticalEventCondition)
	}

	// Patch the server status with the new condition
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
		taintKey := fmt.Sprintf("metal.ironcore.dev/critical-event-%s", event.EventID)
		taint := corev1.Taint{
			Key:    taintKey,
			Value:  event.OriginOfCondition,
			Effect: corev1.TaintEffectNoSchedule,
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
