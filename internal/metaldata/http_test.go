// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package metaldata_test

import (
	"encoding/json"
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

const (
	flavorHeader = "Metadata-Flavor"
	flavorValue  = "IronCore Metal"

	testNamespace = "default"
	testClaimName = "claim-a"
	testSecretRef = "user-data-a"
)

var _ = Describe("HTTP server", func() {
	Describe("metadata-flavor middleware", func() {
		It("rejects requests without the Metadata-Flavor header", func() {
			resp, err := http.Get(testServerURL + "/v1/")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(resp.Body.Close)
			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
		})

		It("rejects requests with an incorrect Metadata-Flavor value", func() {
			req, err := http.NewRequest(http.MethodGet, testServerURL+"/v1/", nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set(flavorHeader, "Google")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(resp.Body.Close)
			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
		})
	})

	Describe("when the client IP is not indexed", func() {
		BeforeEach(func() {
			reader.reset()
		})

		It("returns 404 for /v1/", func() {
			resp := getMetadata("/v1/")
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})

		It("returns 404 for /v1/{field}", func() {
			resp := getMetadata("/v1/server-name")
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})

		It("returns 404 for /v1/user-data", func() {
			resp := getMetadata("/v1/user-data")
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Describe("when the client IP is indexed", func() {
		var server *metalv1alpha1.Server

		BeforeEach(func() {
			server = newServer("server-loopback", "127.0.0.1")
			server.Labels = map[string]string{
				metalv1alpha1.MetadataKeyPrefix + "rack": "rack-1",
			}
			idx.EventHandler().AddFunc(server)
			reader.reset()
		})

		AfterEach(func() {
			idx.EventHandler().DeleteFunc(server)
		})

		Describe("/v1/", func() {
			It("returns the metadata as JSON", func() {
				resp := getMetadata("/v1/")
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))

				body := decodeJSON(resp)
				Expect(body).To(HaveKeyWithValue("server-name", "server-loopback"))
				Expect(body).To(HaveKeyWithValue("rack", "rack-1"))
				Expect(body).To(HaveKeyWithValue(staticKey, staticVal))
			})

			It("does not include user-data when the Server is unclaimed", func() {
				resp := getMetadata("/v1/")
				body := decodeJSON(resp)
				Expect(body).NotTo(HaveKey("user-data"))
			})
		})

		Describe("/v1/{field}", func() {
			It("returns the field value as plain text", func() {
				resp := getMetadata("/v1/server-name")
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(resp.Header.Get("Content-Type")).To(Equal("text/plain; charset=utf-8"))
				Expect(readBody(resp)).To(Equal("server-loopback"))
			})

			It("returns 404 for an unknown field", func() {
				resp := getMetadata("/v1/does-not-exist")
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
			})
		})

		Describe("/v1/user-data", func() {
			It("returns 404 when the Server has no ServerClaim", func() {
				resp := getMetadata("/v1/user-data")
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
			})

			When("the Server is claimed", func() {
				BeforeEach(func() {
					idx.EventHandler().DeleteFunc(server)
					server = newServer("server-loopback", "127.0.0.1")
					server.Spec.ServerClaimRef = &metalv1alpha1.ImmutableObjectReference{
						Namespace: testNamespace,
						Name:      testClaimName,
					}
					idx.EventHandler().AddFunc(server)
				})

				It("returns 404 when the ServerClaim does not exist", func() {
					resp := getMetadata("/v1/user-data")
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				})

				It("returns 404 when the ServerClaim has no UserDataRef", func() {
					reader.reset(claim(nil))
					resp := getMetadata("/v1/user-data")
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				})

				It("returns 404 when the referenced Secret does not exist", func() {
					reader.reset(claim(&corev1.LocalObjectReference{Name: testSecretRef}))
					resp := getMetadata("/v1/user-data")
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				})

				It("returns 500 when the referenced Secret has the wrong type", func() {
					reader.reset(
						claim(&corev1.LocalObjectReference{Name: testSecretRef}),
						userDataSecret(corev1.SecretTypeOpaque, map[string][]byte{
							"key": []byte("value"),
						}),
					)
					resp := getMetadata("/v1/user-data")
					Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
				})

				It("returns the Secret data as JSON", func() {
					reader.reset(
						claim(&corev1.LocalObjectReference{Name: testSecretRef}),
						userDataSecret(metalv1alpha1.SecretTypeUserData, map[string][]byte{
							"key1": []byte("value1"),
							"key2": []byte("value2"),
						}),
					)
					resp := getMetadata("/v1/user-data")
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))

					body := decodeJSON(resp)
					Expect(body).To(HaveKeyWithValue("key1", "value1"))
					Expect(body).To(HaveKeyWithValue("key2", "value2"))
				})

				It("includes user-data in the /v1/ response", func() {
					reader.reset(
						claim(&corev1.LocalObjectReference{Name: testSecretRef}),
						userDataSecret(metalv1alpha1.SecretTypeUserData, map[string][]byte{
							"key": []byte("value"),
						}),
					)
					resp := getMetadata("/v1/")
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					body := decodeJSON(resp)
					Expect(body).To(HaveKey("user-data"))
					Expect(body["user-data"]).To(Equal(map[string]any{"key": "value"}))
				})
			})
		})

		Describe("/v1/user-data/{key}", func() {
			BeforeEach(func() {
				idx.EventHandler().DeleteFunc(server)
				server = newServer("server-loopback", "127.0.0.1")
				server.Spec.ServerClaimRef = &metalv1alpha1.ImmutableObjectReference{
					Namespace: testNamespace,
					Name:      testClaimName,
				}
				idx.EventHandler().AddFunc(server)
				reader.reset(
					claim(&corev1.LocalObjectReference{Name: testSecretRef}),
					userDataSecret(metalv1alpha1.SecretTypeUserData, map[string][]byte{
						"key1": []byte("value1"),
					}),
				)
			})

			It("returns the value for an existing key as plain text", func() {
				resp := getMetadata("/v1/user-data/key1")
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(resp.Header.Get("Content-Type")).To(Equal("text/plain; charset=utf-8"))
				Expect(readBody(resp)).To(Equal("value1"))
			})

			It("returns 404 for an unknown key", func() {
				resp := getMetadata("/v1/user-data/missing")
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
			})
		})

		Describe("route precedence", func() {
			It("routes /v1/user-data to the user-data handler even when a metadata key with that name is set", func() {
				idx.EventHandler().DeleteFunc(server)
				server = newServer("server-loopback", "127.0.0.1")
				server.Labels = map[string]string{
					metalv1alpha1.MetadataKeyPrefix + "user-data": "from-label",
				}
				idx.EventHandler().AddFunc(server)

				resp := getMetadata("/v1/user-data")
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
			})
		})
	})
})

func getMetadata(path string) *http.Response {
	GinkgoHelper()
	req, err := http.NewRequest(http.MethodGet, testServerURL+path, nil)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set(flavorHeader, flavorValue)

	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(resp.Body.Close)
	return resp
}

func decodeJSON(resp *http.Response) map[string]any {
	GinkgoHelper()
	out := map[string]any{}
	Expect(json.NewDecoder(resp.Body).Decode(&out)).To(Succeed())
	return out
}

func readBody(resp *http.Response) string {
	GinkgoHelper()
	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	return string(body)
}

func claim(userDataRef *corev1.LocalObjectReference) *metalv1alpha1.ServerClaim {
	return &metalv1alpha1.ServerClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testClaimName,
		},
		Spec: metalv1alpha1.ServerClaimSpec{
			Power:       metalv1alpha1.PowerOn,
			Image:       "image",
			UserDataRef: userDataRef,
		},
	}
}

func userDataSecret(secretType corev1.SecretType, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testSecretRef,
		},
		Type: secretType,
		Data: data,
	}
}
