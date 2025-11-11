#!/usr/bin/env bash
#// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
#// SPDX-License-Identifier: Apache-2.0

update_settings(k8s_upsert_timeout_secs=60)  # on first tilt up, often can take longer than 30 seconds

settings = {
    "allowed_contexts": [
        "kind-metal"
    ],
    "kubectl": "./bin/kubectl",
    "local_boot_operator": "", # example: "/path/to/boot-operator"
    "local_maintenance_operator": "", # example: "/path/to/maintenance-operator"
    "boot_image": "ghcr.io/ironcore-dev/boot-operator:latest",
    "cert_manager_version": "v1.15.3",
    "new_args": {
        "boot": [
        "--health-probe-bind-address=:8081",
        "--metrics-bind-address=127.0.0.1:8085",
        "--leader-elect",
        "--ipxe-service-url=http://boot-service:30007",
        "--ipxe-service-port=30007",
        "--controllers=ipxebootconfig,serverbootconfigpxe,serverbootconfighttp,httpbootconfig",
        ],
        "maintenance": [
        "--health-probe-bind-address=:8081",
        "--metrics-bind-address=127.0.0.1:8085",
        ],
        "metal": [
         "--health-probe-bind-address=:8081",
         "--metrics-bind-address=127.0.0.1:8080",
         "--leader-elect",
         "--mac-prefixes-file=/etc/macdb/macdb.yaml",
         "--probe-image=ghcr.io/ironcore-dev/metalprobe:latest",
         "--probe-os-image=ghcr.io/ironcore-dev/os-images/gardenlinux:1877.0",
         "--registry-url=http://127.0.0.1:30000",
         "--registry-port=30000",
         "--enforce-first-boot",
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

yaml_metal = kustomize('./config/dev')
new_args = settings.get("new_args").get("metal")
if new_args:
    yaml_metal = encode_yaml_stream(decode_yaml_stream(str(yaml_metal).replace("- args: []", "- args: {}".format(new_args))))
    print("default metal yaml {}\n".format(yaml_metal))
k8s_yaml(yaml_metal)

if settings.get("local_boot_operator") != "":
    local_boot_operator = settings.get("local_boot_operator")
    print("Using local boot-operator from {}".format(local_boot_operator))
    docker_build('boot-controller', local_boot_operator)
    yaml_boot = kustomize(local_boot_operator + "/config/dev")
    new_args = settings.get("new_args").get("boot")
    if new_args:
        yaml_boot = encode_yaml_stream(decode_yaml_stream(str(yaml_boot).replace("- args: []", "- args: {}".format(new_args))))
        print("default boot yaml {}\n".format(yaml_boot))
    k8s_yaml(yaml_boot)
else:
    print("Using remote boot-operator image {}".format(settings.get("boot_image")))
    deploy_boot()

if settings.get("local_maintenance_operator") != "":
    local_maintenance_operator = settings.get("local_maintenance_operator")
    print("Using local maintenance-operator from {}".format(local_maintenance_operator))
    docker_build('maintenance-controller', local_maintenance_operator)
    yaml_maintenance = kustomize(local_maintenance_operator + '/config/dev')
    new_args = settings.get("new_args").get("maintenance")
    if new_args:
        yaml_maintenance = encode_yaml_stream(decode_yaml_stream(str(yaml_maintenance).replace("- args: []", "- args: {}".format(new_args))))
        print("default maintenance yaml {}\n".format(yaml_maintenance))
    k8s_yaml(yaml_maintenance)
else:
    print("Using remote maintenance-operator image {}".format(settings.get("boot_image")))
    # Use the remote images once available
    # deploy_boot_operator()