// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"net"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// DefaultRSAKeySize is the default RSA key size for CSR generation.
	DefaultRSAKeySize = 2048

	// CertificateSecretNamePrefix is the prefix for certificate secret names.
	CertificateSecretNamePrefix = "bmc-cert-"

	// CertificateRequestNamePrefix is the prefix for CertificateRequest names.
	CertificateRequestNamePrefix = "bmc-certreq-"

	// DefaultCertificateRenewalBuffer is the time before expiration to renew certificates.
	DefaultCertificateRenewalBuffer = 30 * 24 * time.Hour // 30 days
)

// reconcileCertificate is the main entry point for certificate management logic.
// It orchestrates CSR generation, CertificateRequest creation, and certificate installation.
func (r *BMCReconciler) reconcileCertificate(ctx context.Context, bmcObj *metalv1alpha1.BMC) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Skip if certificate management is not enabled
	if bmcObj.Spec.Certificate == nil {
		log.V(1).Info("Certificate management not configured for BMC")
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Starting certificate reconciliation")

	// Check if existing certificate is valid
	if bmcObj.Status.CertificateSecretRef != nil {
		valid, err := r.verifyCertificateValidity(ctx, bmcObj)
		if err != nil {
			log.Error(err, "Failed to verify certificate validity")
			// Continue with renewal on verification error
		} else if valid {
			log.V(1).Info("Existing certificate is valid")
			return ctrl.Result{}, r.setCertificateCondition(ctx, bmcObj, corev1.ConditionTrue,
				metalv1alpha1.BMCCertificateReadyReasonIssued, "Certificate is valid")
		}
		log.V(1).Info("Certificate expired or invalid, renewing")
	}

	// Check if there's an existing CertificateRequest
	if bmcObj.Status.CertificateRequestName != "" {
		certReq := &certmanagerv1.CertificateRequest{}
		err := r.Get(ctx, client.ObjectKey{
			Name:      bmcObj.Status.CertificateRequestName,
			Namespace: r.ManagerNamespace,
		}, certReq)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("failed to get CertificateRequest: %w", err)
			}
			// CertificateRequest not found, create new one
			log.V(1).Info("CertificateRequest not found, creating new one")
		} else if r.isCertificateRequestReady(certReq) {
			// Certificate is ready, install it
			log.V(1).Info("CertificateRequest is ready, installing certificate")
			return r.installAndStoreCertificate(ctx, bmcObj, certReq)
		} else if r.isCertificateRequestFailed(certReq) {
			// Request failed, set condition and delete for recreation
			log.V(1).Info("CertificateRequest failed, setting condition and deleting for recreation")

			// Set Failed condition before deletion
			if err := r.setCertificateCondition(ctx, bmcObj, corev1.ConditionFalse,
				metalv1alpha1.BMCCertificateReadyReasonFailed, "Certificate request failed"); err != nil {
				return ctrl.Result{}, err
			}

			// Delete the failed request
			if err := r.Delete(ctx, certReq); err != nil && !apierrors.IsNotFound(err) {
				log.Error(err, "Failed to delete failed CertificateRequest")
				return ctrl.Result{Requeue: true}, err
			}
			// Return to allow deletion to complete before creating new request
			return ctrl.Result{Requeue: true}, nil
		} else {
			// Request is still pending
			log.V(1).Info("CertificateRequest is pending")
			return r.setCertificateConditionWithRequeue(ctx, bmcObj, corev1.ConditionFalse,
				metalv1alpha1.BMCCertificateReadyReasonPending, "Certificate request is pending", 30*time.Second)
		}
	}

	// Generate or get CSR
	csrBytes, privateKey, source, err := r.getOrGenerateCSR(ctx, bmcObj)
	if err != nil {
		return r.handleCertificateError(ctx, bmcObj, err, "Failed to generate CSR")
	}
	log.V(1).Info("Generated CSR", "source", source)

	// Create CertificateRequest
	certReq, err := r.createOrGetCertificateRequest(ctx, bmcObj, csrBytes, privateKey)
	if err != nil {
		return r.handleCertificateError(ctx, bmcObj, err, "Failed to create CertificateRequest")
	}

	// Update status with CertificateRequest name
	if bmcObj.Status.CertificateRequestName != certReq.Name {
		bmcBase := bmcObj.DeepCopy()
		bmcObj.Status.CertificateRequestName = certReq.Name
		if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update CertificateRequest name in status: %w", err)
		}
	}

	log.V(1).Info("Created CertificateRequest, waiting for issuance")
	return r.setCertificateConditionWithRequeue(ctx, bmcObj, corev1.ConditionFalse,
		metalv1alpha1.BMCCertificateReadyReasonPending, "Certificate request created, waiting for issuance", 30*time.Second)
}

