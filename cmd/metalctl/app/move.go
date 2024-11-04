package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"time"

	metalv1alphav1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/spf13/cobra"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	sourceKubeconfig string
	targetKubeconfig string
	crdsOnly         bool
	crsOnly          bool
	namespace        string
	dryRun           bool
	verbose          bool
)

func NewMoveCommand() *cobra.Command {
	move := &cobra.Command{
		Use:   "move",
		Short: "Move metal-operator CRDs and CRs from one cluster to another",
		RunE:  runMove,
	}
	move.Flags().StringVar(&sourceKubeconfig, "source-kubeconfig", "", "Kubeconfig pointing to the source cluster")
	move.Flags().StringVar(&targetKubeconfig, "target-kubeconfig", "", "Kubeconfig pointing to the target cluster")
	move.Flags().BoolVar(&crdsOnly, "crds-only", false, "migrate only the CRDs without CRs")
	move.Flags().BoolVar(&crsOnly, "crs-only", false, "migrate only the CRs without CRDs")
	move.Flags().StringVar(&namespace, "namespace", "", "namespace to filter CRDs and CRs to migrate. Defaults to all namespaces if not specified")
	move.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be moved without executing the migration")
	move.Flags().BoolVar(&verbose, "verbose", false, "enable verbose logging for detailed output during migration")
	move.MarkFlagRequired("source-kubeconfig")
	move.MarkFlagRequired("target-kubeconfig")

	if verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	return move
}

type Clients struct {
	source client.Client
	target client.Client
}

func makeClient(kubeconfig string) (client.Client, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster kubeconfig: %w", err)
	}
	return client.New(cfg, client.Options{Scheme: scheme})
}

func makeClients() (Clients, error) {
	var clients Clients
	var err error

	clients.source, err = makeClient(sourceKubeconfig)
	if err != nil {
		return clients, fmt.Errorf("failed to construct source cluster client: %w", err)
	}
	clients.target, err = makeClient(targetKubeconfig)
	if err != nil {
		return clients, fmt.Errorf("failed to construct target cluster client: %w", err)
	}
	return clients, nil
}

// getMetalObjects returns CRDs and CRs from metal group. CRDs in a returned list are before theirs CRs.
func getMetalObjects(ctx context.Context, cl client.Client) ([]client.Object, error) {
	crds := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := cl.List(ctx, crds); err != nil {
		return nil, fmt.Errorf("couldn't list CRDs: %w", err)
	}

	metalObjects := make([]client.Object, 0)
	for _, crd := range crds.Items {
		if crd.Spec.Group != metalv1alphav1.GroupVersion.Group {
			continue
		}
		if !crsOnly {
			metalObjects = append(metalObjects, &crd)
		}

		if !crdsOnly {
			crs := &unstructured.UnstructuredList{}
			crs.SetGroupVersionKind(schema.GroupVersionKind{Group: crd.Spec.Group, Version: crd.Spec.Versions[0].Name, Kind: crd.Spec.Names.Kind})

			if err := cl.List(ctx, crs, &client.ListOptions{Namespace: namespace}); err != nil {
				return nil, fmt.Errorf("couldn't list CRs: %w", err)
			}
			for _, cr := range crs.Items { // won't work with go version <1.22
				metalObjects = append(metalObjects, &cr)
			}
		}
	}

	return metalObjects, nil
}

func clearObjectFields(obj client.Object) map[string]interface{} {
	so, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	delete(so, "status")
	for _, field := range []string{"creationTimestamp", "resourceVersion", "uid", "generation", "managedFields"} {
		delete(so["metadata"].(map[string]interface{}), field)
	}
	return so
}

func getObjectsToBeMoved(ctx context.Context, sourceObjs []client.Object, targetClient client.Client) ([]client.Object, error) {
	objectsToBeMoved := make([]client.Object, 0, len(sourceObjs))
	for _, sourceObj := range sourceObjs {
		targetObj := sourceObj.DeepCopyObject().(client.Object)
		err := targetClient.Get(ctx, client.ObjectKeyFromObject(sourceObj), targetObj)
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			objectsToBeMoved = append(objectsToBeMoved, sourceObj)
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("failed to check CRD and CR existence in target cluster: %w", err)
		}

		if reflect.DeepEqual(clearObjectFields(sourceObj), clearObjectFields(targetObj)) {
			slog.Debug("source and target CRD or CR are the same", slog.String("object", metalObjectToString(sourceObj)))
			continue
		}
		return nil, fmt.Errorf("%s already exists in the target cluster", client.ObjectKeyFromObject(sourceObj))
	}
	return objectsToBeMoved, nil
}

func moveMetalObjects(ctx context.Context, sourceObjs []client.Object, cl client.Client) error {
	movedObjects := make([]client.Object, 0)

	for _, sourceObj := range sourceObjs {
		var err error
		sourceObj.SetResourceVersion("")
		if err = cl.Create(ctx, sourceObj); err == nil {
			if crd, isCrd := sourceObj.(*apiextensionsv1.CustomResourceDefinition); isCrd &&
				slices.IndexFunc(sourceObjs, func(obj client.Object) bool {
					return obj.GetObjectKind().GroupVersionKind().Kind == crd.Spec.Names.Kind
				}) != -1 {
				err = waitForMetalCRD(ctx, crd, cl)
			}
		}

		if err != nil {
			cleanupErrs := make([]error, 0)
			for _, obj := range movedObjects {
				if err := cl.Delete(ctx, obj); err != nil {
					cleanupErrs = append(cleanupErrs, err)
				}
			}

			return errors.Join(
				fmt.Errorf("%s couldn't be created in the target cluster: %w", metalObjectToString(sourceObj), err),
				fmt.Errorf("clean up was performed to restore a target cluster's state with error result: %w", errors.Join(cleanupErrs...)))
		}
		movedObjects = append(movedObjects, sourceObj)
	}
	return nil
}

func waitForMetalCRD(ctx context.Context, crd *apiextensionsv1.CustomResourceDefinition, cl client.Client) error {
	return wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		targetCrd := apiextensionsv1.CustomResourceDefinition{}
		if err := cl.Get(ctx, client.ObjectKeyFromObject(crd), &targetCrd); apierrors.IsNotFound(err) {
			return false, nil
		}
		for _, condition := range targetCrd.Status.Conditions {
			if condition.Type == apiextensionsv1.Established && condition.Status == apiextensionsv1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
}

func move(ctx context.Context, clients Clients) error {
	sourceObjs, err := getMetalObjects(ctx, clients.source)
	if err != nil {
		return err
	}
	slog.Debug(fmt.Sprintf("found %s CRDs and CRs in the source cluster", metalv1alphav1.GroupVersion.Group), slog.Any("Objects", transform(sourceObjs, metalObjectToString)))

	objectsToBeMoved, err := getObjectsToBeMoved(ctx, sourceObjs, clients.target)
	if err != nil {
		return err
	}
	slog.Debug("moving", slog.Any("Objects", transform(objectsToBeMoved, metalObjectToString)))

	if !dryRun {
		err = moveMetalObjects(ctx, objectsToBeMoved, clients.target)
		if err == nil {
			slog.Debug(fmt.Sprintf("all %s CRDs and CRs from the source cluster were moved to the target cluster", metalv1alphav1.GroupVersion.Group))
		}
	}

	return err
}

func runMove(cmd *cobra.Command, args []string) error {
	clients, err := makeClients()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	return move(ctx, clients)
}
