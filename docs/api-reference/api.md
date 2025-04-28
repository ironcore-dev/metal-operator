<p>Packages:</p>
<ul>
<li>
<a href="#metal.ironcore.dev%2fv1alpha1">metal.ironcore.dev/v1alpha1</a>
</li>
</ul>
<h2 id="metal.ironcore.dev/v1alpha1">metal.ironcore.dev/v1alpha1</h2>
<div>
<p>Package v1alpha1 contains API Schema definitions for the settings.gardener.cloud API group</p>
</div>
Resource Types:
<ul></ul>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSSettingUpdateState">BIOSSettingUpdateState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsStatus">BIOSSettingsStatus</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;IssueSettingUpdate&#34;</p></td>
<td><p>BIOSSettingUpdateStateIssue specifies that the bios new setting was posted to server&rsquo;s RedFish API</p>
</td>
</tr><tr><td><p>&#34;VerifySettingUpdate&#34;</p></td>
<td><p>BIOSSettingUpdateStateVerification specifies that the bios setting is beening verified.</p>
</td>
</tr><tr><td><p>&#34;WaitOnServerRebootPowerOff&#34;</p></td>
<td><p>BIOSSettingUpdateWaitOnServerRebootPowerOff specifies that the bios setting state is waiting on server to turn off during Reboot.</p>
</td>
</tr><tr><td><p>&#34;WaitOnServerRebootPowerOn&#34;</p></td>
<td><p>BIOSSettingUpdateWaitOnServerRebootPowerOn specifies that the bios setting state is waiting on server to turn on during Reboot.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSSettings">BIOSSettings
</h3>
<div>
<p>BIOSSettings is the Schema for the biossettings API.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsSpec">
BIOSSettingsSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>version</code><br/>
<em>
string
</em>
</td>
<td>
<p>Version contains software (eg: BIOS, BMC) version this settings applies to</p>
</td>
</tr>
<tr>
<td>
<code>settings</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>SettingsMap contains software (eg: BIOS, BMC) settings as map</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to a specific server to apply bios setting on.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenancePolicy</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenancePolicy">
ServerMaintenancePolicy
</a>
</em>
</td>
<td>
<p>ServerMaintenancePolicy is maintenance policy to be enforced on the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>ServerMaintenanceRef is a reference to a ServerMaintenance object that BiosSetting has requested for the referred server.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsStatus">
BIOSSettingsStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSSettingsSpec">BIOSSettingsSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSSettings">BIOSSettings</a>)
</p>
<div>
<p>BIOSSettingsSpec defines the desired state of BIOSSettings.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>version</code><br/>
<em>
string
</em>
</td>
<td>
<p>Version contains software (eg: BIOS, BMC) version this settings applies to</p>
</td>
</tr>
<tr>
<td>
<code>settings</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>SettingsMap contains software (eg: BIOS, BMC) settings as map</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to a specific server to apply bios setting on.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenancePolicy</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenancePolicy">
ServerMaintenancePolicy
</a>
</em>
</td>
<td>
<p>ServerMaintenancePolicy is maintenance policy to be enforced on the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>ServerMaintenanceRef is a reference to a ServerMaintenance object that BiosSetting has requested for the referred server.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSSettingsState">BIOSSettingsState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsStatus">BIOSSettingsStatus</a>)
</p>
<div>
<p>BIOSSettingsState specifies the current state of the BIOS maintenance.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Applied&#34;</p></td>
<td><p>BIOSSettingsStateApplied specifies that the bios setting maintenance has been completed.</p>
</td>
</tr><tr><td><p>&#34;Failed&#34;</p></td>
<td><p>BIOSSettingsStateFailed specifies that the bios setting maintenance has failed.</p>
</td>
</tr><tr><td><p>&#34;InProgress&#34;</p></td>
<td><p>BIOSSettingsStateInProgress specifies that the BIOSSetting Controller is updating the settings</p>
</td>
</tr><tr><td><p>&#34;Pending&#34;</p></td>
<td><p>BIOSSettingsStatePending specifies that the bios setting maintenance is waiting</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSSettingsStatus">BIOSSettingsStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSSettings">BIOSSettings</a>)
</p>
<div>
<p>BIOSSettingsStatus defines the observed state of BIOSSettings.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsState">
BIOSSettingsState
</a>
</em>
</td>
<td>
<p>State represents the current state of the bios configuration task.</p>
</td>
</tr>
<tr>
<td>
<code>updateSettingState</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettingUpdateState">
BIOSSettingUpdateState
</a>
</em>
</td>
<td>
<p>UpdateSettingState represents the current state of the bios setting update task.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSVersion">BIOSVersion
</h3>
<div>
<p>BIOSVersion is the Schema for the biosversions API.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSVersionSpec">
BIOSVersionSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>version</code><br/>
<em>
string
</em>
</td>
<td>
<p>Version contains BIOS version to upgrade to</p>
</td>
</tr>
<tr>
<td>
<code>updatePolicy</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.UpdatePolicy">
UpdatePolicy
</a>
</em>
</td>
<td>
<p>An indication of whether the server&rsquo;s upgrade service should bypass vendor update policies</p>
</td>
</tr>
<tr>
<td>
<code>image</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ImageSpec">
ImageSpec
</a>
</em>
</td>
<td>
<p>details regarding the image to use to upgrade to given BIOS version</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to a specific server to apply bios upgrade on.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenancePolicy</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenancePolicy">
ServerMaintenancePolicy
</a>
</em>
</td>
<td>
<p>ServerMaintenancePolicy is maintenance policy to be enforced on the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>ServerMaintenanceRef is a reference to a ServerMaintenance object that that Controller has requested for the referred server.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSVersionStatus">
BIOSVersionStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSVersionSpec">BIOSVersionSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSVersion">BIOSVersion</a>)
</p>
<div>
<p>BIOSVersionSpec defines the desired state of BIOSVersion.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>version</code><br/>
<em>
string
</em>
</td>
<td>
<p>Version contains BIOS version to upgrade to</p>
</td>
</tr>
<tr>
<td>
<code>updatePolicy</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.UpdatePolicy">
UpdatePolicy
</a>
</em>
</td>
<td>
<p>An indication of whether the server&rsquo;s upgrade service should bypass vendor update policies</p>
</td>
</tr>
<tr>
<td>
<code>image</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ImageSpec">
ImageSpec
</a>
</em>
</td>
<td>
<p>details regarding the image to use to upgrade to given BIOS version</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to a specific server to apply bios upgrade on.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenancePolicy</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenancePolicy">
ServerMaintenancePolicy
</a>
</em>
</td>
<td>
<p>ServerMaintenancePolicy is maintenance policy to be enforced on the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>ServerMaintenanceRef is a reference to a ServerMaintenance object that that Controller has requested for the referred server.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSVersionState">BIOSVersionState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSVersionStatus">BIOSVersionStatus</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Completed&#34;</p></td>
<td><p>BIOSVersionStateCompleted specifies that the bios upgrade maintenance has been completed.</p>
</td>
</tr><tr><td><p>&#34;Failed&#34;</p></td>
<td><p>BIOSVersionStateFailed specifies that the bios upgrade maintenance has failed.</p>
</td>
</tr><tr><td><p>&#34;Processing&#34;</p></td>
<td><p>BIOSVersionStateInProgress specifies that upgrading bios is in progress.</p>
</td>
</tr><tr><td><p>&#34;Pending&#34;</p></td>
<td><p>BIOSVersionStatePending specifies that the bios upgrade maintenance is waiting</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSVersionStatus">BIOSVersionStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSVersion">BIOSVersion</a>)
</p>
<div>
<p>BIOSVersionStatus defines the observed state of BIOSVersion.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSVersionState">
BIOSVersionState
</a>
</em>
</td>
<td>
<p>State represents the current state of the bios configuration task.</p>
</td>
</tr>
<tr>
<td>
<code>upgradeTask</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.TaskStatus">
TaskStatus
</a>
</em>
</td>
<td>
<p>UpgradeTask contains the state of the Upgrade Task created by the BMC</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Conditions represents the latest available observations of the Bios version upgrade state.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMC">BMC
</h3>
<div>
<p>BMC is the Schema for the bmcs API</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCSpec">
BMCSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>uuid</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>UUID is the unique identifier for the BMC as defined in REDFISH API.
This field is optional and can be omitted, controller will choose the first avaialbe Manager</p>
</td>
</tr>
<tr>
<td>
<code>endpointRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>EndpointRef is a reference to the Kubernetes object that contains the endpoint information for the BMC.
This reference is typically used to locate the BMC endpoint within the cluster.</p>
</td>
</tr>
<tr>
<td>
<code>access</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.InlineEndpoint">
InlineEndpoint
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Endpoint allows inline configuration of network access details for the BMC.
Use this field if access settings like address are to be configured directly within the BMC resource.</p>
</td>
</tr>
<tr>
<td>
<code>bmcSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BMCSecretRef is a reference to the Kubernetes Secret object that contains the credentials
required to access the BMC. This secret includes sensitive information such as usernames and passwords.</p>
</td>
</tr>
<tr>
<td>
<code>protocol</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Protocol">
Protocol
</a>
</em>
</td>
<td>
<p>Protocol specifies the protocol to be used for communicating with the BMC.
It could be a standard protocol such as IPMI or Redfish.</p>
</td>
</tr>
<tr>
<td>
<code>consoleProtocol</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ConsoleProtocol">
ConsoleProtocol
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ConsoleProtocol specifies the protocol to be used for console access to the BMC.
This field is optional and can be omitted if console access is not required.</p>
</td>
</tr>
<tr>
<td>
<code>bmcSettingsRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BMCSettingRef is a reference to a BMCSettings object that specifies
the BMC configuration for this BMC.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCStatus">
BMCStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCAccess">BMCAccess
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerSpec">ServerSpec</a>)
</p>
<div>
<p>BMCAccess defines the access details for the BMC.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>protocol</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Protocol">
Protocol
</a>
</em>
</td>
<td>
<p>Protocol specifies the protocol to be used for communicating with the BMC.</p>
</td>
</tr>
<tr>
<td>
<code>address</code><br/>
<em>
string
</em>
</td>
<td>
<p>Address is the address of the BMC.</p>
</td>
</tr>
<tr>
<td>
<code>bmcSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BMCSecretRef is a reference to the Kubernetes Secret object that contains the credentials
required to access the BMC. This secret includes sensitive information such as usernames and passwords.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCPowerState">BMCPowerState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCStatus">BMCStatus</a>)
</p>
<div>
<p>BMCPowerState defines the possible power states for a BMC.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Off&#34;</p></td>
<td><p>OffPowerState the system is powered off, although some components may
continue to have AUX power such as management controller.</p>
</td>
</tr><tr><td><p>&#34;On&#34;</p></td>
<td><p>OnPowerState the system is powered on.</p>
</td>
</tr><tr><td><p>&#34;Paused&#34;</p></td>
<td><p>PausedPowerState the system is paused.</p>
</td>
</tr><tr><td><p>&#34;PoweringOff&#34;</p></td>
<td><p>PoweringOffPowerState A temporary state between On and Off. The power
off action can take time while the OS is in the shutdown process.</p>
</td>
</tr><tr><td><p>&#34;PoweringOn&#34;</p></td>
<td><p>PoweringOnPowerState A temporary state between Off and On. This
temporary state can be very short.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCSecret">BMCSecret
</h3>
<div>
<p>BMCSecret is the Schema for the bmcsecrets API</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Standard object&rsquo;s metadata.
More info: <a href="https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata">https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata</a></p>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>immutable</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Immutable, if set to true, ensures that data stored in the Secret cannot
be updated (only object metadata can be modified).
If not set to true, the field can be modified at any time.
Defaulted to nil.</p>
</td>
</tr>
<tr>
<td>
<code>data</code><br/>
<em>
map[string][]byte
</em>
</td>
<td>
<em>(Optional)</em>
<p>Data contains the secret data. Each key must consist of alphanumeric
characters, &lsquo;-&rsquo;, &lsquo;_&rsquo; or &lsquo;.&rsquo;. The serialized form of the secret data is a
base64 encoded string, representing the arbitrary (possibly non-string)
data value here. Described in <a href="https://tools.ietf.org/html/rfc4648#section-4">https://tools.ietf.org/html/rfc4648#section-4</a></p>
</td>
</tr>
<tr>
<td>
<code>stringData</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>stringData allows specifying non-binary secret data in string form.
It is provided as a write-only input field for convenience.
All keys and values are merged into the data field on write, overwriting any existing values.
The stringData field is never output when reading from the API.</p>
</td>
</tr>
<tr>
<td>
<code>type</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#secrettype-v1-core">
Kubernetes core/v1.SecretType
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Used to facilitate programmatic handling of secret data.
More info: <a href="https://kubernetes.io/docs/concepts/configuration/secret/#secret-types">https://kubernetes.io/docs/concepts/configuration/secret/#secret-types</a></p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCSettings">BMCSettings
</h3>
<div>
<p>BMCSettings is the Schema for the BMCSettings API.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCSettingsSpec">
BMCSettingsSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>version</code><br/>
<em>
string
</em>
</td>
<td>
<p>Version contains BMC version this settings applies to</p>
</td>
</tr>
<tr>
<td>
<code>settings</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>SettingsMap contains bmc settings as map</p>
</td>
</tr>
<tr>
<td>
<code>BMCRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BMCRef is a reference to a specific BMC to apply setting to.
ServerRef is ignored if BMCRef is set</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenancePolicy</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenancePolicy">
ServerMaintenancePolicy
</a>
</em>
</td>
<td>
<p>ServerMaintenancePolicy is maintenance policy to be enforced on the server when applying setting.
ServerMaintenancePolicyOwnerApproval is asking for User approval for changing BMC settings
note: User approval is only enforced for server&rsquo;s which are reserved state
ServerMaintenancePolicyEnforced will not create a maintenance request even if bmc reboot is needed.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRefList</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenanceRefList">
[]ServerMaintenanceRefList
</a>
</em>
</td>
<td>
<p>ServerMaintenanceRefList are references to a ServerMaintenance objects that Controller has requested for the each of the related server.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCSettingsStatus">
BMCSettingsStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCSettingsSpec">BMCSettingsSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCSettings">BMCSettings</a>)
</p>
<div>
<p>BMCSettingsSpec defines the desired state of BMCSettings.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>version</code><br/>
<em>
string
</em>
</td>
<td>
<p>Version contains BMC version this settings applies to</p>
</td>
</tr>
<tr>
<td>
<code>settings</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>SettingsMap contains bmc settings as map</p>
</td>
</tr>
<tr>
<td>
<code>BMCRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BMCRef is a reference to a specific BMC to apply setting to.
ServerRef is ignored if BMCRef is set</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenancePolicy</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenancePolicy">
ServerMaintenancePolicy
</a>
</em>
</td>
<td>
<p>ServerMaintenancePolicy is maintenance policy to be enforced on the server when applying setting.
ServerMaintenancePolicyOwnerApproval is asking for User approval for changing BMC settings
note: User approval is only enforced for server&rsquo;s which are reserved state
ServerMaintenancePolicyEnforced will not create a maintenance request even if bmc reboot is needed.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRefList</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenanceRefList">
[]ServerMaintenanceRefList
</a>
</em>
</td>
<td>
<p>ServerMaintenanceRefList are references to a ServerMaintenance objects that Controller has requested for the each of the related server.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCSettingsState">BMCSettingsState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCSettingsStatus">BMCSettingsStatus</a>)
</p>
<div>
<p>ServerMaintenanceState specifies the current state of the server maintenance.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Applied&#34;</p></td>
<td><p>BMCSettingsStateApplied specifies that the BMC maintenance has been completed.</p>
</td>
</tr><tr><td><p>&#34;Failed&#34;</p></td>
<td><p>BMCSettingsStateFailed specifies that the BMC maintenance has failed.</p>
</td>
</tr><tr><td><p>&#34;InProgress&#34;</p></td>
<td><p>BMCSettingsStateInProgress specifies that the BMC setting changes are in progress</p>
</td>
</tr><tr><td><p>&#34;Pending&#34;</p></td>
<td><p>BMCSettingsStatePending specifies that the BMC maintenance is waiting</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCSettingsStatus">BMCSettingsStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCSettings">BMCSettings</a>)
</p>
<div>
<p>BMCSettingsStatus defines the observed state of BMCSettings.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCSettingsState">
BMCSettingsState
</a>
</em>
</td>
<td>
<p>State represents the current state of the BMC configuration task.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCSpec">BMCSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMC">BMC</a>)
</p>
<div>
<p>BMCSpec defines the desired state of BMC</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>uuid</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>UUID is the unique identifier for the BMC as defined in REDFISH API.
This field is optional and can be omitted, controller will choose the first avaialbe Manager</p>
</td>
</tr>
<tr>
<td>
<code>endpointRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>EndpointRef is a reference to the Kubernetes object that contains the endpoint information for the BMC.
This reference is typically used to locate the BMC endpoint within the cluster.</p>
</td>
</tr>
<tr>
<td>
<code>access</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.InlineEndpoint">
InlineEndpoint
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Endpoint allows inline configuration of network access details for the BMC.
Use this field if access settings like address are to be configured directly within the BMC resource.</p>
</td>
</tr>
<tr>
<td>
<code>bmcSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BMCSecretRef is a reference to the Kubernetes Secret object that contains the credentials
required to access the BMC. This secret includes sensitive information such as usernames and passwords.</p>
</td>
</tr>
<tr>
<td>
<code>protocol</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Protocol">
Protocol
</a>
</em>
</td>
<td>
<p>Protocol specifies the protocol to be used for communicating with the BMC.
It could be a standard protocol such as IPMI or Redfish.</p>
</td>
</tr>
<tr>
<td>
<code>consoleProtocol</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ConsoleProtocol">
ConsoleProtocol
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ConsoleProtocol specifies the protocol to be used for console access to the BMC.
This field is optional and can be omitted if console access is not required.</p>
</td>
</tr>
<tr>
<td>
<code>bmcSettingsRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BMCSettingRef is a reference to a BMCSettings object that specifies
the BMC configuration for this BMC.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCState">BMCState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCStatus">BMCStatus</a>)
</p>
<div>
<p>BMCState defines the possible states of a BMC.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Enabled&#34;</p></td>
<td><p>BMCStateEnabled indicates that the BMC is enabled and functioning correctly.</p>
</td>
</tr><tr><td><p>&#34;Error&#34;</p></td>
<td><p>BMCStateError indicates that there is an error with the BMC.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCStatus">BMCStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMC">BMC</a>)
</p>
<div>
<p>BMCStatus defines the observed state of BMC.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>macAddress</code><br/>
<em>
string
</em>
</td>
<td>
<p>MACAddress is the MAC address of the BMC.
The format is validated using a regular expression pattern.</p>
</td>
</tr>
<tr>
<td>
<code>ip</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.IP">
IP
</a>
</em>
</td>
<td>
<p>IP is the IP address of the BMC.
The type is specified as string and is schemaless.</p>
</td>
</tr>
<tr>
<td>
<code>manufacturer</code><br/>
<em>
string
</em>
</td>
<td>
<p>Manufacturer is the name of the BMC manufacturer.</p>
</td>
</tr>
<tr>
<td>
<code>model</code><br/>
<em>
string
</em>
</td>
<td>
<p>Model is the model number or name of the BMC.</p>
</td>
</tr>
<tr>
<td>
<code>sku</code><br/>
<em>
string
</em>
</td>
<td>
<p>SKU is the stock keeping unit identifier for the BMC.</p>
</td>
</tr>
<tr>
<td>
<code>serialNumber</code><br/>
<em>
string
</em>
</td>
<td>
<p>SerialNumber is the serial number of the BMC.</p>
</td>
</tr>
<tr>
<td>
<code>firmwareVersion</code><br/>
<em>
string
</em>
</td>
<td>
<p>FirmwareVersion is the version of the firmware currently running on the BMC.</p>
</td>
</tr>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCState">
BMCState
</a>
</em>
</td>
<td>
<p>State represents the current state of the BMC.</p>
</td>
</tr>
<tr>
<td>
<code>powerState</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCPowerState">
BMCPowerState
</a>
</em>
</td>
<td>
<p>PowerState represents the current power state of the BMC.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Conditions represents the latest available observations of the BMC&rsquo;s current state.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BootOrder">BootOrder
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerSpec">ServerSpec</a>)
</p>
<div>
<p>BootOrder represents the boot order of the server.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name is the name of the boot device.</p>
</td>
</tr>
<tr>
<td>
<code>priority</code><br/>
<em>
int
</em>
</td>
<td>
<p>Priority is the priority of the boot device.</p>
</td>
</tr>
<tr>
<td>
<code>device</code><br/>
<em>
string
</em>
</td>
<td>
<p>Device is the device to boot from.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ConsoleProtocol">ConsoleProtocol
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCSpec">BMCSpec</a>)
</p>
<div>
<p>ConsoleProtocol defines the protocol and port used for console access to the BMC.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ConsoleProtocolName">
ConsoleProtocolName
</a>
</em>
</td>
<td>
<p>Name specifies the name of the console protocol.
This could be a protocol such as &ldquo;SSH&rdquo;, &ldquo;Telnet&rdquo;, etc.</p>
</td>
</tr>
<tr>
<td>
<code>port</code><br/>
<em>
int32
</em>
</td>
<td>
<p>Port specifies the port number used for console access.
This port is used by the specified console protocol to establish connections.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ConsoleProtocolName">ConsoleProtocolName
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ConsoleProtocol">ConsoleProtocol</a>)
</p>
<div>
<p>ConsoleProtocolName defines the possible names for console protocols.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;IPMI&#34;</p></td>
<td><p>ConsoleProtocolNameIPMI represents the IPMI console protocol.</p>
</td>
</tr><tr><td><p>&#34;SSH&#34;</p></td>
<td><p>ConsoleProtocolNameSSH represents the SSH console protocol.</p>
</td>
</tr><tr><td><p>&#34;SSHLenovo&#34;</p></td>
<td><p>ConsoleProtocolNameSSHLenovo represents the SSH console protocol specific to Lenovo hardware.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.Endpoint">Endpoint
</h3>
<div>
<p>Endpoint is the Schema for the endpoints API</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.EndpointSpec">
EndpointSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>macAddress</code><br/>
<em>
string
</em>
</td>
<td>
<p>MACAddress is the MAC address of the endpoint.</p>
</td>
</tr>
<tr>
<td>
<code>ip</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.IP">
IP
</a>
</em>
</td>
<td>
<p>IP is the IP address of the endpoint.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.EndpointStatus">
EndpointStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.EndpointSpec">EndpointSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.Endpoint">Endpoint</a>)
</p>
<div>
<p>EndpointSpec defines the desired state of Endpoint</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>macAddress</code><br/>
<em>
string
</em>
</td>
<td>
<p>MACAddress is the MAC address of the endpoint.</p>
</td>
</tr>
<tr>
<td>
<code>ip</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.IP">
IP
</a>
</em>
</td>
<td>
<p>IP is the IP address of the endpoint.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.EndpointStatus">EndpointStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.Endpoint">Endpoint</a>)
</p>
<div>
<p>EndpointStatus defines the observed state of Endpoint</p>
</div>
<h3 id="metal.ironcore.dev/v1alpha1.IP">IP
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCStatus">BMCStatus</a>, <a href="#metal.ironcore.dev/v1alpha1.EndpointSpec">EndpointSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.InlineEndpoint">InlineEndpoint</a>, <a href="#metal.ironcore.dev/v1alpha1.NetworkInterface">NetworkInterface</a>)
</p>
<div>
<p>IP is an IP address.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>-</code><br/>
<em>
<a href="https://pkg.go.dev/net/netip#Addr">
net/netip.Addr
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.IPPrefix">IPPrefix
</h3>
<div>
<p>IPPrefix represents a network prefix.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>-</code><br/>
<em>
<a href="https://pkg.go.dev/net/netip#Prefix">
net/netip.Prefix
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ImageSpec">ImageSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSVersionSpec">BIOSVersionSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>secretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ImageSecretRef is a reference to the Kubernetes Secret (of type SecretTypeBasicAuth) object that contains the credentials
to access the ImageURI. This secret includes sensitive information such as usernames and passwords.</p>
</td>
</tr>
<tr>
<td>
<code>transferProtocol</code><br/>
<em>
string
</em>
</td>
<td>
<p>The network protocol that the server&rsquo;s update service uses to retrieve &lsquo;ImageURI&rsquo;</p>
</td>
</tr>
<tr>
<td>
<code>URI</code><br/>
<em>
string
</em>
</td>
<td>
<p>The URI of the software image to update/install.&rdquo;</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ImageSpec">ImageSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSVersionSpec">BIOSVersionSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>secretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ImageSecretRef is a reference to the Kubernetes Secret (of type SecretTypeBasicAuth) object that contains the credentials
to access the ImageURI. This secret includes sensitive information such as usernames and passwords.</p>
</td>
</tr>
<tr>
<td>
<code>transferProtocol</code><br/>
<em>
string
</em>
</td>
<td>
<p>The network protocol that the server&rsquo;s update service uses to retrieve &lsquo;ImageURI&rsquo;</p>
</td>
</tr>
<tr>
<td>
<code>URI</code><br/>
<em>
string
</em>
</td>
<td>
<p>The URI of the software image to update/install.&rdquo;</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.OOBMSettings">OOBMSettings
</h3>
<div>
<p>OOBMSettings is the Schema for the oobmsettings API.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.OOBMSettingsSpec">
OOBMSettingsSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>OoBM</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.OOBMSettingsMap">
OOBMSettingsMap
</a>
</em>
</td>
<td>
<p>OOBMSettings specifies the OoBM settings for the selected serverRef&rsquo;s Out-of-Band-Management</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to a specific server&rsquo;s Manager to apply setting to.</p>
</td>
</tr>
<tr>
<td>
<code>BMCRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BMCRef is a reference to a specific BMC to apply setting to.</p>
</td>
</tr>
<tr>
<td>
<code>ServerMaintenancePolicyTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenancePolicy">
ServerMaintenancePolicy
</a>
</em>
</td>
<td>
<p>ServerMaintenancePolicy is maintenance policy to be enforced on the server when applying setting.
ServerMaintenancePolicyOwnerApproval is asking for human approval
ServerMaintenancePolicyEnforced will not create a maintenance request.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>ServerMaintenanceRef is a reference to a ServerMaintenance object that that OoBM has requested for the referred server.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.OOBMSettingsStatus">
OOBMSettingsStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.OOBMSettingsMap">OOBMSettingsMap
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.OOBMSettingsSpec">OOBMSettingsSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>version</code><br/>
<em>
string
</em>
</td>
<td>
<p>Version contains OoBM version</p>
</td>
</tr>
<tr>
<td>
<code>settings</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Settings contains OoBM settings as map</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.OOBMSettingsSpec">OOBMSettingsSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.OOBMSettings">OOBMSettings</a>)
</p>
<div>
<p>OOBMSettingsSpec defines the desired state of OOBMSettings.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>OoBM</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.OOBMSettingsMap">
OOBMSettingsMap
</a>
</em>
</td>
<td>
<p>OOBMSettings specifies the OoBM settings for the selected serverRef&rsquo;s Out-of-Band-Management</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to a specific server&rsquo;s Manager to apply setting to.</p>
</td>
</tr>
<tr>
<td>
<code>BMCRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BMCRef is a reference to a specific BMC to apply setting to.</p>
</td>
</tr>
<tr>
<td>
<code>ServerMaintenancePolicyTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenancePolicy">
ServerMaintenancePolicy
</a>
</em>
</td>
<td>
<p>ServerMaintenancePolicy is maintenance policy to be enforced on the server when applying setting.
ServerMaintenancePolicyOwnerApproval is asking for human approval
ServerMaintenancePolicyEnforced will not create a maintenance request.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>ServerMaintenanceRef is a reference to a ServerMaintenance object that that OoBM has requested for the referred server.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.OOBMSettingsStatus">OOBMSettingsStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.OOBMSettings">OOBMSettings</a>)
</p>
<div>
<p>OOBMSettingsStatus defines the observed state of OOBMSettings.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.OoBMMaintenanceState">
OoBMMaintenanceState
</a>
</em>
</td>
<td>
<p>State represents the current state of the OoBM configuration task.</p>
</td>
</tr>
<tr>
<td>
<code>updateSettingState</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.OoBMSettingUpdateState">
OoBMSettingUpdateState
</a>
</em>
</td>
<td>
<p>UpdateSettingState represents the current state of the OoBM setting update task.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.OoBMMaintenanceState">OoBMMaintenanceState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.OOBMSettingsStatus">OOBMSettingsStatus</a>)
</p>
<div>
<p>ServerMaintenanceState specifies the current state of the server maintenance.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Failed&#34;</p></td>
<td><p>OoBMMaintenanceStateFailed specifies that the server maintenance has failed.</p>
</td>
</tr><tr><td><p>&#34;InSettingUpdate&#34;</p></td>
<td><p>OoBMMaintenanceStateInSettingUpdate specifies that the server OoBM is in setting update path.</p>
</td>
</tr><tr><td><p>&#34;InVersionUpgrade&#34;</p></td>
<td><p>OoBMMaintenanceStateInVersionUpgrade specifies that the server OoBM is in version upgrade path.</p>
</td>
</tr><tr><td><p>&#34;SyncSettingsCompleted&#34;</p></td>
<td><p>OoBMMaintenanceStateSynced specifies that the server OoBM maintenance has been completed.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.OoBMSettingUpdateState">OoBMSettingUpdateState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.OOBMSettingsStatus">OOBMSettingsStatus</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;IssueSettingUpdate&#34;</p></td>
<td><p>SettingUpdateStateIssued specifies that the OoBM new setting was posted to RedFish</p>
</td>
</tr><tr><td><p>&#34;VerifySettingUpdate&#34;</p></td>
<td><p>SettingUpdateStateCompleted specifies that the OoBM setting has been completed.</p>
</td>
</tr><tr><td><p>&#34;WaitOnServerRebootPowerOff&#34;</p></td>
<td><p>SettingUpdateStateWaitOnServerReboot specifies that the OoBM setting state is waiting on server to turn off during Reboot.</p>
</td>
</tr><tr><td><p>&#34;WaitOnServerRebootPowerOn&#34;</p></td>
<td><p>OoBMSettingUpdateWaitOnServerRebootPowerOn specifies that the OoBM setting state is waiting on server to turn on during Reboot.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.Phase">Phase
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerClaimStatus">ServerClaimStatus</a>)
</p>
<div>
<p>Phase defines the possible phases of a ServerClaim.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Bound&#34;</p></td>
<td><p>PhaseBound indicates that the server claim is bound to a server.</p>
</td>
</tr><tr><td><p>&#34;Unbound&#34;</p></td>
<td><p>PhaseUnbound indicates that the server claim is not bound to any server.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.Power">Power
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerClaimSpec">ServerClaimSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.ServerMaintenanceSpec">ServerMaintenanceSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.ServerSpec">ServerSpec</a>)
</p>
<div>
<p>Power defines the possible power states for a device.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Off&#34;</p></td>
<td><p>PowerOff indicates that the device is powered off.</p>
</td>
</tr><tr><td><p>&#34;On&#34;</p></td>
<td><p>PowerOn indicates that the device is powered on.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.Processor">Processor
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerStatus">ServerStatus</a>)
</p>
<div>
<p>Processor defines the details of a Processor.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>id</code><br/>
<em>
string
</em>
</td>
<td>
<p>ID is the name of the Processor.</p>
</td>
</tr>
<tr>
<td>
<code>type</code><br/>
<em>
string
</em>
</td>
<td>
<p>Type is the type of the Processor.</p>
</td>
</tr>
<tr>
<td>
<code>architecture</code><br/>
<em>
string
</em>
</td>
<td>
<p>Architecture is the architecture of the Processor.</p>
</td>
</tr>
<tr>
<td>
<code>instructionSet</code><br/>
<em>
string
</em>
</td>
<td>
<p>InstructionSet is the instruction set of the Processor.</p>
</td>
</tr>
<tr>
<td>
<code>manufacturer</code><br/>
<em>
string
</em>
</td>
<td>
<p>Manufacturer is the manufacturer of the Processor.</p>
</td>
</tr>
<tr>
<td>
<code>model</code><br/>
<em>
string
</em>
</td>
<td>
<p>Model is the model of the Processor.</p>
</td>
</tr>
<tr>
<td>
<code>maxSpeedMHz</code><br/>
<em>
int32
</em>
</td>
<td>
<p>MaxSpeedMHz is the maximum speed of the Processor in MHz.</p>
</td>
</tr>
<tr>
<td>
<code>totalCores</code><br/>
<em>
int32
</em>
</td>
<td>
<p>TotalCores is the total number of cores in the Processor.</p>
</td>
</tr>
<tr>
<td>
<code>totalThreads</code><br/>
<em>
int32
</em>
</td>
<td>
<p>TotalThreads is the total number of threads in the Processor.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.Protocol">Protocol
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCAccess">BMCAccess</a>, <a href="#metal.ironcore.dev/v1alpha1.BMCSpec">BMCSpec</a>)
</p>
<div>
<p>Protocol defines the protocol and port used for communicating with the BMC.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ProtocolName">
ProtocolName
</a>
</em>
</td>
<td>
<p>Name specifies the name of the protocol.
This could be a protocol such as &ldquo;IPMI&rdquo;, &ldquo;Redfish&rdquo;, etc.</p>
</td>
</tr>
<tr>
<td>
<code>port</code><br/>
<em>
int32
</em>
</td>
<td>
<p>Port specifies the port number used for communication.
This port is used by the specified protocol to establish connections.</p>
</td>
</tr>
<tr>
<td>
<code>scheme</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ProtocolScheme">
ProtocolScheme
</a>
</em>
</td>
<td>
<p>Scheme specifies the scheme used for communication.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ProtocolName">ProtocolName
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.Protocol">Protocol</a>)
</p>
<div>
<p>ProtocolName defines the possible names for protocols used for communicating with the BMC.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;IPMI&#34;</p></td>
<td><p>ProtocolNameIPMI represents the IPMI protocol.</p>
</td>
</tr><tr><td><p>&#34;Redfish&#34;</p></td>
<td><p>ProtocolNameRedfish represents the Redfish protocol.</p>
</td>
</tr><tr><td><p>&#34;SSH&#34;</p></td>
<td><p>ProtocolNameSSH represents the SSH protocol.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ProtocolScheme">ProtocolScheme
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.Protocol">Protocol</a>)
</p>
<div>
<p>ProtocolScheme is a string that contains the protocol scheme</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;http&#34;</p></td>
<td><p>HTTPProtocolScheme is the http protocol scheme</p>
</td>
</tr><tr><td><p>&#34;https&#34;</p></td>
<td><p>HTTPSProtocolScheme is the https protocol scheme</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.Server">Server
</h3>
<div>
<p>Server is the Schema for the servers API</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerSpec">
ServerSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>uuid</code><br/>
<em>
string
</em>
</td>
<td>
<p>UUID is the unique identifier for the server.
Deprecated in favor of systemUUID.</p>
</td>
</tr>
<tr>
<td>
<code>systemUUID</code><br/>
<em>
string
</em>
</td>
<td>
<p>SystemUUID is the unique identifier for the server.</p>
</td>
</tr>
<tr>
<td>
<code>power</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Power">
Power
</a>
</em>
</td>
<td>
<p>Power specifies the desired power state of the server.</p>
</td>
</tr>
<tr>
<td>
<code>indicatorLED</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.IndicatorLED">
IndicatorLED
</a>
</em>
</td>
<td>
<p>IndicatorLED specifies the desired state of the server&rsquo;s indicator LED.</p>
</td>
</tr>
<tr>
<td>
<code>serverClaimRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>ServerClaimRef is a reference to a ServerClaim object that claims this server.
This field is optional and can be omitted if no claim is associated with this server.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>ServerMaintenanceRef is a reference to a ServerMaintenance object that maintains this server.</p>
</td>
</tr>
<tr>
<td>
<code>bmcRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BMCRef is a reference to the BMC object associated with this server.
This field is optional and can be omitted if no BMC is associated with this server.</p>
</td>
</tr>
<tr>
<td>
<code>bmc</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCAccess">
BMCAccess
</a>
</em>
</td>
<td>
<p>BMC contains the access details for the BMC.
This field is optional and can be omitted if no BMC access is specified.</p>
</td>
</tr>
<tr>
<td>
<code>bootConfigurationRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>BootConfigurationRef is a reference to a BootConfiguration object that specifies
the boot configuration for this server. This field is optional and can be omitted
if no boot configuration is specified.</p>
</td>
</tr>
<tr>
<td>
<code>maintenanceBootConfigurationRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>MaintenanceBootConfigurationRef is a reference to a BootConfiguration object that specifies
the boot configuration for this server during maintenance. This field is optional and can be omitted</p>
</td>
</tr>
<tr>
<td>
<code>bootOrder</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BootOrder">
[]BootOrder
</a>
</em>
</td>
<td>
<p>BootOrder specifies the boot order of the server.</p>
</td>
</tr>
<tr>
<td>
<code>biosSettingsRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BIOSSettingsRef is a reference to a biossettings object that specifies
the BIOS configuration for this server.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerStatus">
ServerStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerBootConfiguration">ServerBootConfiguration
</h3>
<div>
<p>ServerBootConfiguration is the Schema for the serverbootconfigurations API</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerBootConfigurationSpec">
ServerBootConfigurationSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to the server for which this boot configuration is intended.</p>
</td>
</tr>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
<p>Image specifies the boot image to be used for the server.
This field is optional and can be omitted if not specified.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>IgnitionSecretRef is a reference to the Kubernetes Secret object that contains
the ignition configuration for the server. This field is optional and can be omitted if not specified.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerBootConfigurationStatus">
ServerBootConfigurationStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerBootConfigurationSpec">ServerBootConfigurationSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerBootConfiguration">ServerBootConfiguration</a>, <a href="#metal.ironcore.dev/v1alpha1.ServerBootConfigurationTemplate">ServerBootConfigurationTemplate</a>)
</p>
<div>
<p>ServerBootConfigurationSpec defines the desired state of ServerBootConfiguration.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to the server for which this boot configuration is intended.</p>
</td>
</tr>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
<p>Image specifies the boot image to be used for the server.
This field is optional and can be omitted if not specified.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>IgnitionSecretRef is a reference to the Kubernetes Secret object that contains
the ignition configuration for the server. This field is optional and can be omitted if not specified.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerBootConfigurationState">ServerBootConfigurationState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerBootConfigurationStatus">ServerBootConfigurationStatus</a>)
</p>
<div>
<p>ServerBootConfigurationState defines the possible states of a ServerBootConfiguration.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Error&#34;</p></td>
<td><p>ServerBootConfigurationStateError indicates that there is an error with the boot configuration.</p>
</td>
</tr><tr><td><p>&#34;Pending&#34;</p></td>
<td><p>ServerBootConfigurationStatePending indicates that the boot configuration is pending and not yet ready.</p>
</td>
</tr><tr><td><p>&#34;Ready&#34;</p></td>
<td><p>ServerBootConfigurationStateReady indicates that the boot configuration is ready for use.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerBootConfigurationStatus">ServerBootConfigurationStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerBootConfiguration">ServerBootConfiguration</a>)
</p>
<div>
<p>ServerBootConfigurationStatus defines the observed state of ServerBootConfiguration.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerBootConfigurationState">
ServerBootConfigurationState
</a>
</em>
</td>
<td>
<p>State represents the current state of the boot configuration.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerBootConfigurationTemplate">ServerBootConfigurationTemplate
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerMaintenanceSpec">ServerMaintenanceSpec</a>)
</p>
<div>
<p>ServerBootConfigurationTemplate defines the parameters to be used for rendering a boot configuration.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name specifies the name of the boot configuration.</p>
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerBootConfigurationSpec">
ServerBootConfigurationSpec
</a>
</em>
</td>
<td>
<p>Parameters specifies the parameters to be used for rendering the boot configuration.</p>
<br/>
<br/>
<table>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to the server for which this boot configuration is intended.</p>
</td>
</tr>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
<p>Image specifies the boot image to be used for the server.
This field is optional and can be omitted if not specified.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>IgnitionSecretRef is a reference to the Kubernetes Secret object that contains
the ignition configuration for the server. This field is optional and can be omitted if not specified.</p>
</td>
</tr>
</table>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerClaim">ServerClaim
</h3>
<div>
<p>ServerClaim is the Schema for the serverclaims API</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerClaimSpec">
ServerClaimSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>power</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Power">
Power
</a>
</em>
</td>
<td>
<p>Power specifies the desired power state of the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to a specific server to be claimed.
This field is optional and can be omitted if the server is to be selected using ServerSelector.</p>
</td>
</tr>
<tr>
<td>
<code>serverSelector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<p>ServerSelector specifies a label selector to identify the server to be claimed.
This field is optional and can be omitted if a specific server is referenced using ServerRef.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>IgnitionSecretRef is a reference to the Kubernetes Secret object that contains
the ignition configuration for the server. This field is optional and can be omitted if not specified.</p>
</td>
</tr>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
<p>Image specifies the boot image to be used for the server.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerClaimStatus">
ServerClaimStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerClaimSpec">ServerClaimSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerClaim">ServerClaim</a>)
</p>
<div>
<p>ServerClaimSpec defines the desired state of ServerClaim.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>power</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Power">
Power
</a>
</em>
</td>
<td>
<p>Power specifies the desired power state of the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to a specific server to be claimed.
This field is optional and can be omitted if the server is to be selected using ServerSelector.</p>
</td>
</tr>
<tr>
<td>
<code>serverSelector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<p>ServerSelector specifies a label selector to identify the server to be claimed.
This field is optional and can be omitted if a specific server is referenced using ServerRef.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>IgnitionSecretRef is a reference to the Kubernetes Secret object that contains
the ignition configuration for the server. This field is optional and can be omitted if not specified.</p>
</td>
</tr>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
<p>Image specifies the boot image to be used for the server.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerClaimStatus">ServerClaimStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerClaim">ServerClaim</a>)
</p>
<div>
<p>ServerClaimStatus defines the observed state of ServerClaim.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>phase</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Phase">
Phase
</a>
</em>
</td>
<td>
<p>Phase represents the current phase of the server claim.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerMaintenance">ServerMaintenance
</h3>
<div>
<p>ServerMaintenance is the Schema for the ServerMaintenance API</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenanceSpec">
ServerMaintenanceSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>policy</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenancePolicy">
ServerMaintenancePolicy
</a>
</em>
</td>
<td>
<p>Policy specifies the maintenance policy to be enforced on the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to the server that is to be maintained.</p>
</td>
</tr>
<tr>
<td>
<code>serverPower</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Power">
Power
</a>
</em>
</td>
<td>
<p>ServerPower specifies the power state of the server during maintenance.</p>
</td>
</tr>
<tr>
<td>
<code>serverBootConfigurationTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerBootConfigurationTemplate">
ServerBootConfigurationTemplate
</a>
</em>
</td>
<td>
<p>ServerBootConfigurationTemplate specifies the boot configuration to be applied to the server during maintenance.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenanceStatus">
ServerMaintenanceStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerMaintenancePolicy">ServerMaintenancePolicy
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsSpec">BIOSSettingsSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.BIOSVersionSpec">BIOSVersionSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.BMCSettingsSpec">BMCSettingsSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.ServerMaintenanceSpec">ServerMaintenanceSpec</a>)
</p>
<div>
<p>ServerMaintenancePolicy specifies the maintenance policy to be enforced on the server.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Enforced&#34;</p></td>
<td><p>ServerMaintenancePolicyEnforced specifies that the maintenance policy is enforced.</p>
</td>
</tr><tr><td><p>&#34;OwnerApproval&#34;</p></td>
<td><p>ServerMaintenancePolicyOwnerApproval specifies that the maintenance policy requires owner approval.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerMaintenanceSpec">ServerMaintenanceSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerMaintenance">ServerMaintenance</a>)
</p>
<div>
<p>ServerMaintenanceSpec defines the desired state of a ServerMaintenance</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>policy</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenancePolicy">
ServerMaintenancePolicy
</a>
</em>
</td>
<td>
<p>Policy specifies the maintenance policy to be enforced on the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to the server that is to be maintained.</p>
</td>
</tr>
<tr>
<td>
<code>serverPower</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Power">
Power
</a>
</em>
</td>
<td>
<p>ServerPower specifies the power state of the server during maintenance.</p>
</td>
</tr>
<tr>
<td>
<code>serverBootConfigurationTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerBootConfigurationTemplate">
ServerBootConfigurationTemplate
</a>
</em>
</td>
<td>
<p>ServerBootConfigurationTemplate specifies the boot configuration to be applied to the server during maintenance.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerMaintenanceState">ServerMaintenanceState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerMaintenanceStatus">ServerMaintenanceStatus</a>)
</p>
<div>
<p>ServerMaintenanceState specifies the current state of the server maintenance.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Failed&#34;</p></td>
<td><p>ServerMaintenanceStateCompleted specifies that the server maintenance has been completed.</p>
</td>
</tr><tr><td><p>&#34;InMaintenance&#34;</p></td>
<td><p>ServerMaintenanceStateInMaintenance specifies that the server is in maintenance.</p>
</td>
</tr><tr><td><p>&#34;Pending&#34;</p></td>
<td><p>ServerMaintenanceStatePending specifies that the server maintenance is pending.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerMaintenanceStatus">ServerMaintenanceStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerMaintenance">ServerMaintenance</a>)
</p>
<div>
<p>ServerMaintenanceStatus defines the observed state of a ServerMaintenance</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenanceState">
ServerMaintenanceState
</a>
</em>
</td>
<td>
<p>State specifies the current state of the server maintenance.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerPowerState">ServerPowerState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerStatus">ServerStatus</a>)
</p>
<div>
<p>ServerPowerState defines the possible power states for a server.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Off&#34;</p></td>
<td><p>ServerOffPowerState indicates that the system is powered off, although some components may
continue to have auxiliary power such as the management controller.</p>
</td>
</tr><tr><td><p>&#34;On&#34;</p></td>
<td><p>ServerOnPowerState indicates that the system is powered on.</p>
</td>
</tr><tr><td><p>&#34;Paused&#34;</p></td>
<td><p>ServerPausedPowerState indicates that the system is paused.</p>
</td>
</tr><tr><td><p>&#34;PoweringOff&#34;</p></td>
<td><p>ServerPoweringOffPowerState indicates a temporary state between On and Off.
The power off action can take time while the OS is in the shutdown process.</p>
</td>
</tr><tr><td><p>&#34;PoweringOn&#34;</p></td>
<td><p>ServerPoweringOnPowerState indicates a temporary state between Off and On.
This temporary state can be very short.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerSpec">ServerSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.Server">Server</a>)
</p>
<div>
<p>ServerSpec defines the desired state of a Server.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>uuid</code><br/>
<em>
string
</em>
</td>
<td>
<p>UUID is the unique identifier for the server.
Deprecated in favor of systemUUID.</p>
</td>
</tr>
<tr>
<td>
<code>systemUUID</code><br/>
<em>
string
</em>
</td>
<td>
<p>SystemUUID is the unique identifier for the server.</p>
</td>
</tr>
<tr>
<td>
<code>power</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Power">
Power
</a>
</em>
</td>
<td>
<p>Power specifies the desired power state of the server.</p>
</td>
</tr>
<tr>
<td>
<code>indicatorLED</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.IndicatorLED">
IndicatorLED
</a>
</em>
</td>
<td>
<p>IndicatorLED specifies the desired state of the server&rsquo;s indicator LED.</p>
</td>
</tr>
<tr>
<td>
<code>serverClaimRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>ServerClaimRef is a reference to a ServerClaim object that claims this server.
This field is optional and can be omitted if no claim is associated with this server.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>ServerMaintenanceRef is a reference to a ServerMaintenance object that maintains this server.</p>
</td>
</tr>
<tr>
<td>
<code>bmcRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BMCRef is a reference to the BMC object associated with this server.
This field is optional and can be omitted if no BMC is associated with this server.</p>
</td>
</tr>
<tr>
<td>
<code>bmc</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCAccess">
BMCAccess
</a>
</em>
</td>
<td>
<p>BMC contains the access details for the BMC.
This field is optional and can be omitted if no BMC access is specified.</p>
</td>
</tr>
<tr>
<td>
<code>bootConfigurationRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>BootConfigurationRef is a reference to a BootConfiguration object that specifies
the boot configuration for this server. This field is optional and can be omitted
if no boot configuration is specified.</p>
</td>
</tr>
<tr>
<td>
<code>maintenanceBootConfigurationRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<p>MaintenanceBootConfigurationRef is a reference to a BootConfiguration object that specifies
the boot configuration for this server during maintenance. This field is optional and can be omitted</p>
</td>
</tr>
<tr>
<td>
<code>bootOrder</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BootOrder">
[]BootOrder
</a>
</em>
</td>
<td>
<p>BootOrder specifies the boot order of the server.</p>
</td>
</tr>
<tr>
<td>
<code>biosSettingsRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BIOSSettingsRef is a reference to a biossettings object that specifies
the BIOS configuration for this server.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerState">ServerState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerStatus">ServerStatus</a>)
</p>
<div>
<p>ServerState defines the possible states of a server.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Available&#34;</p></td>
<td><p>ServerStateAvailable indicates that the server is available for use.</p>
</td>
</tr><tr><td><p>&#34;Discovery&#34;</p></td>
<td><p>ServerStateDiscovery indicates that the server is in its discovery state.</p>
</td>
</tr><tr><td><p>&#34;Error&#34;</p></td>
<td><p>ServerStateError indicates that there is an error with the server.</p>
</td>
</tr><tr><td><p>&#34;Initial&#34;</p></td>
<td><p>ServerStateInitial indicates that the server is in its initial state.</p>
</td>
</tr><tr><td><p>&#34;Maintenance&#34;</p></td>
<td><p>ServerStateMaintenance indicates that the server is in maintenance.</p>
</td>
</tr><tr><td><p>&#34;Reserved&#34;</p></td>
<td><p>ServerStateReserved indicates that the server is reserved for a specific use or user.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerStatus">ServerStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.Server">Server</a>)
</p>
<div>
<p>ServerStatus defines the observed state of Server.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>manufacturer</code><br/>
<em>
string
</em>
</td>
<td>
<p>Manufacturer is the name of the server manufacturer.</p>
</td>
</tr>
<tr>
<td>
<code>model</code><br/>
<em>
string
</em>
</td>
<td>
<p>Model is the model of the server.</p>
</td>
</tr>
<tr>
<td>
<code>sku</code><br/>
<em>
string
</em>
</td>
<td>
<p>SKU is the stock keeping unit identifier for the server.</p>
</td>
</tr>
<tr>
<td>
<code>serialNumber</code><br/>
<em>
string
</em>
</td>
<td>
<p>SerialNumber is the serial number of the server.</p>
</td>
</tr>
<tr>
<td>
<code>powerState</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerPowerState">
ServerPowerState
</a>
</em>
</td>
<td>
<p>PowerState represents the current power state of the server.</p>
</td>
</tr>
<tr>
<td>
<code>indicatorLED</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.IndicatorLED">
IndicatorLED
</a>
</em>
</td>
<td>
<p>IndicatorLED specifies the current state of the server&rsquo;s indicator LED.</p>
</td>
</tr>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerState">
ServerState
</a>
</em>
</td>
<td>
<p>State represents the current state of the server.</p>
</td>
</tr>
<tr>
<td>
<code>networkInterfaces</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.NetworkInterface">
[]NetworkInterface
</a>
</em>
</td>
<td>
<p>NetworkInterfaces is a list of network interfaces associated with the server.</p>
</td>
</tr>
<tr>
<td>
<code>totalSystemMemory</code><br/>
<em>
<a href="https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity">
k8s.io/apimachinery/pkg/api/resource.Quantity
</a>
</em>
</td>
<td>
<p>TotalSystemMemory is the total amount of memory in bytes available on the server.</p>
</td>
</tr>
<tr>
<td>
<code>processors</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Processor">
[]Processor
</a>
</em>
</td>
<td>
<p>Processors is a list of Processors associated with the server.</p>
</td>
</tr>
<tr>
<td>
<code>storages</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Storage">
[]Storage
</a>
</em>
</td>
<td>
<p>Storages is a list of storages associated with the server.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Conditions represents the latest available observations of the server&rsquo;s current state.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.Storage">Storage
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerStatus">ServerStatus</a>)
</p>
<div>
<p>Storage defines the details of one storage device</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name is the name of the storage interface.</p>
</td>
</tr>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.StorageState">
StorageState
</a>
</em>
</td>
<td>
<p>State specifies the state of the storage device.</p>
</td>
</tr>
<tr>
<td>
<code>volumes</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.StorageVolume">
[]StorageVolume
</a>
</em>
</td>
<td>
<p>Volumes is a collection of volumes associated with this storage.</p>
</td>
</tr>
<tr>
<td>
<code>drives</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.StorageDrive">
[]StorageDrive
</a>
</em>
</td>
<td>
<p>Drives is a collection of drives associated with this storage.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.StorageDrive">StorageDrive
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.Storage">Storage</a>)
</p>
<div>
<p>StorageDrive defines the details of one storage drive</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name is the name of the storage interface.</p>
</td>
</tr>
<tr>
<td>
<code>mediaType</code><br/>
<em>
string
</em>
</td>
<td>
<p>MediaType specifies the media type of the storage device.</p>
</td>
</tr>
<tr>
<td>
<code>type</code><br/>
<em>
string
</em>
</td>
<td>
<p>Type specifies the type of the storage device.</p>
</td>
</tr>
<tr>
<td>
<code>capacity</code><br/>
<em>
<a href="https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity">
k8s.io/apimachinery/pkg/api/resource.Quantity
</a>
</em>
</td>
<td>
<p>Capacity specifies the size of the storage device in bytes.</p>
</td>
</tr>
<tr>
<td>
<code>vendor</code><br/>
<em>
string
</em>
</td>
<td>
<p>Vendor specifies the vendor of the storage device.</p>
</td>
</tr>
<tr>
<td>
<code>model</code><br/>
<em>
string
</em>
</td>
<td>
<p>Model specifies the model of the storage device.</p>
</td>
</tr>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.StorageState">
StorageState
</a>
</em>
</td>
<td>
<p>State specifies the state of the storage device.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.StorageState">StorageState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.Storage">Storage</a>, <a href="#metal.ironcore.dev/v1alpha1.StorageDrive">StorageDrive</a>, <a href="#metal.ironcore.dev/v1alpha1.StorageVolume">StorageVolume</a>)
</p>
<div>
<p>StorageState represents Storage states</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Absent&#34;</p></td>
<td><p>StorageStateAbsent indicates that the storage device is absent.</p>
</td>
</tr><tr><td><p>&#34;Disabled&#34;</p></td>
<td><p>StorageStateDisabled indicates that the storage device is disabled.</p>
</td>
</tr><tr><td><p>&#34;Enabled&#34;</p></td>
<td><p>StorageStateEnabled indicates that the storage device is enabled.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.StorageVolume">StorageVolume
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.Storage">Storage</a>)
</p>
<div>
<p>StorageVolume defines the details of one storage volume</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name is the name of the storage interface.</p>
</td>
</tr>
<tr>
<td>
<code>capacity</code><br/>
<em>
<a href="https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity">
k8s.io/apimachinery/pkg/api/resource.Quantity
</a>
</em>
</td>
<td>
<p>Capacity specifies the size of the storage device in bytes.</p>
</td>
</tr>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.StorageState">
StorageState
</a>
</em>
</td>
<td>
<p>Status specifies the status of the volume.</p>
</td>
</tr>
<tr>
<td>
<code>raidType</code><br/>
<em>
string
</em>
</td>
<td>
<p>RAIDType specifies the RAID type of the associated Volume.</p>
</td>
</tr>
<tr>
<td>
<code>volumeUsage</code><br/>
<em>
string
</em>
</td>
<td>
<p>VolumeUsage specifies the volume usage type for the Volume.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.TaskStatus">TaskStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSVersionStatus">BIOSVersionStatus</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>taskURI</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>taskState</code><br/>
<em>
github.com/stmcginnis/gofish/redfish.TaskState
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>taskStatus</code><br/>
<em>
github.com/stmcginnis/gofish/common.Health
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>percentageComplete</code><br/>
<em>
int
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.UpdatePolicy">UpdatePolicy
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSVersionSpec">BIOSVersionSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Force&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<hr/>
<p><em>
Generated with <code>gen-crd-api-reference-docs</code>
</em></p>