// getOrGenerateCSR attempts to get a CSR from the BMC first, then falls back to operator generation.
func (r *BMCReconciler) getOrGenerateCSR(ctx context.Context, bmcObj *metalv1alpha1.BMC) (csrPEM []byte, privateKey *rsa.PrivateKey, source string, err error) {
	log := ctrl.LoggerFrom(ctx)

	// Try BMC-generated CSR first (more secure)
	csrPEM, err = r.tryBMCGeneratedCSR(ctx, bmcObj)
	if err == nil && len(csrPEM) > 0 {
		log.V(1).Info("Successfully generated CSR from BMC")
		return csrPEM, nil, "BMC", nil
	}
	log.V(1).Info("BMC CSR generation failed or unsupported, falling back to operator generation", "error", err)

	// Fallback to operator-generated CSR
	csrPEM, privateKey, err = r.generateOperatorCSR(ctx, bmcObj)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to generate operator CSR: %w", err)
	}
	return csrPEM, privateKey, "Operator", nil
}

// tryBMCGeneratedCSR attempts to generate a CSR using the BMC's native functionality.
func (r *BMCReconciler) tryBMCGeneratedCSR(ctx context.Context, bmcObj *metalv1alpha1.BMC) ([]byte, error) {
	log := ctrl.LoggerFrom(ctx)

	// Connect to BMC with insecure=true since cert not installed yet
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, true, r.BMCOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to BMC: %w", err)
	}
	defer bmcClient.Logout()

	// Build CSR parameters from spec
	params := r.buildCSRParameters(bmcObj)
	log.V(1).Info("Requesting CSR from BMC", "commonName", params.CommonName)

	// Call BMC to generate CSR
	csrPEM, err := bmcClient.GenerateCSR(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("BMC CSR generation failed: %w", err)
	}

	return csrPEM, nil
}

// generateOperatorCSR generates a CSR and private key using the operator.
func (r *BMCReconciler) generateOperatorCSR(ctx context.Context, bmcObj *metalv1alpha1.BMC) ([]byte, *rsa.PrivateKey, error) {
	log := ctrl.LoggerFrom(ctx)

	// Generate RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, DefaultRSAKeySize)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Build certificate request template
	commonName := r.getCertificateCommonName(bmcObj)
	log.V(1).Info("Generating CSR", "commonName", commonName)

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: commonName,
		},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}

	// Add DNS names
	if len(bmcObj.Spec.Certificate.DNSNames) > 0 {
		template.DNSNames = bmcObj.Spec.Certificate.DNSNames
	}

	// Add IP addresses
	ipAddresses, err := r.getCertificateIPAddresses(ctx, bmcObj)
	if err != nil {
		// Log error but don't fail - certificate can still be valid without IP SANs
		// This is especially important if status.IP hasn't been populated yet
		log.Error(err, "Failed to parse IP addresses, continuing without them")
		ipAddresses = []net.IP{}
	}
	template.IPAddresses = ipAddresses

	// Create CSR
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate request: %w", err)
	}

	// Encode to PEM
	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	return csrPEM, privateKey, nil
}

