// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultCertificateRenewalBuffer is the time before expiration to renew certificates.
	DefaultCertificateRenewalBuffer = 30 * 24 * time.Hour // 30 days
)

// reconcileCertificate handles TLS certificate management for BMCs.
// It reads certificates from Kubernetes TLS secrets and installs them on BMCs via Redfish.
func (r *BMCReconciler) reconcileCertificate(ctx context.Context, bmcObj *metalv1alpha1.BMC) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Skip if TLSSecretRef not configured
	if bmcObj.Spec.TLSSecretRef == nil {
		log.V(1).Info("TLS certificate management not configured")
		return ctrl.Result{}, nil
	}

	// Skip if BMC is not in enabled state
	if bmcObj.Status.State != metalv1alpha1.BMCStateEnabled {
		log.V(1).Info("Skipping certificate reconciliation, BMC not enabled",
			"state", bmcObj.Status.State)
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Starting TLS certificate reconciliation",
		"secretName", bmcObj.Spec.TLSSecretRef.Name)

	// Get the TLS secret
	secret, err := r.getTLSSecret(ctx, bmcObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.handleCertificateError(ctx, bmcObj,
				fmt.Errorf("TLS secret not found: %s", bmcObj.Spec.TLSSecretRef.Name),
				"TLS secret not found")
		}
		return r.handleCertificateError(ctx, bmcObj, err, "Failed to get TLS secret")
	}

	// Validate secret type and data
	if err := r.validateTLSSecret(secret); err != nil {
		return r.handleCertificateError(ctx, bmcObj, err, "Invalid TLS secret format")
	}

	// Extract certificate and private key from secret
	certPEM := secret.Data["tls.crt"]
	keyPEM := secret.Data["tls.key"]

	// Check if certificate needs installation
	needsInstall, err := r.needsCertificateInstallation(ctx, bmcObj, certPEM)
	if err != nil {
		log.Error(err, "Failed to check certificate status, will attempt installation")
		needsInstall = true
	}

	if !needsInstall {
		log.V(1).Info("Certificate already installed and valid")
		return ctrl.Result{}, r.setCertificateCondition(ctx, bmcObj,
			corev1.ConditionTrue,
			metalv1alpha1.BMCCertificateReadyReasonIssued,
			"Certificate installed and valid")
	}

	log.Info("Installing certificate on BMC")

	// Install certificate on BMC
	if err := r.installCertificateOnBMC(ctx, bmcObj, certPEM, keyPEM); err != nil {
		return r.handleCertificateError(ctx, bmcObj, err, "Failed to install certificate on BMC")
	}

	log.Info("Successfully installed certificate on BMC")

	// Update status
	bmcBase := bmcObj.DeepCopy()
	bmcObj.Status.CertificateSecretRef = &corev1.LocalObjectReference{
		Name: bmcObj.Spec.TLSSecretRef.Name,
	}
	if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update certificate status: %w", err)
	}

	return ctrl.Result{}, r.setCertificateCondition(ctx, bmcObj,
		corev1.ConditionTrue,
		metalv1alpha1.BMCCertificateReadyReasonIssued,
		"Certificate installed successfully")
}

// getTLSSecret retrieves the TLS secret referenced by the BMC.
// It handles namespace resolution (defaults to cluster-scoped if not specified).
func (r *BMCReconciler) getTLSSecret(ctx context.Context, bmcObj *metalv1alpha1.BMC) (*corev1.Secret, error) {
	if bmcObj.Spec.TLSSecretRef == nil {
		return nil, fmt.Errorf("TLSSecretRef is nil")
	}

	secretNamespace := bmcObj.Spec.TLSSecretRef.Namespace
	if secretNamespace == "" {
		// BMC is cluster-scoped, but secrets are namespaced.
		// Use the manager namespace as default if not specified.
		secretNamespace = r.ManagerNamespace
	}

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      bmcObj.Spec.TLSSecretRef.Name,
		Namespace: secretNamespace,
	}, secret)

	return secret, err
}

