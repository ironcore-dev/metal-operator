package deploy

import (
	"context"
	"fmt"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Markers interface {
	MarkerExists(markerName metalv1alpha1.MarkerName) bool
	UpdateMarkers(ctx context.Context, opts ...MarkersUpdateOption) error
}

type AgentTokenStore interface {
	AgentToken(ctx context.Context) (string, error)
	SetAgentToken(ctx context.Context, token string) error
}

type Records struct {
	client  client.Client
	claim   *metalv1alpha1.ServerClaim
	records *metalv1alpha1.Records
}

type MarkersUpdate struct {
	Add    []metalv1alpha1.MarkerName
	Delete []metalv1alpha1.MarkerName
}

type MarkersUpdateOption func(*MarkersUpdate)

func AddMarker(markerNames ...metalv1alpha1.MarkerName) MarkersUpdateOption {
	return func(o *MarkersUpdate) {
		o.Add = append(o.Add, markerNames...)
	}
}

func DeleteMarker(markerName metalv1alpha1.MarkerName) MarkersUpdateOption {
	return func(o *MarkersUpdate) {
		o.Delete = append(o.Delete, markerName)
	}
}

func RecordsFor(ctx context.Context, c client.Client, claim *metalv1alpha1.ServerClaim) (*Records, error) {
	records := &metalv1alpha1.Records{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(claim), records); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("getting records: %w", err)
		}

		records = &metalv1alpha1.Records{
			Namespace: claim.Namespace,
			Name:      claim.Name,
		}
		_ = ctrl.SetControllerReference(claim, records, c.Scheme())
		if err := c.Create(ctx, records); err != nil {
			return nil, fmt.Errorf("creating records: %w", err)
		}
	}

	return &Records{
		client:  c,
		claim:   claim,
		records: records,
	}, nil
}

func (r *Records) AgentToken(ctx context.Context) (string, error) {
	secretRef := r.records.AgentTokenSecretRef
	if secretRef == nil {
		return "", nil
	}

	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{Namespace: r.records.Namespace, Name: secretRef.Name}
	if err := r.client.Get(ctx, secretKey, secret); err != nil {
		return "", fmt.Errorf("getting agent token secret: %w", err)
	}

	token := string(secret.Data["token"])
	return token, nil
}

func (r *Records) SetAgentToken(ctx context.Context, token string) error {
	if secretRef := r.records.AgentTokenSecretRef; secretRef != nil {
		if err := r.client.Apply(ctx,
			v1.Secret(r.records.Namespace, secretRef.Name).
				WithOwnerReferences(
					metav1ac.OwnerReference().
						WithAPIVersion(metalv1alpha1.SchemeGroupVersion.String()).
						WithKind("Records").
						WithName(r.records.Name).
						WithUID(r.records.UID).
						WithController(true)).
				WithStringData(map[string]string{"token": token}),
			client.FieldOwner("metal.ironcore.dev/controller-manager"),
		); err != nil {
			return fmt.Errorf("applying agent token secret: %w", err)
		}
		return nil
	}

	secret := &corev1.Secret{
		Namespace:    r.records.Namespace,
		GenerateName: "agent-token-",
		Type:         corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"token": token,
		},
	}
	_ = ctrl.SetControllerReference(r.records, secret, r.client.Scheme())
	if err := r.client.Create(ctx, secret); err != nil {
		return fmt.Errorf("creating agent token secret: %w", err)
	}

	base := r.records.DeepCopy()
	r.records.AgentTokenSecretRef = &metalv1alpha1.LocalObjectReference{
		Name: secret.Name,
	}
	if err := r.client.Patch(ctx, r.records,
		client.MergeFromWithOptions(base, &client.MergeFromWithOptimisticLock{}),
	); err != nil {
		_ = r.client.Delete(ctx, secret)
		return fmt.Errorf("patching agent token secret: %w", err)
	}
	return nil
}

func (r *Records) MarkerExists(markerName metalv1alpha1.MarkerName) bool {
	return metalv1alpha1.HasMarker(r.records.Markers, markerName)
}

func (r *Records) UpdateMarkers(ctx context.Context, opts ...MarkersUpdateOption) error {
	update := &MarkersUpdate{}
	for _, opt := range opts {
		opt(update)
	}

	base := r.records.DeepCopy()

	var changed bool
	for _, name := range update.Add {
		added := metalv1alpha1.SetMarker(&r.records.Markers, name)
		changed = changed || added
	}
	for _, name := range update.Delete {
		deleted := metalv1alpha1.DeleteMarker(&r.records.Markers, name)
		changed = changed || deleted
	}
	if !changed {
		return nil
	}

	if err := r.client.Patch(ctx, r.records,
		client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}),
	); err != nil {
		return fmt.Errorf("patching records: %w", err)
	}
	return nil
}

func (r *Records) Image() *metalv1alpha1.Image {
	return r.records.Image
}

func (r *Records) SetImage(ctx context.Context, image metalv1alpha1.Image) error {
	if r.records.Image != nil {
		return fmt.Errorf("cannot update image")
	}

	base := r.records.DeepCopy()
	r.records.Image = &image
	if err := r.client.Patch(ctx, r.records,
		client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}),
	); err != nil {
		return fmt.Errorf("patching records: %w", err)
	}
	return nil
}