// createOrGetCertificateRequest creates a CertificateRequest or returns existing one.
func (r *BMCReconciler) createOrGetCertificateRequest(ctx context.Context, bmcObj *metalv1alpha1.BMC, csrPEM []byte, privateKey *rsa.PrivateKey) (*certmanagerv1.CertificateRequest, error) {
	log := ctrl.LoggerFrom(ctx)

	certReqName := fmt.Sprintf("%s%s", CertificateRequestNamePrefix, bmcObj.Name)

	// Check if CertificateRequest already exists
	existingCertReq := &certmanagerv1.CertificateRequest{}
	err := r.Get(ctx, client.ObjectKey{Name: certReqName, Namespace: r.ManagerNamespace}, existingCertReq)
	if err == nil {
		log.V(1).Info("CertificateRequest already exists", "name", certReqName)
		return existingCertReq, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get CertificateRequest: %w", err)
	}

	// Create new CertificateRequest
	certReq := &certmanagerv1.CertificateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certReqName,
			Namespace: r.ManagerNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "metal-operator",
				"metal.ironcore.dev/bmc":       bmcObj.Name,
			},
		},
		Spec: certmanagerv1.CertificateRequestSpec{
			Request: csrPEM,
			IssuerRef: cmmeta.ObjectReference{
				Name:  bmcObj.Spec.Certificate.IssuerRef.Name,
				Kind:  bmcObj.Spec.Certificate.IssuerRef.Kind,
				Group: bmcObj.Spec.Certificate.IssuerRef.Group,
			},
			Usages: []certmanagerv1.KeyUsage{
				certmanagerv1.UsageDigitalSignature,
				certmanagerv1.UsageKeyEncipherment,
				certmanagerv1.UsageServerAuth,
			},
		},
	}

	// Set duration if specified
	if bmcObj.Spec.Certificate.Duration != nil {
		certReq.Spec.Duration = bmcObj.Spec.Certificate.Duration
	}

	// Set owner reference to BMC
	if err := controllerutil.SetControllerReference(bmcObj, certReq, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Store private key in a temporary secret if generated by operator
	if privateKey != nil {
		if err := r.storePrivateKey(ctx, bmcObj, privateKey); err != nil {
			return nil, fmt.Errorf("failed to store private key: %w", err)
		}
	}

	// Create CertificateRequest
	if err := r.Create(ctx, certReq); err != nil {
		return nil, fmt.Errorf("failed to create CertificateRequest: %w", err)
	}

	log.Info("Created CertificateRequest", "name", certReqName)
	return certReq, nil
}

// isCertificateRequestReady checks if the CertificateRequest is ready.
func (r *BMCReconciler) isCertificateRequestReady(certReq *certmanagerv1.CertificateRequest) bool {
	for _, condition := range certReq.Status.Conditions {
		if condition.Type == certmanagerv1.CertificateRequestConditionReady {
			return condition.Status == cmmeta.ConditionTrue && len(certReq.Status.Certificate) > 0
		}
	}
	return false
}

// isCertificateRequestFailed checks if the CertificateRequest has failed.
func (r *BMCReconciler) isCertificateRequestFailed(certReq *certmanagerv1.CertificateRequest) bool {
	for _, condition := range certReq.Status.Conditions {
		if condition.Type == certmanagerv1.CertificateRequestConditionReady {
			return condition.Status == cmmeta.ConditionFalse && condition.Reason == "Failed"
		}
	}
	return false
}