// validateTLSSecret checks that the secret has the correct type and required keys.
func (r *BMCReconciler) validateTLSSecret(secret *corev1.Secret) error {
	if secret.Type != corev1.SecretTypeTLS {
		return fmt.Errorf("secret type must be kubernetes.io/tls, got %s", secret.Type)
	}

	if _, ok := secret.Data["tls.crt"]; !ok {
		return fmt.Errorf("secret missing required key: tls.crt")
	}

	if _, ok := secret.Data["tls.key"]; !ok {
		return fmt.Errorf("secret missing required key: tls.key")
	}

	// Validate certificate can be parsed
	block, _ := pem.Decode(secret.Data["tls.crt"])
	if block == nil {
		return fmt.Errorf("failed to decode PEM certificate")
	}

	_, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	return nil
}

// needsCertificateInstallation checks if the certificate needs to be installed on the BMC.
// It compares the certificate in the secret with the one installed on the BMC.
func (r *BMCReconciler) needsCertificateInstallation(ctx context.Context, bmcObj *metalv1alpha1.BMC, certPEM []byte) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	// Parse the certificate from the secret
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return false, fmt.Errorf("failed to decode PEM certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Check if certificate is expiring soon
	if time.Until(cert.NotAfter) < DefaultCertificateRenewalBuffer {
		log.V(1).Info("Certificate expiring soon",
			"expiresAt", cert.NotAfter,
			"daysRemaining", time.Until(cert.NotAfter).Hours()/24)
		return true, nil
	}

	// Get BMC client to check installed certificate
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.DefaultProtocol, r.SkipCertValidation, r.BMCOptions)
	if err != nil {
		log.V(1).Info("Could not get BMC client to check certificate", "error", err)
		return true, nil // Install if we can't check
	}
	defer bmcClient.Logout()

	// Get certificate info from BMC
	certInfo, err := bmcClient.GetCertificateInfo(ctx)
	if err != nil {
		log.V(1).Info("Could not get certificate info from BMC", "error", err)
		return true, nil // Install if we can't check
	}

	// Compare serial numbers
	if certInfo != nil && certInfo.SerialNumber == cert.SerialNumber.String() {
		log.V(1).Info("Certificate already installed with matching serial number")
		return false, nil
	}

	log.V(1).Info("Certificate needs installation",
		"reason", "serial number mismatch or no certificate installed")
	return true, nil
}

// installCertificateOnBMC installs the certificate on the BMC via Redfish.
func (r *BMCReconciler) installCertificateOnBMC(ctx context.Context, bmcObj *metalv1alpha1.BMC, certPEM, keyPEM []byte) error {
	log := ctrl.LoggerFrom(ctx)

	// Get BMC client
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.DefaultProtocol, r.SkipCertValidation, r.BMCOptions)
	if err != nil {
		return fmt.Errorf("failed to get BMC client: %w", err)
	}
	defer bmcClient.Logout()

	// Install certificate via Redfish
	if err := bmcClient.ReplaceCertificate(ctx, string(certPEM), string(keyPEM)); err != nil {
		return fmt.Errorf("failed to replace certificate via Redfish: %w", err)
	}

	log.Info("Certificate installed on BMC successfully")
	return nil
}

// handleCertificateError sets the certificate condition to failed and logs the error.
func (r *BMCReconciler) handleCertificateError(ctx context.Context, bmcObj *metalv1alpha1.BMC, err error, message string) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Error(err, message)

	if condErr := r.setCertificateCondition(ctx, bmcObj,
		corev1.ConditionFalse,
		metalv1alpha1.BMCCertificateReadyReasonFailed,
		fmt.Sprintf("%s: %v", message, err)); condErr != nil {
		log.Error(condErr, "Failed to update certificate condition")
	}

	// Return error to trigger retry
	return ctrl.Result{}, fmt.Errorf("%s: %w", message, err)
}

// setCertificateCondition updates the CertificateReady condition on the BMC status.
func (r *BMCReconciler) setCertificateCondition(ctx context.Context, bmcObj *metalv1alpha1.BMC, status corev1.ConditionStatus, reason, message string) error {
	log := ctrl.LoggerFrom(ctx)

	if err := r.updateConditions(ctx, bmcObj, true, metalv1alpha1.BMCCertificateReadyCondition, status, reason, message); err != nil {
		return fmt.Errorf("failed to update certificate condition: %w", err)
	}

	log.V(1).Info("Updated certificate condition",
		"status", status,
		"reason", reason)

	return nil
}
