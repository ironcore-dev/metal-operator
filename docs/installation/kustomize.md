# Kustomize Installation

This guide will help you install the Metal Operator using Kustomize.

## Prerequisites

- Kubernetes cluster (v1.30+)
- Kustomize (v5.0.0+)
- Docker (17.03+)
- kubectl (v1.30+)

## Install from git

Install the Metal Operator directly from the GitHub repository using Kustomize.

### Steps

1. **Clone the Repository**

   First, clone the Metal Operator repository to your local machine.

   ```sh
   git clone https://github.com/ironcore-dev/metal-operator.git
   ```
   
2. **Navigate to the Project Directory**

    Change into the Metal Operator directory.
    
    ```sh
    cd metal-operator
    ```
   
3. **Install CRDs**

   Apply the Custom Resource Definitions (CRDs) to your Kubernetes cluster.

   ```sh
   make install
   ```

4. **Deploy the Metal Operator**

   Use Kustomize to deploy the Metal Operator to your Kubernetes cluster.

   ```sh
   make deploy IMG=ghcr.io/ironcore-dev/metal-operator:latest
   ```

### Uninstall

To uninstall the Metal Operator and remove all associated resources from your Kubernetes cluster, follow these steps:

First, make sure no custom resources are using the CRDs; otherwise, the uninstall will fail.
Next, delete the installed APIs (CRDs) and the controller.

Delete the APIs(CRDs) from the cluster:

```sh
make uninstall
```

Uninstall the controller from the cluster:

```sh
make undeploy
```