// installAndStoreCertificate installs the certificate on BMC and stores it in a secret.
func (r *BMCReconciler) installAndStoreCertificate(ctx context.Context, bmcObj *metalv1alpha1.BMC, certReq *certmanagerv1.CertificateRequest) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	certPEM := string(certReq.Status.Certificate)

	// Load private key from secret (if operator-generated)
	privateKey, err := r.loadPrivateKey(ctx, bmcObj)
	if err != nil {
		log.Error(err, "Failed to load private key, attempting installation without it")
	}

	var privateKeyPEM string
	if privateKey != nil {
		privateKeyPEM = string(pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		}))
	}

	// Install certificate on BMC
	if err := r.installCertificateOnBMC(ctx, bmcObj, certPEM, privateKeyPEM); err != nil {
		return r.handleCertificateError(ctx, bmcObj, err, "Failed to install certificate on BMC")
	}
	log.Info("Successfully installed certificate on BMC")

	// Get CA certificate from issuer
	caCertPEM, err := r.getCACertificateFromIssuer(ctx, bmcObj.Spec.Certificate.IssuerRef)
	if err != nil {
		log.Error(err, "Failed to get CA certificate, storing without CA")
	}

	// Store certificate in secret
	secretName, err := r.storeCertificateSecret(ctx, bmcObj, certPEM, privateKeyPEM, caCertPEM)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to store certificate secret: %w", err)
	}

	// Update BMC status with secret reference
	bmcBase := bmcObj.DeepCopy()
	bmcObj.Status.CertificateSecretRef = &corev1.LocalObjectReference{Name: secretName}
	if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update certificate secret reference: %w", err)
	}

	// Clean up temporary private key secret
	if privateKey != nil {
		if err := r.deletePrivateKeySecret(ctx, bmcObj); err != nil {
			log.Error(err, "Failed to delete temporary private key secret")
		}
	}

	log.Info("Certificate installation completed successfully")
	return ctrl.Result{}, r.setCertificateCondition(ctx, bmcObj, corev1.ConditionTrue,
		metalv1alpha1.BMCCertificateReadyReasonIssued, "Certificate installed successfully")
}

// installCertificateOnBMC installs the certificate and private key on the BMC.
func (r *BMCReconciler) installCertificateOnBMC(ctx context.Context, bmcObj *metalv1alpha1.BMC, certPEM, privateKeyPEM string) error {
	log := ctrl.LoggerFrom(ctx)

	// Connect to BMC with insecure=true
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, true, r.BMCOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to BMC: %w", err)
	}
	defer bmcClient.Logout()

	log.V(1).Info("Installing certificate on BMC")
	if err := bmcClient.ReplaceCertificate(ctx, certPEM, privateKeyPEM); err != nil {
		return fmt.Errorf("failed to replace certificate on BMC: %w", err)
	}

	return nil
}

// storeCertificateSecret stores the certificate, private key, and CA certificate in a Kubernetes secret.
func (r *BMCReconciler) storeCertificateSecret(ctx context.Context, bmcObj *metalv1alpha1.BMC, certPEM, privateKeyPEM, caCertPEM string) (string, error) {
	log := ctrl.LoggerFrom(ctx)

	secretName := fmt.Sprintf("%s%s", CertificateSecretNamePrefix, bmcObj.Name)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.ManagerNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "metal-operator",
				"metal.ironcore.dev/bmc":       bmcObj.Name,
			},
		},
		Data: map[string][]byte{},
	}

	// Set secret type based on whether we have a private key
	// SecretTypeTLS requires both tls.crt and tls.key
	// Use SecretTypeOpaque when privateKey is unavailable (BMC-generated CSR case)
	if privateKeyPEM != "" {
		secret.Type = corev1.SecretTypeTLS
	} else {
		secret.Type = corev1.SecretTypeOpaque
	}

	// Add certificate
	if certPEM != "" {
		secret.Data["tls.crt"] = []byte(certPEM)
	}

	// Add private key (if available)
	if privateKeyPEM != "" {
		secret.Data["tls.key"] = []byte(privateKeyPEM)
	}

	// Add CA certificate (if available)
	if caCertPEM != "" {
		secret.Data["ca.crt"] = []byte(caCertPEM)
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(bmcObj, secret, r.Scheme); err != nil {
		return "", fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Create or update secret
	existingSecret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: r.ManagerNamespace}, existingSecret)
	if err == nil {
		// Update existing secret
		existingSecret.Data = secret.Data
		if err := r.Update(ctx, existingSecret); err != nil {
			return "", fmt.Errorf("failed to update secret: %w", err)
		}
		log.V(1).Info("Updated certificate secret", "name", secretName)
	} else if apierrors.IsNotFound(err) {
		// Create new secret
		if err := r.Create(ctx, secret); err != nil {
			return "", fmt.Errorf("failed to create secret: %w", err)
		}
		log.V(1).Info("Created certificate secret", "name", secretName)
	} else {
		return "", fmt.Errorf("failed to get secret: %w", err)
	}

	return secretName, nil
}

