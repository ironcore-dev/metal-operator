# API Reference

## Packages
- [metal.ironcore.dev/v1alpha1](#metalironcoredevv1alpha1)


## metal.ironcore.dev/v1alpha1

Package v1alpha1 contains API Schema definitions for the metal.ironcore.dev API group

Package v1alpha1 contains API Schema definitions for the metal v1alpha1 API group

### Resource Types
- [BIOSSettings](#biossettings)
- [BIOSSettingsSet](#biossettingsset)
- [BIOSVersion](#biosversion)
- [BIOSVersionSet](#biosversionset)
- [BMC](#bmc)
- [BMCSecret](#bmcsecret)
- [BMCSettings](#bmcsettings)
- [BMCSettingsSet](#bmcsettingsset)
- [BMCUser](#bmcuser)
- [BMCVersion](#bmcversion)
- [BMCVersionSet](#bmcversionset)
- [Endpoint](#endpoint)
- [Server](#server)
- [ServerBootConfiguration](#serverbootconfiguration)
- [ServerClaim](#serverclaim)
- [ServerMaintenance](#servermaintenance)



#### BIOSSettings



BIOSSettings is the Schema for the biossettings API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BIOSSettings` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BIOSSettingsSpec](#biossettingsspec)_ |  |  |  |
| `status` _[BIOSSettingsStatus](#biossettingsstatus)_ |  |  |  |


#### BIOSSettingsFlowState

_Underlying type:_ _string_





_Appears in:_
- [BIOSSettingsFlowStatus](#biossettingsflowstatus)

| Field | Description |
| --- | --- |
| `Pending` | BIOSSettingsFlowStatePending specifies that the BIOS settings update for the current priority is pending.<br /> |
| `InProgress` | BIOSSettingsFlowStateInProgress specifies that the BIOS settings update for the current priority is in progress.<br /> |
| `Applied` | BIOSSettingsFlowStateApplied specifies that the BIOS settings for the current priority have been applied.<br /> |
| `Failed` | BIOSSettingsFlowStateFailed specifies that the BIOS settings update has failed.<br /> |


#### BIOSSettingsFlowStatus







_Appears in:_
- [BIOSSettingsStatus](#biossettingsstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `flowState` _[BIOSSettingsFlowState](#biossettingsflowstate)_ | State represents the current state of the BIOS settings update for the current priority. |  |  |
| `name` _string_ | Name identifies the current priority settings from the spec. |  |  |
| `priority` _integer_ | Priority identifies the settings priority from the spec. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#condition-v1-meta) array_ | Conditions represents the latest available observations of the BIOSSettings's current Flowstate. |  |  |
| `lastAppliedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | LastAppliedTime represents the timestamp when the last setting was successfully applied. |  |  |


#### BIOSSettingsSet



BIOSSettingsSet is the Schema for the biossettingssets API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BIOSSettingsSet` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BIOSSettingsSetSpec](#biossettingssetspec)_ |  |  |  |
| `status` _[BIOSSettingsSetStatus](#biossettingssetstatus)_ |  |  |  |


#### BIOSSettingsSetSpec



BIOSSettingsSetSpec defines the desired state of BIOSSettingsSet.



_Appears in:_
- [BIOSSettingsSet](#biossettingsset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `biosSettingsTemplate` _[BIOSSettingsTemplate](#biossettingstemplate)_ | BIOSSettingsTemplate defines the template for the BIOSSettings resource to be applied to the servers. |  |  |
| `serverSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | ServerSelector specifies a label selector to identify the servers that are to be selected. |  |  |


#### BIOSSettingsSetStatus



BIOSSettingsSetStatus defines the observed state of BIOSSettingsSet.



_Appears in:_
- [BIOSSettingsSet](#biossettingsset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `fullyLabeledServers` _integer_ | FullyLabeledServers is the number of servers in the set. |  |  |
| `availableBIOSSettings` _integer_ | AvailableBIOSSettings is the number of BIOSSettings currently created by the set. |  |  |
| `pendingBIOSSettings` _integer_ | PendingBIOSSettings is the total number of pending BIOSSettings in the set. |  |  |
| `inProgressBIOSSettings` _integer_ | InProgressBIOSSettings is the total number of BIOSSettings in the set that are currently in progress. |  |  |
| `completedBIOSSettings` _integer_ | CompletedBIOSSettings is the total number of completed BIOSSettings in the set. |  |  |
| `failedBIOSSettings` _integer_ | FailedBIOSSettings is the total number of failed BIOSSettings in the set. |  |  |


#### BIOSSettingsSpec



BIOSSettingsSpec defines the desired state of BIOSSettings.



_Appears in:_
- [BIOSSettings](#biossettings)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version specifies the software version (e.g. BIOS, BMC) these settings apply to. |  |  |
| `settingsFlow` _[SettingsFlowItem](#settingsflowitem) array_ | SettingsFlow contains the BIOS settings sequence to apply in the given order. |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be enforced on the server. |  |  |
| `serverMaintenanceRef` _[ObjectReference](#objectreference)_ | ServerMaintenanceRef is a reference to a ServerMaintenance object that BIOSSettings has requested for the referred server. |  |  |
| `serverRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | ServerRef is a reference to a specific server to apply the BIOS settings on. |  |  |


#### BIOSSettingsState

_Underlying type:_ _string_

BIOSSettingsState specifies the current state of the BIOS Settings update.



_Appears in:_
- [BIOSSettingsStatus](#biossettingsstatus)

| Field | Description |
| --- | --- |
| `Pending` | BIOSSettingsStatePending specifies that the BIOS settings update is waiting.<br /> |
| `InProgress` | BIOSSettingsStateInProgress specifies that the BIOS settings update is in progress.<br /> |
| `Applied` | BIOSSettingsStateApplied specifies that the BIOS settings have been applied.<br /> |
| `Failed` | BIOSSettingsStateFailed specifies that the BIOS settings update has failed.<br /> |


#### BIOSSettingsStatus



BIOSSettingsStatus defines the observed state of BIOSSettings.



_Appears in:_
- [BIOSSettings](#biossettings)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `state` _[BIOSSettingsState](#biossettingsstate)_ | State represents the current state of the BIOS settings update. |  |  |
| `flowState` _[BIOSSettingsFlowStatus](#biossettingsflowstatus) array_ | FlowState is a list of individual BIOSSettings operation flows. |  |  |
| `lastAppliedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | LastAppliedTime represents the timestamp when the last setting was successfully applied. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#condition-v1-meta) array_ | Conditions represents the latest available observations of the BIOSSettings's current state. |  |  |


#### BIOSSettingsTemplate







_Appears in:_
- [BIOSSettingsSetSpec](#biossettingssetspec)
- [BIOSSettingsSpec](#biossettingsspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version specifies the software version (e.g. BIOS, BMC) these settings apply to. |  |  |
| `settingsFlow` _[SettingsFlowItem](#settingsflowitem) array_ | SettingsFlow contains the BIOS settings sequence to apply in the given order. |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be enforced on the server. |  |  |


#### BIOSVersion



BIOSVersion is the Schema for the biosversions API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BIOSVersion` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BIOSVersionSpec](#biosversionspec)_ |  |  |  |
| `status` _[BIOSVersionStatus](#biosversionstatus)_ |  |  |  |


#### BIOSVersionSet



BIOSVersionSet is the Schema for the biosversionsets API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BIOSVersionSet` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BIOSVersionSetSpec](#biosversionsetspec)_ |  |  |  |
| `status` _[BIOSVersionSetStatus](#biosversionsetstatus)_ |  |  |  |


#### BIOSVersionSetSpec



BIOSVersionSetSpec defines the desired state of BIOSVersionSet.



_Appears in:_
- [BIOSVersionSet](#biosversionset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serverSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | ServerSelector specifies a label selector to identify the servers that are to be selected. |  |  |
| `biosVersionTemplate` _[BIOSVersionTemplate](#biosversiontemplate)_ | BIOSVersionTemplate defines the template for the BIOSVersion resource to be applied to the servers. |  |  |


#### BIOSVersionSetStatus



BIOSVersionSetStatus defines the observed state of BIOSVersionSet.



_Appears in:_
- [BIOSVersionSet](#biosversionset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `fullyLabeledServers` _integer_ | FullyLabeledServers is the number of servers in the set. |  |  |
| `availableBIOSVersion` _integer_ | AvailableBIOSVersion is the number of BIOSVersion created by the set. |  |  |
| `pendingBIOSVersion` _integer_ | PendingBIOSVersion is the total number of pending BIOSVersion in the set. |  |  |
| `inProgressBIOSVersion` _integer_ | InProgressBIOSVersion is the total number of BIOSVersion resources in the set that are currently in progress. |  |  |
| `completedBIOSVersion` _integer_ | CompletedBIOSVersion is the total number of completed BIOSVersion in the set. |  |  |
| `failedBIOSVersion` _integer_ | FailedBIOSVersion is the total number of failed BIOSVersion in the set. |  |  |


#### BIOSVersionSpec



BIOSVersionSpec defines the desired state of BIOSVersion.



_Appears in:_
- [BIOSVersion](#biosversion)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version specifies the BIOS version to upgrade to. |  |  |
| `updatePolicy` _[UpdatePolicy](#updatepolicy)_ | UpdatePolicy indicates whether the server's upgrade service should bypass vendor update policies. |  |  |
| `image` _[ImageSpec](#imagespec)_ | Image specifies the image to use to upgrade to the given BIOS version. |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be enforced on the server. |  |  |
| `serverMaintenanceRef` _[ObjectReference](#objectreference)_ | ServerMaintenanceRef is a reference to a ServerMaintenance object that the controller has requested for the referred server. |  |  |
| `serverRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | ServerRef is a reference to a specific server to apply the BIOS upgrade on. |  |  |


#### BIOSVersionState

_Underlying type:_ _string_





_Appears in:_
- [BIOSVersionStatus](#biosversionstatus)

| Field | Description |
| --- | --- |
| `Pending` | BIOSVersionStatePending specifies that the BIOS upgrade is waiting.<br /> |
| `InProgress` | BIOSVersionStateInProgress specifies that upgrading BIOS is in progress.<br /> |
| `Completed` | BIOSVersionStateCompleted specifies that the BIOS upgrade has been completed.<br /> |
| `Failed` | BIOSVersionStateFailed specifies that the BIOS upgrade has failed.<br /> |


#### BIOSVersionStatus



BIOSVersionStatus defines the observed state of BIOSVersion.



_Appears in:_
- [BIOSVersion](#biosversion)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `state` _[BIOSVersionState](#biosversionstate)_ | State represents the current state of the BIOS upgrade task. |  |  |
| `upgradeTask` _[Task](#task)_ | UpgradeTask contains the state of the Upgrade Task created by the BMC |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#condition-v1-meta) array_ | Conditions represents the latest available observations of the BIOS version upgrade state. |  |  |


#### BIOSVersionTemplate







_Appears in:_
- [BIOSVersionSetSpec](#biosversionsetspec)
- [BIOSVersionSpec](#biosversionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version specifies the BIOS version to upgrade to. |  |  |
| `updatePolicy` _[UpdatePolicy](#updatepolicy)_ | UpdatePolicy indicates whether the server's upgrade service should bypass vendor update policies. |  |  |
| `image` _[ImageSpec](#imagespec)_ | Image specifies the image to use to upgrade to the given BIOS version. |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be enforced on the server. |  |  |


#### BMC



BMC is the Schema for the bmcs API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BMC` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BMCSpec](#bmcspec)_ |  |  |  |
| `status` _[BMCStatus](#bmcstatus)_ |  |  |  |


#### BMCAccess



BMCAccess defines the access details for the BMC.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `protocol` _[Protocol](#protocol)_ | Protocol specifies the protocol to be used for communicating with the BMC. |  |  |
| `address` _string_ | Address is the address of the BMC. |  |  |
| `bmcSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | BMCSecretRef is a reference to the BMCSecret object that contains the credentials<br />required to access the BMC. |  |  |


#### BMCPowerState

_Underlying type:_ _string_

BMCPowerState defines the possible power states for a BMC.



_Appears in:_
- [BMCStatus](#bmcstatus)

| Field | Description |
| --- | --- |
| `On` | OnPowerState the system is powered on.<br /> |
| `Off` | OffPowerState the system is powered off, although some components may<br />continue to have AUX power such as management controller.<br /> |
| `Paused` | PausedPowerState the system is paused.<br /> |
| `PoweringOn` | PoweringOnPowerState A temporary state between Off and On. This<br />temporary state can be very short.<br /> |
| `PoweringOff` | PoweringOffPowerState A temporary state between On and Off. The power<br />off action can take time while the OS is in the shutdown process.<br /> |
| `Unknown` | UnknownPowerState indicates that power state is unknown for this BMC.<br /> |


#### BMCSecret



BMCSecret is the Schema for the bmcsecrets API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BMCSecret` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `immutable` _boolean_ | Immutable, if set to true, ensures that data stored in the Secret cannot<br />be updated (only object metadata can be modified).<br />If not set to true, the field can be modified at any time.<br />Defaulted to nil. |  |  |
| `data` _object (keys:string, values:integer array)_ | Data contains the secret data. Each key must consist of alphanumeric<br />characters, '-', '_' or '.'. The serialized form of the secret data is a<br />base64 encoded string, representing the arbitrary (possibly non-string)<br />data value here. Described in https://tools.ietf.org/html/rfc4648#section-4 |  |  |
| `stringData` _object (keys:string, values:string)_ | stringData allows specifying non-binary secret data in string form.<br />It is provided as a write-only input field for convenience.<br />All keys and values are merged into the data field on write, overwriting any existing values.<br />The stringData field is never output when reading from the API. |  |  |
| `type` _[SecretType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#secrettype-v1-core)_ | Used to facilitate programmatic handling of secret data.<br />More info: https://kubernetes.io/docs/concepts/configuration/secret/#secret-types |  |  |


#### BMCSettings



BMCSettings is the Schema for the BMCSettings API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BMCSettings` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BMCSettingsSpec](#bmcsettingsspec)_ |  |  |  |
| `status` _[BMCSettingsStatus](#bmcsettingsstatus)_ |  |  |  |


#### BMCSettingsSet



BMCSettingsSet is the Schema for the bmcsettingssets API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BMCSettingsSet` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BMCSettingsSetSpec](#bmcsettingssetspec)_ |  |  |  |
| `status` _[BMCSettingsSetStatus](#bmcsettingssetstatus)_ |  |  |  |


#### BMCSettingsSetSpec



BMCSettingsSetSpec defines the desired state of BMCSettingsSet.



_Appears in:_
- [BMCSettingsSet](#bmcsettingsset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bmcSettingsTemplate` _[BMCSettingsTemplate](#bmcsettingstemplate)_ | BMCSettingsTemplate defines the template for the BMCSettings resource to be applied to the BMCs. |  |  |
| `bmcSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | BMCSelector specifies a label selector to identify the BMCs to be selected. |  |  |


#### BMCSettingsSetStatus



BMCSettingsSetStatus defines the observed state of BMCSettingsSet.



_Appears in:_
- [BMCSettingsSet](#bmcsettingsset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `fullyLabeledBMCs` _integer_ | FullyLabeledBMCs is the number of BMCs in the set. |  |  |
| `availableBMCSettings` _integer_ | AvailableBMCSettings is the number of BMCSettings currently created by the set. |  |  |
| `pendingBMCSettings` _integer_ | PendingBMCSettings is the total number of pending BMCSettings in the set. |  |  |
| `inProgressBMCSettings` _integer_ | InProgressBMCSettings is the total number of BMCSettings in the set that are currently in progress. |  |  |
| `completedBMCSettings` _integer_ | CompletedBMCSettings is the total number of completed BMCSettings in the set. |  |  |
| `failedBMCSettings` _integer_ | FailedBMCSettings is the total number of failed BMCSettings in the set. |  |  |


#### BMCSettingsSpec



BMCSettingsSpec defines the desired state of BMCSettings.



_Appears in:_
- [BMCSettings](#bmcsettings)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version specifies the BMC firmware version for which the settings should be applied. |  |  |
| `settings` _object (keys:string, values:string)_ | SettingsMap contains BMC settings as a map. |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be applied on the server. |  |  |
| `serverMaintenanceRefs` _[ServerMaintenanceRefItem](#servermaintenancerefitem) array_ | ServerMaintenanceRefs are references to ServerMaintenance objects which are created by the controller for each<br />server that needs to be updated with the BMC settings. |  |  |
| `BMCRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | BMCRef is a reference to a specific BMC to apply settings to. |  |  |


#### BMCSettingsState

_Underlying type:_ _string_

BMCSettingsState specifies the current state of the server maintenance.



_Appears in:_
- [BMCSettingsStatus](#bmcsettingsstatus)

| Field | Description |
| --- | --- |
| `Pending` | BMCSettingsStatePending specifies that the BMC settings update is waiting.<br /> |
| `InProgress` | BMCSettingsStateInProgress specifies that the BMC settings changes are in progress.<br /> |
| `Applied` | BMCSettingsStateApplied specifies that the BMC settings have been applied.<br /> |
| `Failed` | BMCSettingsStateFailed specifies that the BMC settings update has failed.<br /> |


#### BMCSettingsStatus



BMCSettingsStatus defines the observed state of BMCSettings.



_Appears in:_
- [BMCSettings](#bmcsettings)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `state` _[BMCSettingsState](#bmcsettingsstate)_ | State represents the current state of the BMC configuration task. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#condition-v1-meta) array_ | Conditions represents the latest available observations of the BMC Settings Resource state. |  |  |


#### BMCSettingsTemplate







_Appears in:_
- [BMCSettingsSetSpec](#bmcsettingssetspec)
- [BMCSettingsSpec](#bmcsettingsspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version specifies the BMC firmware version for which the settings should be applied. |  |  |
| `settings` _object (keys:string, values:string)_ | SettingsMap contains BMC settings as a map. |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be applied on the server. |  |  |


#### BMCSpec



BMCSpec defines the desired state of BMC



_Appears in:_
- [BMC](#bmc)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bmcUUID` _string_ | BMCUUID is the unique identifier for the BMC as defined in Redfish API. |  | Optional: \{\} <br /> |
| `endpointRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | EndpointRef is a reference to the Endpoint object that contains the network access information for the BMC. |  | Optional: \{\} <br /> |
| `access` _[InlineEndpoint](#inlineendpoint)_ | Endpoint specifies inline network access details for the BMC. |  | Optional: \{\} <br /> |
| `bmcSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | BMCSecretRef is a reference to the BMCSecret object that contains the credentials<br />required to access the BMC. |  |  |
| `protocol` _[Protocol](#protocol)_ | Protocol specifies the protocol to be used for communicating with the BMC. |  |  |
| `consoleProtocol` _[ConsoleProtocol](#consoleprotocol)_ | ConsoleProtocol specifies the protocol to be used for console access to the BMC. |  |  |
| `bmcSettingsRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | BMCSettingRef is a reference to a BMCSettings object that specifies<br />the BMC configuration for this BMC. |  |  |
| `hostname` _string_ | Hostname is the hostname of the BMC. |  |  |


#### BMCState

_Underlying type:_ _string_

BMCState defines the possible states of a BMC.



_Appears in:_
- [BMCStatus](#bmcstatus)

| Field | Description |
| --- | --- |
| `Enabled` | BMCStateEnabled indicates that the BMC is enabled and functioning correctly.<br /> |
| `Error` | BMCStateError indicates that there is an error with the BMC.<br /> |
| `Pending` | BMCStatePending indicates that there is an error connecting with the BMC.<br /> |


#### BMCStatus



BMCStatus defines the observed state of BMC.



_Appears in:_
- [BMC](#bmc)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `macAddress` _string_ | MACAddress is the MAC address of the BMC. |  | Pattern: `^([0-9A-Fa-f]\{2\}[:-])\{5\}([0-9A-Fa-f]\{2\})$` <br /> |
| `ip` _[IP](#ip)_ | IP is the IP address of the BMC. |  | Format: ip <br />Schemaless: \{\} <br />Type: string <br /> |
| `manufacturer` _string_ | Manufacturer is the name of the BMC manufacturer. |  |  |
| `model` _string_ | Model is the model number or name of the BMC. |  |  |
| `sku` _string_ | SKU is the stock keeping unit identifier for the BMC. |  |  |
| `serialNumber` _string_ | SerialNumber is the serial number of the BMC. |  |  |
| `firmwareVersion` _string_ | FirmwareVersion is the version of the firmware currently running on the BMC. |  |  |
| `state` _[BMCState](#bmcstate)_ | State represents the current state of the BMC.<br />kubebuilder:validation:Enum=Enabled;Error;Pending | Pending |  |
| `powerState` _[BMCPowerState](#bmcpowerstate)_ | PowerState represents the current power state of the BMC. |  |  |
| `lastResetTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | LastResetTime is the timestamp of the last reset operation performed on the BMC. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#condition-v1-meta) array_ | Conditions represents the latest available observations of the BMC's current state. |  |  |


#### BMCUser



BMCUser is the Schema for the bmcusers API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BMCUser` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BMCUserSpec](#bmcuserspec)_ |  |  |  |
| `status` _[BMCUserStatus](#bmcuserstatus)_ |  |  |  |


#### BMCUserSpec



BMCUserSpec defines the desired state of BMCUser.



_Appears in:_
- [BMCUser](#bmcuser)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `userName` _string_ | UserName is the username of the BMC user. |  |  |
| `roleID` _string_ | RoleID is the ID of the role to assign to the user. |  |  |
| `description` _string_ | Description is a description for the BMC user. |  |  |
| `rotationPeriod` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | RotationPeriod defines how often the password should be rotated.<br />If not set, the password will not be rotated. |  |  |
| `bmcSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | BMCSecretRef references the BMCSecret containing the credentials for this user.<br />If not set, the operator will generate a secure password based on BMC manufacturer requirements. |  |  |
| `bmcRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | BMCRef references the BMC this user should be created on. |  |  |


#### BMCUserStatus



BMCUserStatus defines the observed state of BMCUser.



_Appears in:_
- [BMCUser](#bmcuser)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `effectiveBMCSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | EffectiveBMCSecretRef references the BMCSecret currently used for this user.<br />This may differ from Spec.BMCSecretRef if the operator generated a password. |  |  |
| `lastRotation` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | LastRotation is the timestamp of the last password rotation. |  |  |
| `passwordExpiration` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | PasswordExpiration is the timestamp when the password will expire. |  |  |
| `id` _string_ | ID is the identifier of the user in the BMC system. |  |  |


#### BMCVersion



BMCVersion is the Schema for the bmcversions API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BMCVersion` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BMCVersionSpec](#bmcversionspec)_ |  |  |  |
| `status` _[BMCVersionStatus](#bmcversionstatus)_ |  |  |  |


#### BMCVersionSet



BMCVersionSet is the Schema for the bmcversionsets API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BMCVersionSet` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BMCVersionSetSpec](#bmcversionsetspec)_ |  |  |  |
| `status` _[BMCVersionSetStatus](#bmcversionsetstatus)_ |  |  |  |


#### BMCVersionSetSpec



BMCVersionSetSpec defines the desired state of BMCVersionSet.



_Appears in:_
- [BMCVersionSet](#bmcversionset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bmcSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | BMCSelector specifies a label selector to identify the BMCs to be selected. |  |  |
| `bmcVersionTemplate` _[BMCVersionTemplate](#bmcversiontemplate)_ | BMCVersionTemplate defines the template for the BMCVersion resource to be applied to the BMCs. |  |  |


#### BMCVersionSetStatus



BMCVersionSetStatus defines the observed state of BMCVersionSet.



_Appears in:_
- [BMCVersionSet](#bmcversionset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `fullyLabeledBMCs` _integer_ | FullyLabeledBMCs is the number of BMCs in the set. |  |  |
| `availableBMCVersion` _integer_ | AvailableBMCVersion is the number of BMCVersion resources currently created by the set. |  |  |
| `pendingBMCVersion` _integer_ | PendingBMCVersion is the total number of pending BMCVersion resources in the set. |  |  |
| `inProgressBMCVersion` _integer_ | InProgressBMCVersion is the total number of BMCVersion resources in the set that are currently in progress. |  |  |
| `completedBMCVersion` _integer_ | CompletedBMCVersion is the total number of completed BMCVersion resources in the set. |  |  |
| `failedBMCVersion` _integer_ | FailedBMCVersion is the total number of failed BMCVersion resources in the set. |  |  |


#### BMCVersionSpec



BMCVersionSpec defines the desired state of BMCVersion.



_Appears in:_
- [BMCVersion](#bmcversion)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version specifies the BMC version to upgrade to. |  |  |
| `updatePolicy` _[UpdatePolicy](#updatepolicy)_ | UpdatePolicy indicates whether the server's upgrade service should bypass vendor update policies. |  |  |
| `image` _[ImageSpec](#imagespec)_ | Image specifies the image to use to upgrade to the given BMC version. |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be enforced on the server managed by referred BMC. |  |  |
| `serverMaintenanceRefs` _[ObjectReference](#objectreference) array_ | ServerMaintenanceRefs are references to ServerMaintenance objects that the controller has requested for the related servers. |  |  |
| `bmcRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | BMCRef is a reference to a specific BMC to apply BMC upgrade on. |  |  |


#### BMCVersionState

_Underlying type:_ _string_





_Appears in:_
- [BMCVersionStatus](#bmcversionstatus)

| Field | Description |
| --- | --- |
| `Pending` | BMCVersionStatePending specifies that the BMC upgrade is waiting.<br /> |
| `InProgress` | BMCVersionStateInProgress specifies that upgrading BMC is in progress.<br /> |
| `Completed` | BMCVersionStateCompleted specifies that the BMC upgrade maintenance has been completed.<br /> |
| `Failed` | BMCVersionStateFailed specifies that the BMC upgrade maintenance has failed.<br /> |


#### BMCVersionStatus



BMCVersionStatus defines the observed state of BMCVersion.



_Appears in:_
- [BMCVersion](#bmcversion)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `state` _[BMCVersionState](#bmcversionstate)_ | State represents the current state of the BMC configuration task. |  |  |
| `upgradeTask` _[Task](#task)_ | UpgradeTask contains the state of the upgrade task created by the BMC. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#condition-v1-meta) array_ | Conditions represents the latest available observations of the BMC version upgrade state. |  |  |


#### BMCVersionTemplate







_Appears in:_
- [BMCVersionSetSpec](#bmcversionsetspec)
- [BMCVersionSpec](#bmcversionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version specifies the BMC version to upgrade to. |  |  |
| `updatePolicy` _[UpdatePolicy](#updatepolicy)_ | UpdatePolicy indicates whether the server's upgrade service should bypass vendor update policies. |  |  |
| `image` _[ImageSpec](#imagespec)_ | Image specifies the image to use to upgrade to the given BMC version. |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be enforced on the server managed by referred BMC. |  |  |


#### BootOrder



BootOrder represents the boot order of the server.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the boot device. |  |  |
| `priority` _integer_ | Priority is the priority of the boot device. |  |  |
| `device` _string_ | Device is the device to boot from. |  |  |


#### ConsoleProtocol



ConsoleProtocol defines the protocol and port used for console access to the BMC.



_Appears in:_
- [BMCSpec](#bmcspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _[ConsoleProtocolName](#consoleprotocolname)_ | Name specifies the name of the console protocol. |  | Enum: [IPMI SSH SSHLenovo] <br /> |
| `port` _integer_ | Port specifies the port number used for console access. |  |  |


#### ConsoleProtocolName

_Underlying type:_ _string_

ConsoleProtocolName defines the possible names for console protocols.



_Appears in:_
- [ConsoleProtocol](#consoleprotocol)

| Field | Description |
| --- | --- |
| `IPMI` | ConsoleProtocolNameIPMI represents the IPMI console protocol.<br /> |
| `SSH` | ConsoleProtocolNameSSH represents the SSH console protocol.<br /> |
| `SSHLenovo` | ConsoleProtocolNameSSHLenovo represents the SSH console protocol specific to Lenovo hardware.<br /> |


#### Endpoint



Endpoint is the Schema for the endpoints API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `Endpoint` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[EndpointSpec](#endpointspec)_ |  |  |  |
| `status` _[EndpointStatus](#endpointstatus)_ |  |  |  |


#### EndpointSpec



EndpointSpec defines the desired state of Endpoint



_Appears in:_
- [Endpoint](#endpoint)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `macAddress` _string_ | MACAddress is the MAC address of the endpoint. |  |  |
| `ip` _[IP](#ip)_ | IP is the IP address of the endpoint. |  | Format: ip <br />Schemaless: \{\} <br />Type: string <br /> |


#### EndpointStatus



EndpointStatus defines the observed state of Endpoint



_Appears in:_
- [Endpoint](#endpoint)



#### IP



IP is an IP address.

_Validation:_
- Format: ip
- Type: string

_Appears in:_
- [BMCStatus](#bmcstatus)
- [EndpointSpec](#endpointspec)
- [InlineEndpoint](#inlineendpoint)
- [NetworkInterface](#networkinterface)





#### ImageSpec







_Appears in:_
- [BIOSVersionSpec](#biosversionspec)
- [BIOSVersionTemplate](#biosversiontemplate)
- [BMCVersionSpec](#bmcversionspec)
- [BMCVersionTemplate](#bmcversiontemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secretRef` _[SecretReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#secretreference-v1-core)_ | SecretRef is a reference to the Secret containing the credentials to access the image URI. |  |  |
| `transferProtocol` _string_ | TransferProtocol is the network protocol used to retrieve the image URI. |  |  |
| `URI` _string_ | URI is the URI of the software image to install. |  |  |


#### IndicatorLED

_Underlying type:_ _string_

IndicatorLED represents LED indicator states



_Appears in:_
- [ServerSpec](#serverspec)
- [ServerStatus](#serverstatus)

| Field | Description |
| --- | --- |
| `Unknown` | UnknownIndicatorLED indicates the state of the Indicator LED cannot be<br />determined.<br /> |
| `Lit` | LitIndicatorLED indicates the Indicator LED is lit.<br /> |
| `Blinking` | BlinkingIndicatorLED indicates the Indicator LED is blinking.<br /> |
| `Off` | OffIndicatorLED indicates the Indicator LED is off.<br /> |


#### InlineEndpoint



InlineEndpoint defines inline network access configuration for the BMC.



_Appears in:_
- [BMCSpec](#bmcspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `macAddress` _string_ | MACAddress is the MAC address of the endpoint. |  |  |
| `ip` _[IP](#ip)_ | IP is the IP address of the BMC. |  | Format: ip <br />Schemaless: \{\} <br />Type: string <br /> |


#### LLDPNeighbor



LLDPNeighbor defines the details of an LLDP neighbor.



_Appears in:_
- [NetworkInterface](#networkinterface)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `macAddress` _string_ | MACAddress is the MAC address of the LLDP neighbor. |  |  |
| `portID` _string_ | PortID is the port identifier of the LLDP neighbor. |  |  |
| `portDescription` _string_ | PortDescription is the port description of the LLDP neighbor. |  |  |
| `systemName` _string_ | SystemName is the system name of the LLDP neighbor. |  |  |
| `systemDescription` _string_ | SystemDescription is the system description of the LLDP neighbor. |  |  |


#### NetworkInterface



NetworkInterface defines the details of a network interface.



_Appears in:_
- [ServerStatus](#serverstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the network interface. |  |  |
| `ip` _[IP](#ip)_ | IP is the IP address assigned to the network interface.<br />Deprecated: Use IPs instead. Kept for backward compatibility, always nil. |  | Format: ip <br />Schemaless: \{\} <br />Type: string <br /> |
| `ips` _[IP](#ip) array_ | IPs is a list of IP addresses (both IPv4 and IPv6) assigned to the network interface. |  | Format: ip <br />Type: string <br /> |
| `macAddress` _string_ | MACAddress is the MAC address of the network interface. |  |  |
| `carrierStatus` _string_ | CarrierStatus is the operational carrier status of the network interface. |  |  |
| `neighbors` _[LLDPNeighbor](#lldpneighbor) array_ | Neighbors contains the LLDP neighbors discovered on this interface. |  |  |


#### ObjectReference



ObjectReference is the namespaced name reference to an object.



_Appears in:_
- [BIOSSettingsSpec](#biossettingsspec)
- [BIOSVersionSpec](#biosversionspec)
- [BMCVersionSpec](#bmcversionspec)
- [ServerMaintenanceRefItem](#servermaintenancerefitem)
- [ServerSpec](#serverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | APIVersion is the API version of the referenced object. |  |  |
| `kind` _string_ | Kind is the kind of the referenced object. |  |  |
| `namespace` _string_ | Namespace is the namespace of the referenced object. |  |  |
| `name` _string_ | Name is the name of the referenced object. |  |  |
| `uid` _[UID](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#uid-types-pkg)_ | UID is the uid of the referenced object. |  |  |


#### Phase

_Underlying type:_ _string_

Phase defines the possible phases of a ServerClaim.



_Appears in:_
- [ServerClaimStatus](#serverclaimstatus)

| Field | Description |
| --- | --- |
| `Bound` | PhaseBound indicates that the server claim is bound to a server.<br /> |
| `Unbound` | PhaseUnbound indicates that the server claim is not bound to any server.<br /> |


#### Power

_Underlying type:_ _string_

Power defines the possible power states for a device.



_Appears in:_
- [ServerClaimSpec](#serverclaimspec)
- [ServerMaintenanceSpec](#servermaintenancespec)
- [ServerSpec](#serverspec)

| Field | Description |
| --- | --- |
| `On` | PowerOn indicates that the device is powered on.<br /> |
| `Off` | PowerOff indicates that the device is powered off.<br /> |


#### Processor



Processor defines the details of a Processor.



_Appears in:_
- [ServerStatus](#serverstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is the name of the Processor. |  |  |
| `type` _string_ | Type is the type of the Processor. |  |  |
| `architecture` _string_ | Architecture is the architecture of the Processor. |  |  |
| `instructionSet` _string_ | InstructionSet is the instruction set of the Processor. |  |  |
| `manufacturer` _string_ | Manufacturer is the manufacturer of the Processor. |  |  |
| `model` _string_ | Model is the model of the Processor. |  |  |
| `maxSpeedMHz` _integer_ | MaxSpeedMHz is the maximum speed of the Processor in MHz. |  |  |
| `totalCores` _integer_ | TotalCores is the total number of cores in the Processor. |  |  |
| `totalThreads` _integer_ | TotalThreads is the total number of threads in the Processor. |  |  |


#### Protocol



Protocol defines the protocol and port used for communicating with the BMC.



_Appears in:_
- [BMCAccess](#bmcaccess)
- [BMCSpec](#bmcspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _[ProtocolName](#protocolname)_ | Name specifies the name of the protocol. |  |  |
| `port` _integer_ | Port specifies the port number used for communication. |  |  |
| `scheme` _[ProtocolScheme](#protocolscheme)_ | Scheme specifies the scheme used for communication. |  |  |


#### ProtocolName

_Underlying type:_ _string_

ProtocolName defines the possible names for protocols used for communicating with the BMC.



_Appears in:_
- [Protocol](#protocol)

| Field | Description |
| --- | --- |
| `Redfish` | ProtocolNameRedfish represents the Redfish protocol.<br /> |
| `IPMI` | ProtocolNameIPMI represents the IPMI protocol.<br /> |
| `SSH` | ProtocolNameSSH represents the SSH protocol.<br /> |


#### ProtocolScheme

_Underlying type:_ _string_

ProtocolScheme is a string that contains the protocol scheme



_Appears in:_
- [Protocol](#protocol)

| Field | Description |
| --- | --- |
| `http` | HTTPProtocolScheme is the http protocol scheme<br /> |
| `https` | HTTPSProtocolScheme is the https protocol scheme<br /> |


#### Server



Server is the Schema for the servers API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `Server` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ServerSpec](#serverspec)_ |  |  |  |
| `status` _[ServerStatus](#serverstatus)_ |  |  |  |


#### ServerBootConfiguration



ServerBootConfiguration is the Schema for the serverbootconfigurations API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `ServerBootConfiguration` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ServerBootConfigurationSpec](#serverbootconfigurationspec)_ |  |  |  |
| `status` _[ServerBootConfigurationStatus](#serverbootconfigurationstatus)_ |  |  |  |


#### ServerBootConfigurationSpec



ServerBootConfigurationSpec defines the desired state of ServerBootConfiguration.



_Appears in:_
- [ServerBootConfiguration](#serverbootconfiguration)
- [ServerBootConfigurationTemplate](#serverbootconfigurationtemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serverRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | ServerRef is a reference to the server for which this boot configuration is intended. |  |  |
| `image` _string_ | Image specifies the boot image to be used for the server. |  |  |
| `ignitionSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | IgnitionSecretRef is a reference to the Secret object that contains<br />the ignition configuration for the server. |  |  |


#### ServerBootConfigurationState

_Underlying type:_ _string_

ServerBootConfigurationState defines the possible states of a ServerBootConfiguration.



_Appears in:_
- [ServerBootConfigurationStatus](#serverbootconfigurationstatus)

| Field | Description |
| --- | --- |
| `Pending` | ServerBootConfigurationStatePending indicates that the boot configuration is pending and not yet ready.<br /> |
| `Ready` | ServerBootConfigurationStateReady indicates that the boot configuration is ready for use.<br /> |
| `Error` | ServerBootConfigurationStateError indicates that there is an error with the boot configuration.<br /> |


#### ServerBootConfigurationStatus



ServerBootConfigurationStatus defines the observed state of ServerBootConfiguration.



_Appears in:_
- [ServerBootConfiguration](#serverbootconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `state` _[ServerBootConfigurationState](#serverbootconfigurationstate)_ | State represents the current state of the boot configuration. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#condition-v1-meta) array_ | Conditions represents the latest available observations of the ServerBootConfig's current state. |  |  |


#### ServerBootConfigurationTemplate



ServerBootConfigurationTemplate defines the parameters to be used for rendering a boot configuration.



_Appears in:_
- [ServerMaintenanceSpec](#servermaintenancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name specifies the name of the boot configuration. |  |  |
| `spec` _[ServerBootConfigurationSpec](#serverbootconfigurationspec)_ | Spec specifies the boot configuration to be rendered. |  |  |


#### ServerClaim



ServerClaim is the Schema for the serverclaims API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `ServerClaim` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ServerClaimSpec](#serverclaimspec)_ |  |  |  |
| `status` _[ServerClaimStatus](#serverclaimstatus)_ |  |  |  |


#### ServerClaimSpec



ServerClaimSpec defines the desired state of ServerClaim.



_Appears in:_
- [ServerClaim](#serverclaim)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `power` _[Power](#power)_ | Power specifies the desired power state of the server. |  |  |
| `serverRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | ServerRef is a reference to a specific server to be claimed. |  | Optional: \{\} <br /> |
| `serverSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | ServerSelector specifies a label selector to identify the server to be claimed. |  | Optional: \{\} <br /> |
| `ignitionSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | IgnitionSecretRef is a reference to the Secret object that contains<br />the ignition configuration for the server. |  |  |
| `image` _string_ | Image specifies the boot image to be used for the server. |  |  |


#### ServerClaimStatus



ServerClaimStatus defines the observed state of ServerClaim.



_Appears in:_
- [ServerClaim](#serverclaim)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[Phase](#phase)_ | Phase represents the current phase of the server claim. |  |  |


#### ServerMaintenance



ServerMaintenance is the Schema for the ServerMaintenance API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `ServerMaintenance` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ServerMaintenanceSpec](#servermaintenancespec)_ |  |  |  |
| `status` _[ServerMaintenanceStatus](#servermaintenancestatus)_ |  |  |  |


#### ServerMaintenancePolicy

_Underlying type:_ _string_

ServerMaintenancePolicy specifies the maintenance policy to be enforced on the server.



_Appears in:_
- [BIOSSettingsSpec](#biossettingsspec)
- [BIOSSettingsTemplate](#biossettingstemplate)
- [BIOSVersionSpec](#biosversionspec)
- [BIOSVersionTemplate](#biosversiontemplate)
- [BMCSettingsSpec](#bmcsettingsspec)
- [BMCSettingsTemplate](#bmcsettingstemplate)
- [BMCVersionSpec](#bmcversionspec)
- [BMCVersionTemplate](#bmcversiontemplate)
- [ServerMaintenanceSpec](#servermaintenancespec)

| Field | Description |
| --- | --- |
| `OwnerApproval` | ServerMaintenancePolicyOwnerApproval specifies that the maintenance policy requires owner approval.<br /> |
| `Enforced` | ServerMaintenancePolicyEnforced specifies that the maintenance policy is enforced.<br /> |


#### ServerMaintenanceRefItem



ServerMaintenanceRefItem is a reference to a ServerMaintenance object.



_Appears in:_
- [BMCSettingsSpec](#bmcsettingsspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serverMaintenanceRef` _[ObjectReference](#objectreference)_ | ServerMaintenanceRef is a reference to a ServerMaintenance object that the BMCSettings has requested for the referred server. |  |  |


#### ServerMaintenanceSpec



ServerMaintenanceSpec defines the desired state of a ServerMaintenance



_Appears in:_
- [ServerMaintenance](#servermaintenance)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `policy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | Policy specifies the maintenance policy to be enforced on the server. |  |  |
| `serverRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | ServerRef is a reference to the server that is to be maintained. |  |  |
| `serverPower` _[Power](#power)_ | ServerPower specifies the power state of the server during maintenance. |  |  |
| `serverBootConfigurationTemplate` _[ServerBootConfigurationTemplate](#serverbootconfigurationtemplate)_ | ServerBootConfigurationTemplate specifies the boot configuration to be applied to the server during maintenance. |  |  |


#### ServerMaintenanceState

_Underlying type:_ _string_

ServerMaintenanceState specifies the current state of the server maintenance.



_Appears in:_
- [ServerMaintenanceStatus](#servermaintenancestatus)

| Field | Description |
| --- | --- |
| `Pending` | ServerMaintenanceStatePending specifies that the server maintenance is pending.<br /> |
| `InMaintenance` | ServerMaintenanceStateInMaintenance specifies that the server is in maintenance.<br /> |
| `Failed` | ServerMaintenanceStateFailed specifies that the server maintenance has failed.<br /> |


#### ServerMaintenanceStatus



ServerMaintenanceStatus defines the observed state of a ServerMaintenance



_Appears in:_
- [ServerMaintenance](#servermaintenance)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `state` _[ServerMaintenanceState](#servermaintenancestate)_ | State specifies the current state of the server maintenance. |  |  |


#### ServerPowerState

_Underlying type:_ _string_

ServerPowerState defines the possible power states for a server.



_Appears in:_
- [ServerStatus](#serverstatus)

| Field | Description |
| --- | --- |
| `On` | ServerOnPowerState indicates that the system is powered on.<br /> |
| `Off` | ServerOffPowerState indicates that the system is powered off, although some components may<br />continue to have auxiliary power such as the management controller.<br /> |
| `Paused` | ServerPausedPowerState indicates that the system is paused.<br /> |
| `PoweringOn` | ServerPoweringOnPowerState indicates a temporary state between Off and On.<br />This temporary state can be very short.<br /> |
| `PoweringOff` | ServerPoweringOffPowerState indicates a temporary state between On and Off.<br />The power off action can take time while the OS is in the shutdown process.<br /> |


#### ServerSpec



ServerSpec defines the desired state of a Server.



_Appears in:_
- [Server](#server)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `uuid` _string_ | UUID is the unique identifier for the server.<br />Deprecated in favor of systemUUID. |  |  |
| `systemUUID` _string_ | SystemUUID is the unique identifier for the server. |  |  |
| `systemURI` _string_ | SystemURI is the unique URI for the server resource in REDFISH API. |  |  |
| `power` _[Power](#power)_ | Power specifies the desired power state of the server. |  |  |
| `indicatorLED` _[IndicatorLED](#indicatorled)_ | IndicatorLED specifies the desired state of the server's indicator LED. |  |  |
| `serverClaimRef` _[ObjectReference](#objectreference)_ | ServerClaimRef is a reference to a ServerClaim object that claims this server. |  | Optional: \{\} <br /> |
| `serverMaintenanceRef` _[ObjectReference](#objectreference)_ | ServerMaintenanceRef is a reference to a ServerMaintenance object that maintains this server. |  |  |
| `bmcRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | BMCRef is a reference to the BMC object associated with this server. |  |  |
| `bmc` _[BMCAccess](#bmcaccess)_ | BMC contains the access details for the BMC. |  |  |
| `bootConfigurationRef` _[ObjectReference](#objectreference)_ | BootConfigurationRef is a reference to a BootConfiguration object that specifies<br />the boot configuration for this server. |  |  |
| `maintenanceBootConfigurationRef` _[ObjectReference](#objectreference)_ | MaintenanceBootConfigurationRef is a reference to a BootConfiguration object that specifies<br />the boot configuration for this server during maintenance. |  |  |
| `bootOrder` _[BootOrder](#bootorder) array_ | BootOrder specifies the boot order of the server. |  |  |
| `biosSettingsRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | BIOSSettingsRef is a reference to a biossettings object that specifies<br />the BIOS configuration for this server. |  |  |


#### ServerState

_Underlying type:_ _string_

ServerState defines the possible states of a server.



_Appears in:_
- [ServerStatus](#serverstatus)

| Field | Description |
| --- | --- |
| `Initial` | ServerStateInitial indicates that the server is in its initial state.<br /> |
| `Discovery` | ServerStateDiscovery indicates that the server is in its discovery state.<br /> |
| `Available` | ServerStateAvailable indicates that the server is available for use.<br /> |
| `Reserved` | ServerStateReserved indicates that the server is reserved for a specific use or user.<br /> |
| `Error` | ServerStateError indicates that there is an error with the server.<br /> |
| `Maintenance` | ServerStateMaintenance indicates that the server is in maintenance.<br /> |


#### ServerStatus



ServerStatus defines the observed state of Server.



_Appears in:_
- [Server](#server)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `manufacturer` _string_ | Manufacturer is the name of the server manufacturer. |  |  |
| `biosVersion` _string_ | BIOSVersion is the version of the server's BIOS. |  |  |
| `model` _string_ | Model is the model of the server. |  |  |
| `sku` _string_ | SKU is the stock keeping unit identifier for the server. |  |  |
| `serialNumber` _string_ | SerialNumber is the serial number of the server. |  |  |
| `powerState` _[ServerPowerState](#serverpowerstate)_ | PowerState represents the current power state of the server. |  |  |
| `indicatorLED` _[IndicatorLED](#indicatorled)_ | IndicatorLED specifies the current state of the server's indicator LED. |  |  |
| `state` _[ServerState](#serverstate)_ | State represents the current state of the server. |  |  |
| `networkInterfaces` _[NetworkInterface](#networkinterface) array_ | NetworkInterfaces is a list of network interfaces associated with the server. |  |  |
| `totalSystemMemory` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#quantity-resource-api)_ | TotalSystemMemory is the total amount of memory in bytes available on the server. |  |  |
| `processors` _[Processor](#processor) array_ | Processors is a list of Processors associated with the server. |  |  |
| `storages` _[Storage](#storage) array_ | Storages is a list of storages associated with the server. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#condition-v1-meta) array_ | Conditions represents the latest available observations of the server's current state. |  |  |


#### SettingsFlowItem







_Appears in:_
- [BIOSSettingsSpec](#biossettingsspec)
- [BIOSSettingsTemplate](#biossettingstemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the flow item. |  | MaxLength: 1000 <br />MinLength: 1 <br /> |
| `settings` _object (keys:string, values:string)_ | Settings contains software (e.g. BIOS, BMC) settings as a map. |  |  |
| `priority` _integer_ | Priority defines the order of applying the settings. Lower numbers have higher priority (i.e. lower numbers are applied first). |  | Maximum: 2.147483645e+09 <br />Minimum: 1 <br /> |


#### Storage



Storage defines the details of one storage device.



_Appears in:_
- [ServerStatus](#serverstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the storage device. |  |  |
| `state` _[StorageState](#storagestate)_ | State specifies the state of the storage device. |  |  |
| `volumes` _[StorageVolume](#storagevolume) array_ | Volumes is a collection of volumes associated with this storage. |  |  |
| `drives` _[StorageDrive](#storagedrive) array_ | Drives is a collection of drives associated with this storage. |  |  |


#### StorageDrive



StorageDrive defines the details of one storage drive.



_Appears in:_
- [Storage](#storage)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the storage drive. |  |  |
| `mediaType` _string_ | MediaType specifies the media type of the storage device. |  |  |
| `type` _string_ | Type specifies the type of the storage device. |  |  |
| `capacity` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#quantity-resource-api)_ | Capacity specifies the size of the storage device in bytes. |  |  |
| `vendor` _string_ | Vendor specifies the vendor of the storage device. |  |  |
| `model` _string_ | Model specifies the model of the storage device. |  |  |
| `state` _[StorageState](#storagestate)_ | State specifies the state of the storage device. |  |  |


#### StorageState

_Underlying type:_ _string_

StorageState represents Storage states



_Appears in:_
- [Storage](#storage)
- [StorageDrive](#storagedrive)
- [StorageVolume](#storagevolume)

| Field | Description |
| --- | --- |
| `Enabled` | StorageStateEnabled indicates that the storage device is enabled.<br /> |
| `Disabled` | StorageStateDisabled indicates that the storage device is disabled.<br /> |
| `Absent` | StorageStateAbsent indicates that the storage device is absent.<br /> |


#### StorageVolume



StorageVolume defines the details of one storage volume.



_Appears in:_
- [Storage](#storage)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the storage volume. |  |  |
| `capacity` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#quantity-resource-api)_ | Capacity specifies the size of the storage device in bytes. |  |  |
| `state` _[StorageState](#storagestate)_ | Status specifies the status of the volume. |  |  |
| `raidType` _string_ | RAIDType specifies the RAID type of the associated Volume. |  |  |
| `volumeUsage` _string_ | VolumeUsage specifies the volume usage type for the Volume. |  |  |


#### Task



Task contains the status of the task created by the BMC for the BIOS upgrade.



_Appears in:_
- [BIOSVersionStatus](#biosversionstatus)
- [BMCVersionStatus](#bmcversionstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `URI` _string_ | URI is the URI of the task created by the BMC for the BIOS upgrade. |  |  |
| `state` _[TaskState](#taskstate)_ | State is the current state of the task. |  |  |
| `status` _[Health](#health)_ | Status is the current status of the task. |  |  |
| `percentageComplete` _integer_ | PercentComplete is the percentage of completion of the task. |  |  |


#### UpdatePolicy

_Underlying type:_ _string_





_Appears in:_
- [BIOSVersionSpec](#biosversionspec)
- [BIOSVersionTemplate](#biosversiontemplate)
- [BMCVersionSpec](#bmcversionspec)
- [BMCVersionTemplate](#bmcversiontemplate)

| Field | Description |
| --- | --- |
| `Force` |  |


