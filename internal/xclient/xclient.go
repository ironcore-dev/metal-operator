package xclient

import (
	"context"
	"maps"
	"strings"

	"github.com/ironcore-dev/controller-utils/metautils"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type InNamespace string

func (in InNamespace) ApplyToSelect(o *SelectOptions) {
	o.Namespace = string(in)
}

func (in InNamespace) ApplyToSelectOrCreate(o *SelectOrCreateOptions) {
	in.ApplyToSelect(&o.SelectOptions)
}

func (in InNamespace) ApplyToDeleteSelection(o *DeleteSelectionOptions) {
	in.ApplyToSelect(&o.SelectOptions)
}

type WithLabels map[string]string

func (w WithLabels) ApplyToSelect(o *SelectOptions) {
	o.Labels = w
}

func (w WithLabels) ApplyToSelectOrCreate(o *SelectOrCreateOptions) {
	w.ApplyToSelect(&o.SelectOptions)
}

func (w WithLabels) ApplyToDeleteSelection(o *DeleteSelectionOptions) {
	w.ApplyToSelect(&o.SelectOptions)
}

type WithOwner struct {
	Object client.Object
}

func (w WithOwner) ApplyToSelect(o *SelectOptions) {
	o.Owner = w.Object
}

func (w WithOwner) ApplyToSelectOrCreate(o *SelectOrCreateOptions) {
	w.ApplyToSelect(&o.SelectOptions)
}

func (w WithOwner) ApplyToDeleteSelection(o *DeleteSelectionOptions) {
	w.ApplyToSelect(&o.SelectOptions)
}

type SelectOptions struct {
	Namespace string
	Labels    map[string]string
	Owner     client.Object
}

type SelectOption interface {
	ApplyToSelect(*SelectOptions)
}

func (o *SelectOptions) ApplyToSelect(opts *SelectOptions) {
	if o.Namespace != "" {
		opts.Namespace = o.Namespace
	}
	if o.Labels != nil {
		opts.Labels = o.Labels
	}
	if o.Owner != nil {
		opts.Owner = o.Owner
	}
}

func (o *SelectOptions) ApplyToSelectOrCreate(opts *SelectOrCreateOptions) {
	o.ApplyToSelect(&opts.SelectOptions)
}

func (o *SelectOptions) ApplyToDeleteSelection(opts *DeleteSelectionOptions) {
	o.ApplyToSelect(&opts.SelectOptions)
}

func selectForEach(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	namespace string,
	labels map[string]string,
	owner client.Object,
	f func() error,
) error {
	_, list, err := metautils.NewListForObject(c.Scheme(), obj)
	if err != nil {
		return err
	}

	var listOpts []client.ListOption
	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}
	if labels != nil {
		listOpts = append(listOpts, client.MatchingLabels(labels))
	}
	if err := c.List(ctx, list, listOpts...); err != nil {
		return err
	}

	return meta.EachListItem(list, func(item runtime.Object) error {
		if err := c.Scheme().Convert(item, obj, nil); err != nil {
			return err
		}
		if owner != nil && !metav1.IsControlledBy(obj, owner) {
			return nil
		}

		return f()
	})
}

func listAndMatch(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	namespace string,
	labels map[string]string,
	owner client.Object,
	pred func() (bool, error),
	onOutdated func() error,
) error {
	gvk, err := apiutil.GVKForObject(obj, c.Scheme())
	if err != nil {
		return err
	}

	var (
		found    bool
		matching = obj.DeepCopyObject().(client.Object)
	)
	if err := selectForEach(ctx, c, obj, namespace, labels, owner, func() error {
		ok := matching != nil && obj.GetCreationTimestamp().After(matching.GetCreationTimestamp().Time)
		if ok {
			if pred != nil {
				var err error
				ok, err = pred()
				if err != nil {
					return err
				}
			}
		}
		if !ok {
			if onOutdated != nil {
				if err := onOutdated(); err != nil {
					return err
				}
			}
			return nil
		}

		found = true
		return c.Scheme().Convert(obj, matching, nil)
	}); err != nil {
		return err
	}
	if !found {
		return apierrors.NewNotFound(schema.GroupResource{
			Group:    gvk.Group,
			Resource: strings.ToLower(gvk.Kind),
		}, "")
	}
	return nil
}

func Select(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	opts ...SelectOption,
) error {
	gOpts := &SelectOptions{}
	for _, opt := range opts {
		opt.ApplyToSelect(gOpts)
	}

	return listAndMatch(ctx, c, obj, gOpts.Namespace, gOpts.Labels, gOpts.Owner, nil, nil)
}

type SelectOrCreateOptions struct {
	SelectOptions
}

type SelectOrCreateOption interface {
	ApplyToSelectOrCreate(*SelectOrCreateOptions)
}

func SelectOrCreate(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	mutate func() error,
	opts ...SelectOrCreateOption,
) error {
	log := logf.FromContext(ctx)
	aOpts := &SelectOrCreateOptions{}
	for _, opt := range opts {
		opt.ApplyToSelectOrCreate(aOpts)
	}

	postMutate := func() error {
		obj.SetNamespace(aOpts.Namespace)

		lbls := obj.GetLabels()
		maps.Copy(lbls, aOpts.Labels)
		obj.SetLabels(aOpts.Labels)

		if aOpts.Owner != nil {
			if err := ctrl.SetControllerReference(aOpts.Owner, obj, c.Scheme()); err != nil {
				return err
			}
		}
		return nil
	}

	if err := listAndMatch(ctx, c, obj, aOpts.Namespace, aOpts.Labels, aOpts.Owner,
		func() (bool, error) {
			existing := obj.DeepCopyObject()
			if err := mutate(); err != nil {
				return false, err
			}
			if err := postMutate(); err != nil {
				return false, err
			}
			// Reset generate name since this can be set by mutate.
			obj.SetGenerateName("")

			if !equality.Semantic.DeepEqual(existing, obj) {
				return false, nil
			}
			return true, nil
		}, func() error {
			if err := c.Delete(ctx, obj); client.IgnoreNotFound(err) != nil {
				log.Error(err, "Failed to delete stale object", "obj", klog.KObj(obj))
			}
			return nil
		},
	); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err := mutate(); err != nil {
			return err
		}
		if err := postMutate(); err != nil {
			return err
		}
		return c.Create(ctx, obj)
	}
	return nil
}

type DeleteSelectionOptions struct {
	SelectOptions
}

type DeleteSelectionOption interface {
	ApplyToDeleteSelection(*DeleteSelectionOptions)
}

func DeleteSelection(ctx context.Context, c client.Client, obj client.Object, opts ...DeleteSelectionOption) error {
	dOpts := &DeleteSelectionOptions{}
	for _, opt := range opts {
		opt.ApplyToDeleteSelection(dOpts)
	}

	return selectForEach(ctx, c, obj, dOpts.Namespace, dOpts.Labels, dOpts.Owner, func() error {
		return client.IgnoreNotFound(c.Delete(ctx, obj))
	})
}
