#!/usr/bin/env bash
#// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
#// SPDX-License-Identifier: Apache-2.0

update_settings(k8s_upsert_timeout_secs=60)  # on first tilt up, often can take longer than 30 seconds

settings = {
    "allowed_contexts": [
        "kind-metal"
    ],
    "kubectl": "./bin/kubectl",
    "boot_image": "ghcr.io/ironcore-dev/boot-operator:latest",
    "cert_manager_version": "v1.15.3",
    "new_args": {
        "boot": [
        "--health-probe-bind-address=:8081",
        "--metrics-bind-address=127.0.0.1:8085",
        "--leader-elect",
#        "--default-ipxe-server-url=http://boot-service:30007",
#        "--default-kernel-url=http://boot-service:30007/image?imageName=ghcr.io/ironcore-dev/os-images/gardenlinux&version=1443.10&layerName=vmlinuz",
#        "--default-initrd-url=http://boot-service:30007/image?imageName=ghcr.io/ironcore-dev/os-images/gardenlinux&version=1443.10&layerName=initramfs",
#        "--default-squashfs-url=http://boot-service:30007/image?imageName=ghcr.io/ironcore-dev/os-images/gardenlinux&version=1443.10&layerName=squashfs",
        "--ipxe-service-url=http://boot-service:30007",
        "--ipxe-service-port=30007",
        "--controllers=ipxebootconfig,serverbootconfigpxe,serverbootconfighttp,httpbootconfig",
        ],
    }
}

kubectl = settings.get("kubectl")

if "allowed_contexts" in settings:
    allow_k8s_contexts(settings.get("allowed_contexts"))

def deploy_cert_manager():
    version = settings.get("cert_manager_version")
    print("Installing cert-manager")
    local("{} apply -f https://github.com/cert-manager/cert-manager/releases/download/{}/cert-manager.yaml".format(kubectl, version), quiet=True, echo_off=True)

    print("Waiting for cert-manager to start")
    local("{} wait --for=condition=Available --timeout=300s -n cert-manager deployment/cert-manager".format(kubectl), quiet=True, echo_off=True)
    local("{} wait --for=condition=Available --timeout=300s -n cert-manager deployment/cert-manager-cainjector".format(kubectl), quiet=True, echo_off=True)
    local("{} wait --for=condition=Available --timeout=300s -n cert-manager deployment/cert-manager-webhook".format(kubectl), quiet=True, echo_off=True)

# deploy boot-operator
def deploy_boot():
    version = settings.get("boot_version")
    image = settings.get("boot_image")
    new_args = settings.get("new_args").get("boot")
    boot_uri = "https://github.com/ironcore-dev/boot-operator//config/dev"
    cmd = "{} apply -k {}".format(kubectl, boot_uri)
    local(cmd, quiet=True)

    replace_args_with_new_args("boot-operator-system", "boot-operator-controller-manager", new_args)
    patch_image("boot-operator-system", "boot-operator-controller-manager", image)

def patch_image(namespace, name, image):
    patch = [{
        "op": "replace",
        "path": "/spec/template/spec/containers/0/image",
        "value": image,
    }]
    local("{} patch deployment {} -n {} --type json -p='{}'".format(kubectl, name, namespace, str(encode_json(patch)).replace("\n", "")))

def replace_args_with_new_args(namespace, name, extra_args):
    patch = [{
        "op": "replace",
        "path": "/spec/template/spec/containers/0/args",
        "value": extra_args,
    }]
    local("{} patch deployment {} -n {} --type json -p='{}'".format(kubectl, name, namespace, str(encode_json(patch)).replace("\n", "")))

def waitforsystem():
    print("Waiting for metal-operator to start")
    local("{} wait --for=condition=ready --timeout=300s -n metal-operator-system pod --all".format(kubectl), quiet=False, echo_off=True)

##############################
# Actual work happens here
##############################

deploy_cert_manager()

docker_build('controller', '.', target = 'manager')

deploy_boot()

yaml = kustomize('./config/dev')

k8s_yaml(yaml)