// storePrivateKey stores the private key in a temporary secret.
func (r *BMCReconciler) storePrivateKey(ctx context.Context, bmcObj *metalv1alpha1.BMC, privateKey *rsa.PrivateKey) error {
	secretName := fmt.Sprintf("%s%s-key", CertificateSecretNamePrefix, bmcObj.Name)

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.ManagerNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "metal-operator",
				"metal.ironcore.dev/bmc":       bmcObj.Name,
				"metal.ironcore.dev/temporary": "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"tls.key": privateKeyPEM,
		},
	}

	if err := controllerutil.SetControllerReference(bmcObj, secret, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := r.Create(ctx, secret); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create private key secret: %w", err)
	}

	return nil
}

// loadPrivateKey loads the private key from the temporary secret.
func (r *BMCReconciler) loadPrivateKey(ctx context.Context, bmcObj *metalv1alpha1.BMC) (*rsa.PrivateKey, error) {
	secretName := fmt.Sprintf("%s%s-key", CertificateSecretNamePrefix, bmcObj.Name)

	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: r.ManagerNamespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil // No private key stored, BMC generated CSR
		}
		return nil, fmt.Errorf("failed to get private key secret: %w", err)
	}

	privateKeyPEM, ok := secret.Data["tls.key"]
	if !ok {
		return nil, fmt.Errorf("private key not found in secret")
	}

	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return privateKey, nil
}

// deletePrivateKeySecret deletes the temporary private key secret.
func (r *BMCReconciler) deletePrivateKeySecret(ctx context.Context, bmcObj *metalv1alpha1.BMC) error {
	secretName := fmt.Sprintf("%s%s-key", CertificateSecretNamePrefix, bmcObj.Name)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.ManagerNamespace,
		},
	}

	if err := r.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete private key secret: %w", err)
	}

	return nil
}

// verifyCertificateValidity checks if the existing certificate is valid.
func (r *BMCReconciler) verifyCertificateValidity(ctx context.Context, bmcObj *metalv1alpha1.BMC) (bool, error) {
	if bmcObj.Status.CertificateSecretRef == nil {
		return false, nil
	}

	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      bmcObj.Status.CertificateSecretRef.Name,
		Namespace: r.ManagerNamespace,
	}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get certificate secret: %w", err)
	}

	certPEM, ok := secret.Data["tls.crt"]
	if !ok {
		return false, fmt.Errorf("certificate not found in secret")
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return false, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Check if certificate is expired or will expire soon
	now := time.Now()
	renewalTime := cert.NotAfter.Add(-DefaultCertificateRenewalBuffer)
	if now.After(cert.NotAfter) {
		return false, nil // Certificate expired
	}
	if now.After(renewalTime) {
		return false, nil // Certificate will expire soon
	}

	return true, nil
}

