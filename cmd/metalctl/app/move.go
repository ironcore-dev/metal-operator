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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	sourceKubeconfig string
	targetKubeconfig string
	namespace        string
	dryRun           bool
	verbose          bool
)

func NewMoveCommand() *cobra.Command {
	move := &cobra.Command{
		Use:   "move",
		Short: "Move metal-operator CRs from one cluster to another",
		RunE:  runMove,
	}
	move.Flags().StringVar(&sourceKubeconfig, "source-kubeconfig", "", "Kubeconfig pointing to the source cluster")
	move.Flags().StringVar(&targetKubeconfig, "target-kubeconfig", "", "Kubeconfig pointing to the target cluster")
	move.Flags().StringVar(&namespace, "namespace", "", "namespace to filter CRs to migrate. Defaults to all namespaces if not specified")
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
		return clients, fmt.Errorf("failed to construct a source cluster client: %w", err)
	}
	clients.target, err = makeClient(targetKubeconfig)
	if err != nil {
		return clients, fmt.Errorf("failed to construct a target cluster client: %w", err)
	}
	return clients, nil
}

func getMetalCrs(ctx context.Context, cl client.Client) ([]*unstructured.Unstructured, error) {
	crs := make([]*unstructured.Unstructured, 0)

	for _, crdKind := range []string{"BMC", "BMCSecret", "Endpoint", "Server", "ServerBootConfiguration", "ServerClaim"} {
		crsList := &unstructured.UnstructuredList{}
		crsList.SetGroupVersionKind(schema.GroupVersionKind{Group: "metal.ironcore.dev", Version: "v1alpha1", Kind: crdKind})

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

func getCrsToBeMoved(ctx context.Context, targetClient client.Client, sourceCrs []*unstructured.Unstructured) ([]*unstructured.Unstructured, error) {
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
		return nil, fmt.Errorf("a CR %s/%s already exists in the target cluster and is different then in the source cluster", sourceCr.GetNamespace(), sourceCr.GetName())
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

func cleanup[T client.Object](ctx context.Context, cl client.Client, objs []T) error {
	cleanupErrs := make([]error, 0)
	for _, obj := range objs {
		if err := cl.Delete(ctx, obj); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
	}
	return errors.Join(cleanupErrs...)
}

func moveCrs(ctx context.Context, cl client.Client, crsTrees []*Node, ownerUid ...types.UID) (movedCrs []*unstructured.Unstructured, err error) {
	movedCrs = make([]*unstructured.Unstructured, 0)

	for _, crsTree := range crsTrees {
		ownerReferences := crsTree.Cr.GetOwnerReferences()
		if len(ownerReferences) == 1 && len(ownerUid) == 1 {
			ownerReferences[0].UID = ownerUid[0]
			crsTree.Cr.SetOwnerReferences(ownerReferences)
		}

		crsTree.Cr.SetResourceVersion("")
		if err = cl.Create(ctx, crsTree.Cr); err != nil {
			err = fmt.Errorf("CR %s couldn't be created in the target cluster: %w", crName(crsTree.Cr), err)
			return
		}
		movedCrs = append(movedCrs, crsTree.Cr)
	}

	for _, crsTree := range crsTrees {
		err = wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
			// retrive uid of an owner
			ownerCr := crsTree.Cr.DeepCopy()
			err := cl.Get(ctx, client.ObjectKeyFromObject(crsTree.Cr), ownerCr)
			if err != nil {
				return false, client.IgnoreNotFound(err)
			}

			// create children CRs
			var movedChildrenCrs []*unstructured.Unstructured
			movedChildrenCrs, err = moveCrs(ctx, cl, crsTree.Children, ownerCr.GetUID())
			movedCrs = slices.Concat(movedCrs, movedChildrenCrs)
			return true, err
		})
		if err != nil {
			return
		}
	}

	return
}

func move(ctx context.Context, clients Clients) error {
	sourceCrs, err := getMetalCrs(ctx, clients.source)
	if err != nil {
		return err
	}
	slog.Debug(fmt.Sprintf("found %s CRs in the source cluster", metalv1alphav1.GroupVersion.Group),
		slog.Any("CRs", transform(sourceCrs, crName)))

	crsToMove, err := getCrsToBeMoved(ctx, clients.target, sourceCrs)
	if err != nil {
		return err
	}
	slog.Debug("moving", slog.Any("CRs", transform(crsToMove, crName)))

	if !dryRun {
		crsTrees := crsOwnerReferenceTrees(crsToMove)
		movedCrs := []*unstructured.Unstructured{}
		if movedCrs, err = moveCrs(ctx, clients.target, crsTrees); err != nil {
			cleanupErr := cleanup(ctx, clients.target, movedCrs)
			err = errors.Join(err,
				fmt.Errorf("clean up of CRs was performed to restore a target cluster's state with error result: %w", cleanupErr))
		} else {
			slog.Debug(fmt.Sprintf("all %s CRs from the source cluster were moved to the target cluster", metalv1alphav1.GroupVersion.Group))
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
