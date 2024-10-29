package app

import (
	"context"
	"errors"
	"fmt"

	metalv1alphav1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/spf13/cobra"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	targetKubeconfig string
	errCrdCreate     error = errors.New("failed to create metal CRDs")
)

func NewMoveCommand() *cobra.Command {
	move := &cobra.Command{
		Use:   "move",
		Short: "Move metal-operator CRDs and CRs from one cluster to another",
		RunE:  runMove,
	}
	move.Flags().StringVar(&targetKubeconfig, "target-kubeconfig", "", "Kubeconfig pointing to the target cluster")
	move.MarkFlagRequired("target-kubeconfig")
	return move
}

type clients struct {
	source client.Client
	target client.Client
}

func makeClients() (clients, error) {
	var clients clients
	sourceCfg, err := GetKubeconfig()
	if err != nil {
		return clients, fmt.Errorf("failed to load source cluster kubeconfig: %w", err)
	}
	clients.source, err = client.New(sourceCfg, client.Options{Scheme: scheme})
	if err != nil {
		return clients, fmt.Errorf("failed to construct source cluster client: %w", err)
	}
	targetCfg, err := clientcmd.BuildConfigFromFlags("", targetKubeconfig)
	if err != nil {
		return clients, fmt.Errorf("failed to load target cluster kubeconfig: %w", err)
	}
	clients.target, err = client.New(targetCfg, client.Options{Scheme: scheme})
	if err != nil {
		return clients, fmt.Errorf("failed to construct target cluster client: %w", err)
	}
	return clients, nil
}

func moveCRDs(ctx context.Context, clients clients) error {
	var crds apiextensionsv1.CustomResourceDefinitionList
	if err := clients.source.List(ctx, &crds); err != nil {
		return err
	}
	metalCrds := make([]apiextensionsv1.CustomResourceDefinition, 0)
	for _, crd := range crds.Items {
		if crd.Spec.Group == metalv1alphav1.GroupVersion.Group {
			metalCrds = append(metalCrds, crd)
		}
	}
	// it may be better to compare on semantics instead of CRD name
	for _, sourceCrd := range metalCrds {
		var targetCrd apiextensionsv1.CustomResourceDefinition
		err := clients.target.Get(ctx, client.ObjectKeyFromObject(&sourceCrd), &targetCrd)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to check CRD existence in target cluster: %w", err)
		}
		return fmt.Errorf("CRD for %s/%s already exists in the target cluster", sourceCrd.Spec.Group, sourceCrd.Spec.Names.Plural)
	}
	for _, crd := range metalCrds {
		crd.ResourceVersion = ""
		if err := clients.target.Create(ctx, &crd); err != nil {
			return errCrdCreate
		}
	}
	return nil
}

func cleanupCRDs(ctx context.Context, clients clients) error {
	var crds apiextensionsv1.CustomResourceDefinitionList
	if err := clients.target.List(ctx, &crds); err != nil {
		return err
	}
	metalCrds := make([]apiextensionsv1.CustomResourceDefinition, 0)
	for _, crd := range crds.Items {
		if crd.Spec.Group == metalv1alphav1.GroupVersion.Group {
			metalCrds = append(metalCrds, crd)
		}
	}
	errs := make([]error, 0)
	for _, crd := range metalCrds {
		crd.ResourceVersion = ""
		if err := clients.target.Delete(ctx, &crd); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func runMove(cmd *cobra.Command, args []string) error {
	clients, err := makeClients()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	err = moveCRDs(ctx, clients)
	switch {
	case errors.Is(err, errCrdCreate):
		return cleanupCRDs(ctx, clients)
	case err != nil:
		return err
	}
	return nil
}
