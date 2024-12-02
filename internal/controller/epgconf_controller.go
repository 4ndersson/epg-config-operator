/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	epgv1alpha1 "github.com/4ndersson/epg-config-operator/api/v1alpha1"
	"github.com/4ndersson/epg-config-operator/pkg/aci"
	"github.com/go-logr/logr"
	"github.com/samber/lo"
)

// ConfReconciler reconciles a Conf object
type EpgconfReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	ApicClient aci.ApicInterface
	CniConfig  CniConfig
}

type CniConfig struct {
	ApicIp             string
	ApicUsername       string
	ApicPassword       string
	ApicPrivateKey     string
	KeyPath            string
	Tenant             string
	BridgeDomain       string
	VmmDomain          string
	VmmDomainType      string
	ApplicationProfile string
	ProvidedContracts  []string
	ConsumedContracts  []string
}

// +kubebuilder:rbac:groups=epg.custom.aci,resources=epgconfs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=epg.custom.aci,resources=epgconfs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=epg.custom.aci,resources=epgconfs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Conf object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
const epgConfFinalizer = "epg.custom.config/finalizer"

func (r *EpgconfReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	conf := &epgv1alpha1.Epgconf{}
	err := r.Get(ctx, req.NamespacedName, conf)
	if err != nil {
		if errors.IsNotFound(err) {
			l.Info("Epg config resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		l.Error(err, "Failed to get Epg config resource")
		return ctrl.Result{}, err
	}

	isEpgConfigMarkedToBeDeleted := conf.GetDeletionTimestamp() != nil
	if isEpgConfigMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(conf, epgConfFinalizer) {
			if err := r.finalizeEpgConf(ctx, l, conf); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(conf, epgConfFinalizer)
			err := r.Update(ctx, conf)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(conf, epgConfFinalizer) {
		l.Info("adding finalizer", "finalizer", epgConfFinalizer)
		controllerutil.AddFinalizer(conf, epgConfFinalizer)
		err = r.Update(ctx, conf)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.ReconcileEpgConf(ctx, l, conf)

	if err != nil {
		conf.Status.State = "Failed"
		err = r.Status().Update(context.Background(), conf)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("error occurred while setting the status: %w", err)
		}
		return result, err
	}

	conf.Status.State = "Ready"
	err = r.Status().Update(context.Background(), conf)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error occurred while setting the status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *EpgconfReconciler) ReconcileEpgConf(ctx context.Context, l logr.Logger, conf *epgv1alpha1.Epgconf) (ctrl.Result, error) {
	err := r.ApicClient.CreateEpg(fmt.Sprintf(conf.GetNamespace()+"_EPG"), r.CniConfig.ApplicationProfile, r.CniConfig.Tenant, r.CniConfig.BridgeDomain, r.CniConfig.VmmDomain, r.CniConfig.VmmDomainType) //nolint:govet
	if err != nil {
		l.Error(err, "error occurred while creating epg")
		return ctrl.Result{}, err
	}

	l.Info(fmt.Sprintf("Adds annotation on namespace %s", conf.GetNamespace()))
	err = r.AnnotateNamespace(ctx, conf.GetNamespace(), r.CniConfig.ApplicationProfile, r.CniConfig.Tenant)
	if err != nil {
		l.Info("error occurred while annotating the namespace: %w", err)
		return ctrl.Result{}, err
	}

	consumedContracts, err := r.ApicClient.GetConsumedContracts(conf.GetNamespace()+"_EPG", r.CniConfig.ApplicationProfile, r.CniConfig.Tenant)
	if err != nil {
		return ctrl.Result{}, err
	}

	_, diffConsumedContracts := lo.Difference(consumedContracts, r.CniConfig.ConsumedContracts)

	l.Info(fmt.Sprintf("Consume contracts for EPG %s", conf.Name))
	for _, contract := range diffConsumedContracts {
		err = r.ApicClient.ConsumeContract(conf.GetNamespace()+"_EPG", r.CniConfig.ApplicationProfile, r.CniConfig.Tenant, contract)
		if err != nil {
			l.Info("error occurred while consuming contract: %w", err)
			return ctrl.Result{}, err
		}
	}

	providedContracts, err := r.ApicClient.GetProvidedContracts(conf.GetNamespace()+"_EPG", r.CniConfig.ApplicationProfile, r.CniConfig.Tenant)
	if err != nil {
		return ctrl.Result{}, err
	}

	_, diffProvidedContracts := lo.Difference(providedContracts, r.CniConfig.ProvidedContracts)

	l.Info(fmt.Sprintf("Provide contracts for EPG %s", conf.Name))
	for _, contract := range diffProvidedContracts {
		err = r.ApicClient.ProvideContract(conf.GetNamespace()+"_EPG", r.CniConfig.ApplicationProfile, r.CniConfig.Tenant, contract)
		if err != nil {
			l.Info("error occurred while providing contract: %w", err)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EpgconfReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epgv1alpha1.Epgconf{}).
		Complete(r)
}

func (r *EpgconfReconciler) finalizeEpgConf(ctx context.Context, l logr.Logger, c *epgv1alpha1.Epgconf) error {
	l.Info(fmt.Sprintf("Deleting EPG  %s", c.GetNamespace()+"_EPG"))
	err := r.ApicClient.DeleteEpg(c.GetNamespace()+"_EPG", r.CniConfig.ApplicationProfile, r.CniConfig.Tenant)

	if err != nil {
		return fmt.Errorf("error occurred while deleting EPG: %w", err)
	}

	err = r.RemoveAnnotationNamespace(ctx, c.GetNamespace())
	if err != nil {
		return fmt.Errorf("error occurred while deleting annotation on namespace: %w", err)
	}
	return nil
}

func (r *EpgconfReconciler) AnnotateNamespace(ctx context.Context, nsName, app, tenant string) error {
	dnJson := fmt.Sprintf(`{\"tenant\":\"%s\",\"app-profile\":\"%s\",\"name\":\"%s_EPG\"}`, tenant, app, nsName)
	patch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{"opflex.cisco.com/endpoint-group": "%s"}}}`, dnJson))
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	if err := r.Client.Patch(ctx, ns, client.RawPatch(types.MergePatchType, patch)); err != nil {
		return err
	}
	return nil
}

func (r *EpgconfReconciler) RemoveAnnotationNamespace(ctx context.Context, nsName string) error {
	patch := []byte(`[{"op": "remove", "path": "/metadata/annotations/opflex.cisco.com~1endpoint-group"}]`)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	err := r.Client.Patch(ctx, ns, client.RawPatch(types.JSONPatchType, patch))
	if err != nil {
		return err
	}
	return nil
}