// getCACertificateFromIssuer retrieves the CA certificate from the Issuer or ClusterIssuer.
func (r *BMCReconciler) getCACertificateFromIssuer(ctx context.Context, issuerRef metalv1alpha1.CertificateIssuerRef) (string, error) {
	log := ctrl.LoggerFrom(ctx)

	// Determine if we're dealing with a ClusterIssuer or namespaced Issuer
	if issuerRef.Kind == "ClusterIssuer" {
		// Fetch ClusterIssuer
		clusterIssuer := &certmanagerv1.ClusterIssuer{}
		if err := r.Get(ctx, client.ObjectKey{Name: issuerRef.Name}, clusterIssuer); err != nil {
			return "", fmt.Errorf("failed to get ClusterIssuer %s: %w", issuerRef.Name, err)
		}
		return r.extractCAFromIssuerSpec(ctx, &clusterIssuer.Spec, "", issuerRef.Name)
	}

	// Fetch namespaced Issuer
	issuer := &certmanagerv1.Issuer{}
	// Note: Issuer must be in the same namespace as the BMC
	// For cluster-scoped BMC resources, we would need to determine the namespace differently
	if err := r.Get(ctx, client.ObjectKey{Name: issuerRef.Name, Namespace: r.ManagerNamespace}, issuer); err != nil {
		log.V(1).Info("Failed to get Issuer, trying without namespace", "issuer", issuerRef.Name, "error", err)
		// Try without namespace for cluster-scoped resources
		if err := r.Get(ctx, client.ObjectKey{Name: issuerRef.Name}, issuer); err != nil {
			return "", fmt.Errorf("failed to get Issuer %s: %w", issuerRef.Name, err)
		}
	}
	return r.extractCAFromIssuerSpec(ctx, &issuer.Spec, issuer.Namespace, issuerRef.Name)
}

// extractCAFromIssuerSpec extracts CA certificate from an issuer spec.
func (r *BMCReconciler) extractCAFromIssuerSpec(ctx context.Context, spec *certmanagerv1.IssuerSpec, namespace, issuerName string) (string, error) {
	log := ctrl.LoggerFrom(ctx)

	// Handle CA issuer type
	if spec.CA != nil && spec.CA.SecretName != "" {
		return r.getCAFromSecret(ctx, spec.CA.SecretName, namespace)
	}

	// Handle SelfSigned issuer - self-signed certs don't have a separate CA
	if spec.SelfSigned != nil {
		log.V(1).Info("SelfSigned issuer detected, no separate CA certificate available", "issuer", issuerName)
		return "", nil
	}

	// Handle ACME issuer - ACME certificates are signed by Let's Encrypt or other ACME providers
	if spec.ACME != nil {
		log.V(1).Info("ACME issuer detected, CA certificates are managed by ACME provider", "issuer", issuerName)
		// ACME providers like Let's Encrypt have well-known CA certificates
		// In practice, the system's trusted CA bundle will handle these
		return "", nil
	}

	// Handle Vault issuer
	if spec.Vault != nil {
		log.V(1).Info("Vault issuer detected, CA certificate retrieval from Vault not yet implemented", "issuer", issuerName)
		// Vault CA would need to be retrieved from Vault's PKI backend
		// This would require additional Vault API calls
		return "", nil
	}

	// Handle Venafi issuer
	if spec.Venafi != nil {
		log.V(1).Info("Venafi issuer detected, CA certificate retrieval from Venafi not yet implemented", "issuer", issuerName)
		return "", nil
	}

	log.V(1).Info("Issuer type does not provide CA certificate or is not yet supported", "issuer", issuerName)
	return "", nil
}

// getCAFromSecret retrieves the CA certificate from a Kubernetes Secret.
func (r *BMCReconciler) getCAFromSecret(ctx context.Context, secretName, namespace string) (string, error) {
	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: secretName, Namespace: namespace}

	if err := r.Get(ctx, key, secret); err != nil {
		return "", fmt.Errorf("failed to get CA secret %s/%s: %w", namespace, secretName, err)
	}

	// Try common CA certificate keys
	caCertKeys := []string{"ca.crt", "tls.crt", "ca-cert.pem"}
	for _, key := range caCertKeys {
		if caCert, exists := secret.Data[key]; exists && len(caCert) > 0 {
			return string(caCert), nil
		}
	}

	return "", fmt.Errorf("no CA certificate found in secret %s/%s (tried keys: %v)", namespace, secretName, caCertKeys)
}

