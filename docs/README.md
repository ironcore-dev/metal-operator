# Metal-Operator Documentation

**Welcome to the Metal-Operator Documentation!**

The `metal-operator` is a Kubernetes-native operator, part of the IronCore open-source project, designed for robust bare metal infrastructure management. By leveraging Baseboard Management Controllers (BMCs) and the Redfish API, `metal-operator` enables streamlined and automated server discovery, provisioning, and lifecycle management. Using the Kubernetes Controller pattern, `metal-operator` provides a CRD-based operational model that standardizes bare metal management across different hardware environments. Integration with vendor-specific tooling is also possible for enhanced functionality when needed.

---

## Key Features

### 1. **Discover and Onboard Bare Metal Servers**
- Automatically detect and register bare metal servers through BMCs and the Redfish API.
- Efficiently gather hardware specs, network configurations, and initial health checks directly from BMC interfaces.

### 2. **Provision Software on Bare Metal Servers**
- Deploy and configure software on registered servers using BMC interactions and standardized provisioning workflows.
- Support for dynamic software configuration and Redfish API-based management for consistent, vendor-neutral provisioning.

### 3. **Manage Server Reservations**
- Reserve specific bare metal resources based on workload needs.
- Prevent resource conflicts by managing reservations via Kubernetes-native CRDs, ensuring that workloads align with available hardware resources.

### 4. **Perform Day 2 Operations**
- Utilize the Redfish API to manage BIOS, firmware, and driver updates.
- Automate ongoing maintenance tasks and operational workflows to maintain infrastructure resilience and uptime.

### 5. **Decommission and Maintain Faulty Servers**
- Decommission servers via BMC controls for clean removal from active pools.
- Schedule and perform maintenance tasks with BMC data to optimize uptime and maintain hardware reliability.

---

## How It Works

The `metal-operator` relies on **BMCs and the Redfish API** to handle bare metal server management tasks. Through a CRD-based operational model, `metal-operator` provides Kubernetes-native management of bare metal infrastructure, enabling consistent, vendor-neutral interactions.

### Core Components
- **Custom Resources (CRs)**: Extend Kubernetes to manage server configurations, reservations, and operational workflows.
- **Controllers**: Automate lifecycle management through Redfish-enabled interactions, from provisioning to decommissioning.
- **Reconcilers**: Ensure the desired state matches the actual state by continuously monitoring hardware via BMC integrations.

### [Architecture Overview](architecture.md)

1. **Discovery**: Register new bare metal servers through BMCs and Redfish API, creating CRDs for streamlined management.
2. **Provisioning**: Apply software images and configurations using Redfish API, based on templates or custom configurations.
3. **Operations**: Execute BIOS, firmware updates, and other maintenance tasks through standardized workflows.
4. **Decommissioning**: Safely remove or maintain servers using Redfish and BMC controls, marking them for reuse or retirement as needed.

---

The `metal-operator` is a core component of the IronCore project, designed to simplify and automate bare metal management across various hardware environments using BMC and Redfish API integrations. Expect continuous updates to expand capabilities and enhance usability.
