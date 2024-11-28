// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package cmdutils

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"time"

	metalv1alphav1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	pollInterval = time.Second / 10
	pollTimeout  = time.Second * 30
)

func getCrs(
	ctx context.Context,
	cl client.Client,
	crsGvk []schema.GroupVersionKind,
	namespace string,
) ([]*unstructured.Unstructured, error) {
	crs := make([]*unstructured.Unstructured, 0)

	for _, crGvk := range crsGvk {
		crsList := &unstructured.UnstructuredList{}
		crsList.SetGroupVersionKind(crGvk)

		if err := cl.List(ctx, crsList, &client.ListOptions{Namespace: namespace}); err != nil {
			return nil, fmt.Errorf("couldn't list CRs: %w", err)
		}
		for _, cr := range crsList.Items { // won't work with go version <1.22
			crs = append(crs, &cr)
		}
	}

	return crs, nil
}

func clearFields(obj client.Object) map[string]any {
	so, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)

	for _, field := range []string{"creationTimestamp", "resourceVersion", "uid", "generation", "managedFields"} {
		delete(so["metadata"].(map[string]any), field)
	}

	if so["status"] != nil && so["status"].(map[string]any)["conditions"] != nil {
		for _, field := range so["status"].(map[string]any)["conditions"].([]interface{}) {
			delete(field.(map[string]any), "lastTransitionTime")
		}
	}

	return so
}

func getCrsToBeMoved(
	ctx context.Context,
	targetClient client.Client,
	sourceCrs []*unstructured.Unstructured,
) ([]*unstructured.Unstructured, error) {
	crsToMove := make([]*unstructured.Unstructured, 0, len(sourceCrs))
	for _, sourceCr := range sourceCrs {
		targetCr := sourceCr.DeepCopy()
		err := targetClient.Get(ctx, client.ObjectKeyFromObject(sourceCr), targetCr)
		if apierrors.IsNotFound(err) {
			crsToMove = append(crsToMove, sourceCr)
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("failed to check CR existence in the target cluster: %w", err)
		}

		if reflect.DeepEqual(clearFields(sourceCr), clearFields(targetCr)) {
			slog.Debug("source and target CRs are the same", slog.String("CR", crName(sourceCr)))
			continue
		}
		return nil, fmt.Errorf(
			"a CR %s/%s already exists in the target cluster and is different then in the source cluster",
			sourceCr.GetNamespace(), sourceCr.GetName())
	}
	return crsToMove, nil
}

type Node struct {
	Cr       *unstructured.Unstructured
	Children []*Node
}

func crsOwnerReferenceTrees(crs []*unstructured.Unstructured) []*Node {
	nodeMap := make(map[types.UID]*Node)

	for _, cr := range crs {
		nodeMap[cr.GetUID()] = &Node{Cr: cr}
	}
	roots := []*Node{}
	for _, cr := range crs {
		if len(cr.GetOwnerReferences()) == 0 || nodeMap[cr.GetOwnerReferences()[0].UID] == nil {
			roots = append(roots, nodeMap[cr.GetUID()])
		} else {
			owner := nodeMap[cr.GetOwnerReferences()[0].UID]
			owner.Children = append(owner.Children, nodeMap[cr.GetUID()])
		}
	}
	return roots
}

func cleanup(ctx context.Context, cl client.Client, crs []*unstructured.Unstructured) error {
	cleanupErrs := make([]error, 0)
	for _, cr := range crs {
		if err := cl.Delete(ctx, cr); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
	}
	return errors.Join(cleanupErrs...)
}

func moveCrs(
	ctx context.Context,
	cl client.Client,
	crsTrees []*Node,
	ownerUid ...types.UID,
) ([]*unstructured.Unstructured, error) {
	movedCrs := make([]*unstructured.Unstructured, 0)

	for _, crsTree := range crsTrees {
		cr := crsTree.Cr.DeepCopy()
		ownerReferences := cr.GetOwnerReferences()
		if len(ownerReferences) == 1 && len(ownerUid) == 1 {
			ownerReferences[0].UID = ownerUid[0]
			cr.SetOwnerReferences(ownerReferences)
		}
		cr.SetResourceVersion("")
		if err := cl.Create(ctx, cr); err != nil {
			err = fmt.Errorf("CR %s couldn't be created in the target cluster: %w", crName(cr), err)
			return movedCrs, err
		}
		movedCrs = append(movedCrs, cr)
	}

	for _, crsTree := range crsTrees {
		err := wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
			// get CR from target cluster
			cr := crsTree.Cr.DeepCopy()
			if err := cl.Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
				return false, client.IgnoreNotFound(err)
			}

			if err := copyStatus(ctx, cl, crsTree.Cr, cr); err != nil {
				return false, err
			}

			// create children CRs
			movedChildrenCrs, err := moveCrs(ctx, cl, crsTree.Children, cr.GetUID())
			movedCrs = slices.Concat(movedCrs, movedChildrenCrs)
			return true, err
		})
		if err != nil {
			return movedCrs, err
		}
	}

	return movedCrs, nil
}

func copyStatus(ctx context.Context, cl client.Client, sourceCr, targetCr *unstructured.Unstructured) error {
	status, found, err := unstructured.NestedMap(sourceCr.Object, "status")
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	if err := unstructured.SetNestedField(targetCr.Object, status, "status"); err != nil {
		return err
	}
	return cl.Status().Update(ctx, targetCr)
}

func Move(
	ctx context.Context,
	clients Clients,
	crsGvk []schema.GroupVersionKind,
	namespace string,
	dryRun bool,
) error {
	sourceCrs, err := getCrs(ctx, clients.Source, crsGvk, namespace)
	if err != nil {
		return err
	}
	slog.Debug(fmt.Sprintf("found %s CRs in the source cluster", metalv1alphav1.GroupVersion.Group),
		slog.Any("CRs", transform(sourceCrs, crName)))

	crsToMove, err := getCrsToBeMoved(ctx, clients.Target, sourceCrs)
	if err != nil {
		return err
	}
	slog.Debug("moving", slog.Any("CRs", transform(crsToMove, crName)))

	if !dryRun {
		crsTrees := crsOwnerReferenceTrees(crsToMove)
		var movedCrs []*unstructured.Unstructured
		if movedCrs, err = moveCrs(ctx, clients.Target, crsTrees); err != nil {
			cleanupErr := cleanup(ctx, clients.Target, movedCrs)
			err = errors.Join(err,
				fmt.Errorf("clean up of CRs was performed to restore a target cluster's state with error result: %w",
					cleanupErr))
		} else {
			slog.Debug(fmt.Sprintf("all %s CRs from the source cluster were moved to the target cluster",
				metalv1alphav1.GroupVersion.Group))
		}
	}

	return err
}
