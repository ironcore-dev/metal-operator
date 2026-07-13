// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	serverSystemUUIDIndexField = "spec.systemUUID"
	serverRefField             = "spec.serverRef.name"
	bmcRefField                = "spec.bmcRef.name"
)

func RegisterIndexFields(ctx context.Context, indexer client.FieldIndexer) error {
	if err := indexer.IndexField(ctx, &metalv1alpha1.Server{}, serverSystemUUIDIndexField, func(rawObj client.Object) []string {
		server, ok := rawObj.(*metalv1alpha1.Server)
		if !ok {
			return nil
		}
		if server.Spec.SystemUUID == "" {
			return nil
		}
		return []string{server.Spec.SystemUUID}
	}); err != nil {
		return err
	}

	if err := indexer.IndexField(ctx, &metalv1alpha1.BIOSSettings{}, serverRefField, func(rawObj client.Object) []string {
		biosSettings, ok := rawObj.(*metalv1alpha1.BIOSSettings)
		if !ok {
			return nil
		}
		if biosSettings.Spec.ServerRef == nil {
			return nil
		}
		return []string{biosSettings.Spec.ServerRef.Name}
	}); err != nil {
		return err
	}

	if err := indexer.IndexField(ctx, &metalv1alpha1.Server{}, bmcRefField, func(rawObj client.Object) []string {
		server, ok := rawObj.(*metalv1alpha1.Server)
		if !ok {
			return nil
		}
		if server.Spec.BMCRef == nil {
			return nil
		}
		return []string{server.Spec.BMCRef.Name}
	}); err != nil {
		return err
	}

	smObj := &unstructured.Unstructured{}
	smObj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "servermaintenance.metal.ironcore.dev",
		Version: "v1alpha1",
		Kind:    "ServerMaintenance",
	})
	if err := indexer.IndexField(ctx, smObj, serverRefField, func(rawObj client.Object) []string {
		item, ok := rawObj.(*unstructured.Unstructured)
		if !ok {
			return nil
		}
		name, _, _ := unstructured.NestedString(item.Object, "spec", "serverRef", "name")
		if name == "" {
			return nil
		}
		return []string{name}
	}); err != nil {
		return err
	}

	return nil
}