// buildCSRParameters builds CSR parameters from BMC spec.
func (r *BMCReconciler) buildCSRParameters(bmcObj *metalv1alpha1.BMC) bmc.CSRParameters {
	params := bmc.CSRParameters{
		CommonName:       r.getCertificateCommonName(bmcObj),
		KeyPairAlgorithm: "RSA",
		KeyBitLength:     DefaultRSAKeySize,
	}

	// Add alternative names (DNS + IPs)
	if len(bmcObj.Spec.Certificate.DNSNames) > 0 {
		params.AlternativeNames = append(params.AlternativeNames, bmcObj.Spec.Certificate.DNSNames...)
	}
	if len(bmcObj.Spec.Certificate.IPAddresses) > 0 {
		params.AlternativeNames = append(params.AlternativeNames, bmcObj.Spec.Certificate.IPAddresses...)
	}

	return params
}

// getCertificateCommonName returns the common name for the certificate.
func (r *BMCReconciler) getCertificateCommonName(bmcObj *metalv1alpha1.BMC) string {
	// Use explicit CN if specified
	if bmcObj.Spec.Certificate.CommonName != "" {
		return bmcObj.Spec.Certificate.CommonName
	}

	// Use hostname if available
	if bmcObj.Spec.Hostname != nil && *bmcObj.Spec.Hostname != "" {
		return *bmcObj.Spec.Hostname
	}

	// Use first DNS name if available
	if len(bmcObj.Spec.Certificate.DNSNames) > 0 {
		return bmcObj.Spec.Certificate.DNSNames[0]
	}

	// Use BMC name as fallback
	return bmcObj.Name
}

// getCertificateIPAddresses parses IP addresses from the BMC spec.
func (r *BMCReconciler) getCertificateIPAddresses(_ context.Context, bmcObj *metalv1alpha1.BMC) ([]net.IP, error) {
	var ips []net.IP

	// Add IPs from certificate spec
	for _, ipStr := range bmcObj.Spec.Certificate.IPAddresses {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address: %s", ipStr)
		}
		ips = append(ips, ip)
	}

	// Add BMC IP if not already included
	bmcIP := bmcObj.Status.IP.String()
	if bmcIP != "" {
		ip := net.ParseIP(bmcIP)
		if ip != nil {
			// Check if already included
			found := false
			for _, existingIP := range ips {
				if existingIP.Equal(ip) {
					found = true
					break
				}
			}
			if !found {
				ips = append(ips, ip)
			}
		}
	}

	return ips, nil
}

// setCertificateCondition sets the CertificateReady condition.
func (r *BMCReconciler) setCertificateCondition(ctx context.Context, bmcObj *metalv1alpha1.BMC, status corev1.ConditionStatus, reason, message string) error {
	return r.updateConditions(ctx, bmcObj, true, metalv1alpha1.BMCCertificateReadyCondition, status, reason, message)
}

// setCertificateConditionWithRequeue sets the CertificateReady condition and returns a requeue result.
func (r *BMCReconciler) setCertificateConditionWithRequeue(ctx context.Context, bmcObj *metalv1alpha1.BMC, status corev1.ConditionStatus, reason, message string, requeueAfter time.Duration) (ctrl.Result, error) {
	if err := r.setCertificateCondition(ctx, bmcObj, status, reason, message); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// handleCertificateError handles certificate errors with graceful degradation.
func (r *BMCReconciler) handleCertificateError(ctx context.Context, bmcObj *metalv1alpha1.BMC, err error, reason string) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	log.Error(err, reason)

	// Set condition to failed
	if condErr := r.setCertificateCondition(ctx, bmcObj, corev1.ConditionFalse,
		metalv1alpha1.BMCCertificateReadyReasonFailed, fmt.Sprintf("%s: %v", reason, err)); condErr != nil {
		return ctrl.Result{}, condErr
	}

	// Continue with Insecure=true (graceful degradation)
	log.V(1).Info("Certificate management failed, continuing with insecure connection")

	// Requeue after configured interval to retry
	return ctrl.Result{RequeueAfter: r.CertificateRetryInterval}, nil
}
