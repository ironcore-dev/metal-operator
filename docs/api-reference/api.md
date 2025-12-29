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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<code>BIOSSettingsTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsTemplate">
BIOSSettingsTemplate
</a>
</em>
</td>
<td>
<p>
(Members of <code>BIOSSettingsTemplate</code> are embedded into this type.)
</p>
<p>BIOSSettingsTemplate defines the template for BIOS Settings to be applied on the servers.</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to a specific server to apply bios setting on.</p>
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
<h3 id="metal.ironcore.dev/v1alpha1.BIOSSettingsFlowState">BIOSSettingsFlowState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsFlowStatus">BIOSSettingsFlowStatus</a>)
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
<tbody><tr><td><p>&#34;Applied&#34;</p></td>
<td><p>BIOSSettingsFlowStateApplied specifies that the bios setting has been completed for current Priority</p>
</td>
</tr><tr><td><p>&#34;Failed&#34;</p></td>
<td><p>BIOSSettingsFlowStateFailed specifies that the bios setting update has failed.</p>
</td>
</tr><tr><td><p>&#34;InProgress&#34;</p></td>
<td><p>BIOSSettingsFlowStateInProgress specifies that the BIOSSetting Controller is updating the settings for current Priority</p>
</td>
</tr><tr><td><p>&#34;Pending&#34;</p></td>
<td><p>BIOSSettingsFlowStatePending specifies that the BIOSSetting Controller is updating the settings for current Priority</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSSettingsFlowStatus">BIOSSettingsFlowStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsStatus">BIOSSettingsStatus</a>)
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
<code>flowState</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsFlowState">
BIOSSettingsFlowState
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>State represents the current state of the bios configuration task for current priority.</p>
</td>
</tr>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Name identifies current priority settings from the Spec</p>
</td>
</tr>
<tr>
<td>
<code>priority</code><br/>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>Priority identifies the settings priority from the Spec</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Conditions represents the latest available observations of the BIOSSettings&rsquo;s current Flowstate.</p>
</td>
</tr>
<tr>
<td>
<code>lastAppliedTime</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastAppliedTime represents the timestamp when the last setting was successfully applied.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSSettingsSet">BIOSSettingsSet
</h3>
<div>
<p>BIOSSettingsSet is the Schema for the biossettingssets API.</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsSetSpec">
BIOSSettingsSetSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>biosSettingsTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsTemplate">
BIOSSettingsTemplate
</a>
</em>
</td>
<td>
<p>BiosSettingsTemplate defines the template for the BIOSSettings Resource to be applied to the servers.</p>
</td>
</tr>
<tr>
<td>
<code>serverSelector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<p>ServerSelector specifies a label selector to identify the servers that are to be selected.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsSetStatus">
BIOSSettingsSetStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSSettingsSetSpec">BIOSSettingsSetSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsSet">BIOSSettingsSet</a>)
</p>
<div>
<p>BIOSSettingsSetSpec defines the desired state of BIOSSettingsSet.</p>
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
<code>biosSettingsTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsTemplate">
BIOSSettingsTemplate
</a>
</em>
</td>
<td>
<p>BiosSettingsTemplate defines the template for the BIOSSettings Resource to be applied to the servers.</p>
</td>
</tr>
<tr>
<td>
<code>serverSelector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<p>ServerSelector specifies a label selector to identify the servers that are to be selected.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSSettingsSetStatus">BIOSSettingsSetStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsSet">BIOSSettingsSet</a>)
</p>
<div>
<p>BIOSSettingsSetStatus defines the observed state of BIOSSettingsSet.</p>
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
<code>fullyLabeledServers</code><br/>
<em>
int32
</em>
</td>
<td>
<p>FullyLabeledServers is the number of server in the set.</p>
</td>
</tr>
<tr>
<td>
<code>availableBIOSSettings</code><br/>
<em>
int32
</em>
</td>
<td>
<p>AvailableBIOSVersion is the number of Settings current created by the set.</p>
</td>
</tr>
<tr>
<td>
<code>pendingBIOSSettings</code><br/>
<em>
int32
</em>
</td>
<td>
<p>PendingBIOSSettings is the total number of pending server in the set.</p>
</td>
</tr>
<tr>
<td>
<code>inProgressBIOSSettings</code><br/>
<em>
int32
</em>
</td>
<td>
<p>InProgressBIOSSettings is the total number of server in the set that are currently in InProgress.</p>
</td>
</tr>
<tr>
<td>
<code>completedBIOSSettings</code><br/>
<em>
int32
</em>
</td>
<td>
<p>CompletedBIOSSettings is the total number of completed server in the set.</p>
</td>
</tr>
<tr>
<td>
<code>failedBIOSSettings</code><br/>
<em>
int32
</em>
</td>
<td>
<p>FailedBIOSSettings is the total number of failed server in the set.</p>
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
<code>BIOSSettingsTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsTemplate">
BIOSSettingsTemplate
</a>
</em>
</td>
<td>
<p>
(Members of <code>BIOSSettingsTemplate</code> are embedded into this type.)
</p>
<p>BIOSSettingsTemplate defines the template for BIOS Settings to be applied on the servers.</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>ServerRef is a reference to a specific server to apply bios setting on.</p>
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
<p>BIOSSettingsState specifies the current state of the BIOS Settings update.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Applied&#34;</p></td>
<td><p>BIOSSettingsStateApplied specifies that the bios setting update has been completed.</p>
</td>
</tr><tr><td><p>&#34;Failed&#34;</p></td>
<td><p>BIOSSettingsStateFailed specifies that the bios setting update has failed.</p>
</td>
</tr><tr><td><p>&#34;InProgress&#34;</p></td>
<td><p>BIOSSettingsStateInProgress specifies that the BIOSSetting Controller is updating the settings</p>
</td>
</tr><tr><td><p>&#34;Pending&#34;</p></td>
<td><p>BIOSSettingsStatePending specifies that the bios setting update is waiting</p>
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
<em>(Optional)</em>
<p>State represents the current state of the bios configuration task.</p>
</td>
</tr>
<tr>
<td>
<code>flowState</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsFlowStatus">
[]BIOSSettingsFlowStatus
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>lastAppliedTime</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastAppliedTime represents the timestamp when the last setting was successfully applied.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Conditions represents the latest available observations of the BIOSSettings&rsquo;s current state.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSSettingsTemplate">BIOSSettingsTemplate
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsSetSpec">BIOSSettingsSetSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsSpec">BIOSSettingsSpec</a>)
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
<p>Version contains software (eg: BIOS, BMC) version this settings applies to</p>
</td>
</tr>
<tr>
<td>
<code>settingsFlow</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.SettingsFlowItem">
[]SettingsFlowItem
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SettingsFlow contains BIOS settings sequence to apply on the BIOS in given order</p>
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
<em>(Optional)</em>
<p>ServerMaintenancePolicy is a maintenance policy to be enforced on the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerMaintenanceRef is a reference to a ServerMaintenance object that BiosSetting has requested for the referred server.</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<code>BIOSVersionTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSVersionTemplate">
BIOSVersionTemplate
</a>
</em>
</td>
<td>
<p>
(Members of <code>BIOSVersionTemplate</code> are embedded into this type.)
</p>
<p>BIOSVersionTemplate defines the template for Version to be applied on the servers.</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerRef is a reference to a specific server to apply bios upgrade on.</p>
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
<h3 id="metal.ironcore.dev/v1alpha1.BIOSVersionSet">BIOSVersionSet
</h3>
<div>
<p>BIOSVersionSet is the Schema for the biosversionsets API.</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<a href="#metal.ironcore.dev/v1alpha1.BIOSVersionSetSpec">
BIOSVersionSetSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>serverSelector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<p>ServerSelector specifies a label selector to identify the servers that are to be selected.</p>
</td>
</tr>
<tr>
<td>
<code>biosVersionTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSVersionTemplate">
BIOSVersionTemplate
</a>
</em>
</td>
<td>
<p>BIOSVersionTemplate defines the template for the BIOSversion Resource to be applied to the servers.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSVersionSetStatus">
BIOSVersionSetStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSVersionSetSpec">BIOSVersionSetSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSVersionSet">BIOSVersionSet</a>)
</p>
<div>
<p>BIOSVersionSetSpec defines the desired state of BIOSVersionSet.</p>
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
<code>serverSelector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<p>ServerSelector specifies a label selector to identify the servers that are to be selected.</p>
</td>
</tr>
<tr>
<td>
<code>biosVersionTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSVersionTemplate">
BIOSVersionTemplate
</a>
</em>
</td>
<td>
<p>BIOSVersionTemplate defines the template for the BIOSversion Resource to be applied to the servers.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BIOSVersionSetStatus">BIOSVersionSetStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSVersionSet">BIOSVersionSet</a>)
</p>
<div>
<p>BIOSVersionSetStatus defines the observed state of BIOSVersionSet.</p>
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
<code>fullyLabeledServers</code><br/>
<em>
int32
</em>
</td>
<td>
<p>FullyLabeledServers is the number of servers in the set.</p>
</td>
</tr>
<tr>
<td>
<code>availableBIOSVersion</code><br/>
<em>
int32
</em>
</td>
<td>
<p>AvailableBIOSVersion is the number of BIOSVersion created by the set.</p>
</td>
</tr>
<tr>
<td>
<code>pendingBIOSVersion</code><br/>
<em>
int32
</em>
</td>
<td>
<p>PendingBIOSVersion is the total number of pending BIOSVersion in the set.</p>
</td>
</tr>
<tr>
<td>
<code>inProgressBIOSVersion</code><br/>
<em>
int32
</em>
</td>
<td>
<p>InProgressBIOSVersion is the total number of BIOSVersion in the set that are currently in InProgress.</p>
</td>
</tr>
<tr>
<td>
<code>completedBIOSVersion</code><br/>
<em>
int32
</em>
</td>
<td>
<p>CompletedBIOSVersion is the total number of completed BIOSVersion in the set.</p>
</td>
</tr>
<tr>
<td>
<code>failedBIOSVersion</code><br/>
<em>
int32
</em>
</td>
<td>
<p>FailedBIOSVersion is the total number of failed BIOSVersion in the set.</p>
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
<code>BIOSVersionTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSVersionTemplate">
BIOSVersionTemplate
</a>
</em>
</td>
<td>
<p>
(Members of <code>BIOSVersionTemplate</code> are embedded into this type.)
</p>
<p>BIOSVersionTemplate defines the template for Version to be applied on the servers.</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerRef is a reference to a specific server to apply bios upgrade on.</p>
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
<em>(Optional)</em>
<p>State represents the current state of the bios configuration task.</p>
</td>
</tr>
<tr>
<td>
<code>upgradeTask</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Task">
Task
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>UpgradeTask contains the state of the Upgrade Task created by the BMC</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta">
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
<h3 id="metal.ironcore.dev/v1alpha1.BIOSVersionTemplate">BIOSVersionTemplate
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSVersionSetSpec">BIOSVersionSetSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.BIOSVersionSpec">BIOSVersionSpec</a>)
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
<p>Version contains a BIOS version to upgrade to</p>
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
<em>(Optional)</em>
<p>UpdatePolicy An indication of whether the server&rsquo;s upgrade service should bypass vendor update policies</p>
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
<code>serverMaintenancePolicy</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenancePolicy">
ServerMaintenancePolicy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerMaintenancePolicy is a maintenance policy to be enforced on the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerMaintenanceRef is a reference to a ServerMaintenance object that that Controller has requested for the referred server.</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<code>bmcUUID</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>BMCUUID is the unique identifier for the BMC as defined in Redfish API.</p>
</td>
</tr>
<tr>
<td>
<code>endpointRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#secrettype-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<p>Version defines the BMC firmware for which the settings should be applied.</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>BMCRef is a reference to a specific BMC to apply setting to.</p>
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
<em>(Optional)</em>
<p>ServerMaintenancePolicy is a maintenance policy to be applied on the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRefs</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenanceRefItem">
[]ServerMaintenanceRefItem
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerMaintenanceRefs are references to ServerMaintenance objects which are created by the controller for each
server that needs to be updated with the BMC settings.</p>
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
<p>Version defines the BMC firmware for which the settings should be applied.</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>BMCRef is a reference to a specific BMC to apply setting to.</p>
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
<em>(Optional)</em>
<p>ServerMaintenancePolicy is a maintenance policy to be applied on the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRefs</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenanceRefItem">
[]ServerMaintenanceRefItem
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerMaintenanceRefs are references to ServerMaintenance objects which are created by the controller for each
server that needs to be updated with the BMC settings.</p>
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
<p>BMCSettingsState specifies the current state of the server maintenance.</p>
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
<em>(Optional)</em>
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
<code>bmcUUID</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>BMCUUID is the unique identifier for the BMC as defined in Redfish API.</p>
</td>
</tr>
<tr>
<td>
<code>endpointRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
<p>PowerState represents the current power state of the BMC.</p>
</td>
</tr>
<tr>
<td>
<code>lastResetTime</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastResetTime is the timestamp of the last reset operation performed on the BMC.</p>
</td>
</tr>
<tr>
<td>
<code>metricsReportSubscriptionLink</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>MetricsReportSubscriptionLink is the link to the metrics report subscription of the bmc.</p>
</td>
</tr>
<tr>
<td>
<code>eventsSubscriptionLink</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>EventsSubscriptionLink is the link to the events subscription of the bmc.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta">
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
<h3 id="metal.ironcore.dev/v1alpha1.BMCVersion">BMCVersion
</h3>
<div>
<p>BMCVersion is the Schema for the bmcversions API.</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<a href="#metal.ironcore.dev/v1alpha1.BMCVersionSpec">
BMCVersionSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>BMCVersionTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCVersionTemplate">
BMCVersionTemplate
</a>
</em>
</td>
<td>
<p>
(Members of <code>BMCVersionTemplate</code> are embedded into this type.)
</p>
<p>BMCVersionTemplate defines the template for BMC version to be applied on the server&rsquo;s BMC.</p>
</td>
</tr>
<tr>
<td>
<code>bmcRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BMCRef is a reference to a specific BMC to apply BMC upgrade on.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCVersionStatus">
BMCVersionStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCVersionSet">BMCVersionSet
</h3>
<div>
<p>BMCVersionSet is the Schema for the bmcversionsets API.</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<a href="#metal.ironcore.dev/v1alpha1.BMCVersionSetSpec">
BMCVersionSetSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>bmcSelector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<p>BMCSelector specifies a label selector to identify the BMC that are to be selected.</p>
</td>
</tr>
<tr>
<td>
<code>bmcVersionTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCVersionTemplate">
BMCVersionTemplate
</a>
</em>
</td>
<td>
<p>BMCVersionTemplate defines the template for the BMCversion Resource to be applied to the servers.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCVersionSetStatus">
BMCVersionSetStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCVersionSetSpec">BMCVersionSetSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCVersionSet">BMCVersionSet</a>)
</p>
<div>
<p>BMCVersionSetSpec defines the desired state of BMCVersionSet.</p>
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
<code>bmcSelector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<p>BMCSelector specifies a label selector to identify the BMC that are to be selected.</p>
</td>
</tr>
<tr>
<td>
<code>bmcVersionTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCVersionTemplate">
BMCVersionTemplate
</a>
</em>
</td>
<td>
<p>BMCVersionTemplate defines the template for the BMCversion Resource to be applied to the servers.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCVersionSetStatus">BMCVersionSetStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCVersionSet">BMCVersionSet</a>)
</p>
<div>
<p>BMCVersionSetStatus defines the observed state of BMCVersionSet.</p>
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
<code>fullyLabeledBMCs</code><br/>
<em>
int32
</em>
</td>
<td>
<p>FullyLabeledBMCs is the number of server in the set.</p>
</td>
</tr>
<tr>
<td>
<code>availableBMCVersion</code><br/>
<em>
int32
</em>
</td>
<td>
<p>AvailableBMCVersion is the number of BMCVersion current created by the set.</p>
</td>
</tr>
<tr>
<td>
<code>pendingBMCVersion</code><br/>
<em>
int32
</em>
</td>
<td>
<p>PendingBMCVersion is the total number of pending BMCVersion in the set.</p>
</td>
</tr>
<tr>
<td>
<code>inProgressBMCVersion</code><br/>
<em>
int32
</em>
</td>
<td>
<p>InProgressBMCVersion is the total number of BMCVersion in the set that are currently in InProgress.</p>
</td>
</tr>
<tr>
<td>
<code>completedBMCVersion</code><br/>
<em>
int32
</em>
</td>
<td>
<p>CompletedBMCVersion is the total number of completed BMCVersion in the set.</p>
</td>
</tr>
<tr>
<td>
<code>failedBMCVersion</code><br/>
<em>
int32
</em>
</td>
<td>
<p>FailedBMCVersion is the total number of failed BMCVersion in the set.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCVersionSpec">BMCVersionSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCVersion">BMCVersion</a>)
</p>
<div>
<p>BMCVersionSpec defines the desired state of BMCVersion.</p>
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
<code>BMCVersionTemplate</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BMCVersionTemplate">
BMCVersionTemplate
</a>
</em>
</td>
<td>
<p>
(Members of <code>BMCVersionTemplate</code> are embedded into this type.)
</p>
<p>BMCVersionTemplate defines the template for BMC version to be applied on the server&rsquo;s BMC.</p>
</td>
</tr>
<tr>
<td>
<code>bmcRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>BMCRef is a reference to a specific BMC to apply BMC upgrade on.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCVersionState">BMCVersionState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCVersionStatus">BMCVersionStatus</a>)
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
<td><p>BMCVersionStateCompleted specifies that the BMC upgrade maintenance has been completed.</p>
</td>
</tr><tr><td><p>&#34;Failed&#34;</p></td>
<td><p>BMCVersionStateFailed specifies that the BMC upgrade maintenance has failed.</p>
</td>
</tr><tr><td><p>&#34;InProgress&#34;</p></td>
<td><p>BMCVersionStateInProgress specifies that upgrading BMC is in progress.</p>
</td>
</tr><tr><td><p>&#34;Pending&#34;</p></td>
<td><p>BMCVersionStatePending specifies that the BMC upgrade maintenance is waiting</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCVersionStatus">BMCVersionStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCVersion">BMCVersion</a>)
</p>
<div>
<p>BMCVersionStatus defines the observed state of BMCVersion.</p>
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
<a href="#metal.ironcore.dev/v1alpha1.BMCVersionState">
BMCVersionState
</a>
</em>
</td>
<td>
<p>State represents the current state of the BMC configuration task.</p>
</td>
</tr>
<tr>
<td>
<code>upgradeTask</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.Task">
Task
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Conditions represents the latest available observations of the BMC version upgrade state.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCVersionTemplate">BMCVersionTemplate
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCVersionSetSpec">BMCVersionSetSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.BMCVersionSpec">BMCVersionSpec</a>)
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
<p>Version contains a BMC version to upgrade to</p>
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
<em>(Optional)</em>
<p>UpdatePolicy is an indication of whether the server&rsquo;s upgrade service should bypass vendor update policies</p>
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
<p>details regarding the image to use to upgrade to given BMC version</p>
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
<em>(Optional)</em>
<p>ServerMaintenancePolicy is a maintenance policy to be enforced on the server managed by referred BMC.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRefs</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.ServerMaintenanceRefItem">
[]ServerMaintenanceRefItem
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerMaintenanceRefs are references to a ServerMaintenance objects that Controller has requested for the each of the related server.</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSVersionTemplate">BIOSVersionTemplate</a>, <a href="#metal.ironcore.dev/v1alpha1.BMCVersionTemplate">BMCVersionTemplate</a>)
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<h3 id="metal.ironcore.dev/v1alpha1.IndicatorLED">IndicatorLED
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerSpec">ServerSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.ServerStatus">ServerStatus</a>)
</p>
<div>
<p>IndicatorLED represents LED indicator states</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Blinking&#34;</p></td>
<td><p>BlinkingIndicatorLED indicates the Indicator LED is blinking.</p>
</td>
</tr><tr><td><p>&#34;Lit&#34;</p></td>
<td><p>LitIndicatorLED indicates the Indicator LED is lit.</p>
</td>
</tr><tr><td><p>&#34;Off&#34;</p></td>
<td><p>OffIndicatorLED indicates the Indicator LED is off.</p>
</td>
</tr><tr><td><p>&#34;Unknown&#34;</p></td>
<td><p>UnknownIndicatorLED indicates the state of the Indicator LED cannot be
determined.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.InlineEndpoint">InlineEndpoint
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCSpec">BMCSpec</a>)
</p>
<div>
<p>InlineEndpoint defines inline network access configuration for the BMC.</p>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
<p>IP is the IP address of the BMC.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.NetworkInterface">NetworkInterface
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerStatus">ServerStatus</a>)
</p>
<div>
<p>NetworkInterface defines the details of a network interface.</p>
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
<p>Name is the name of the network interface.</p>
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
<p>IP is the IP address assigned to the network interface.
The type is specified as string and is schemaless.</p>
</td>
</tr>
<tr>
<td>
<code>macAddress</code><br/>
<em>
string
</em>
</td>
<td>
<p>MACAddress is the MAC address of the network interface.</p>
</td>
</tr>
</tbody>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<em>(Optional)</em>
<p>SystemUUID is the unique identifier for the server.</p>
</td>
</tr>
<tr>
<td>
<code>systemURI</code><br/>
<em>
string
</em>
</td>
<td>
<p>SystemURI is the unique URI for the server resource in REDFISH API.</p>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
<p>IndicatorLED specifies the desired state of the server&rsquo;s indicator LED.</p>
</td>
</tr>
<tr>
<td>
<code>serverClaimRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerClaimRef is a reference to a ServerClaim object that claims this server.
This field is optional and can be omitted if no claim is associated with this server.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerMaintenanceRef is a reference to a ServerMaintenance object that maintains this server.</p>
</td>
</tr>
<tr>
<td>
<code>bmcRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<em>(Optional)</em>
<p>BMC contains the access details for the BMC.
This field is optional and can be omitted if no BMC access is specified.</p>
</td>
</tr>
<tr>
<td>
<code>bootConfigurationRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>BootConfigurationRef is a reference to a BootConfiguration object that specifies
the boot configuration for this server. This field is optional and can be omitted
if no boot configuration is specified.</p>
</td>
</tr>
<tr>
<td>
<code>maintenanceBootConfigurationRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<em>(Optional)</em>
<p>BootOrder specifies the boot order of the server.</p>
</td>
</tr>
<tr>
<td>
<code>biosSettingsRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
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
<em>(Optional)</em>
<p>Image specifies the boot image to be used for the server.
This field is optional and can be omitted if not specified.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
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
<em>(Optional)</em>
<p>Image specifies the boot image to be used for the server.
This field is optional and can be omitted if not specified.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<em>(Optional)</em>
<p>State represents the current state of the boot configuration.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Conditions represents the latest available observations of the ServerBootConfig&rsquo;s current state.</p>
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
<p>Parameters specify the parameters to be used for rendering the boot configuration.</p>
<br/>
<br/>
<table>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
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
<em>(Optional)</em>
<p>Image specifies the boot image to be used for the server.
This field is optional and can be omitted if not specified.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerRef is a reference to a specific server to be claimed.
This field is optional and can be omitted if the server is to be selected using ServerSelector.</p>
</td>
</tr>
<tr>
<td>
<code>serverSelector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerSelector specifies a label selector to identify the server to be claimed.
This field is optional and can be omitted if a specific server is referenced using ServerRef.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerRef is a reference to a specific server to be claimed.
This field is optional and can be omitted if the server is to be selected using ServerSelector.</p>
</td>
</tr>
<tr>
<td>
<code>serverSelector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerSelector specifies a label selector to identify the server to be claimed.
This field is optional and can be omitted if a specific server is referenced using ServerRef.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta">
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
<em>(Optional)</em>
<p>Policy specifies the maintenance policy to be enforced on the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsTemplate">BIOSSettingsTemplate</a>, <a href="#metal.ironcore.dev/v1alpha1.BIOSVersionTemplate">BIOSVersionTemplate</a>, <a href="#metal.ironcore.dev/v1alpha1.BMCSettingsSpec">BMCSettingsSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.BMCVersionTemplate">BMCVersionTemplate</a>, <a href="#metal.ironcore.dev/v1alpha1.ServerMaintenanceSpec">ServerMaintenanceSpec</a>)
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
<h3 id="metal.ironcore.dev/v1alpha1.ServerMaintenanceRefItem">ServerMaintenanceRefItem
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCSettingsSpec">BMCSettingsSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.BMCVersionTemplate">BMCVersionTemplate</a>)
</p>
<div>
<p>ServerMaintenanceRefItem is a reference to a ServerMaintenance object.</p>
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
<code>serverMaintenanceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerMaintenanceRef is a reference to a ServerMaintenance object that the BMCSettings has requested for the referred server.</p>
</td>
</tr>
</tbody>
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
<em>(Optional)</em>
<p>Policy specifies the maintenance policy to be enforced on the server.</p>
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<td><p>ServerMaintenanceStateFailed specifies that the server maintenance has failed.</p>
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
<em>(Optional)</em>
<p>SystemUUID is the unique identifier for the server.</p>
</td>
</tr>
<tr>
<td>
<code>systemURI</code><br/>
<em>
string
</em>
</td>
<td>
<p>SystemURI is the unique URI for the server resource in REDFISH API.</p>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
<p>IndicatorLED specifies the desired state of the server&rsquo;s indicator LED.</p>
</td>
</tr>
<tr>
<td>
<code>serverClaimRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerClaimRef is a reference to a ServerClaim object that claims this server.
This field is optional and can be omitted if no claim is associated with this server.</p>
</td>
</tr>
<tr>
<td>
<code>serverMaintenanceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServerMaintenanceRef is a reference to a ServerMaintenance object that maintains this server.</p>
</td>
</tr>
<tr>
<td>
<code>bmcRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<em>(Optional)</em>
<p>BMC contains the access details for the BMC.
This field is optional and can be omitted if no BMC access is specified.</p>
</td>
</tr>
<tr>
<td>
<code>bootConfigurationRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>BootConfigurationRef is a reference to a BootConfiguration object that specifies
the boot configuration for this server. This field is optional and can be omitted
if no boot configuration is specified.</p>
</td>
</tr>
<tr>
<td>
<code>maintenanceBootConfigurationRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<em>(Optional)</em>
<p>BootOrder specifies the boot order of the server.</p>
</td>
</tr>
<tr>
<td>
<code>biosSettingsRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
<em>(Optional)</em>
<p>Manufacturer is the name of the server manufacturer.</p>
</td>
</tr>
<tr>
<td>
<code>biosVersion</code><br/>
<em>
string
</em>
</td>
<td>
<p>BIOSVersion is the version of the server&rsquo;s BIOS.</p>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
<p>Storages is a list of storages associated with the server.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta">
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
<h3 id="metal.ironcore.dev/v1alpha1.SettingsFlowItem">SettingsFlowItem
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSSettingsTemplate">BIOSSettingsTemplate</a>)
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
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name identifies what this settings is doing</p>
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
<p>Settings contains software (eg: BIOS, BMC) settings as map</p>
</td>
</tr>
<tr>
<td>
<code>priority</code><br/>
<em>
int32
</em>
</td>
<td>
<p>Priority defines the order of applying the settings
any int greater than 0. lower number have higher Priority (ie; lower number is applied first)</p>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
<p>VolumeUsage specifies the volume usage type for the Volume.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.Task">Task
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSVersionStatus">BIOSVersionStatus</a>, <a href="#metal.ironcore.dev/v1alpha1.BMCVersionStatus">BMCVersionStatus</a>)
</p>
<div>
<p>Task contains the status of the task created by the BMC for the BIOS upgrade.</p>
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
<code>URI</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>URI is the URI of the task created by the BMC for the BIOS upgrade.</p>
</td>
</tr>
<tr>
<td>
<code>state</code><br/>
<em>
github.com/stmcginnis/gofish/redfish.TaskState
</em>
</td>
<td>
<em>(Optional)</em>
<p>State is the current state of the task.</p>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
github.com/stmcginnis/gofish/common.Health
</em>
</td>
<td>
<em>(Optional)</em>
<p>Status is the current status of the task.</p>
</td>
</tr>
<tr>
<td>
<code>percentageComplete</code><br/>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>PercentComplete is the percentage of completion of the task.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.UpdatePolicy">UpdatePolicy
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BIOSVersionTemplate">BIOSVersionTemplate</a>, <a href="#metal.ironcore.dev/v1alpha1.BMCVersionTemplate">BMCVersionTemplate</a>)
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
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BIOSSettingsSpec](#biossettingsspec)_ |  |  |  |
| `status` _[BIOSSettingsStatus](#biossettingsstatus)_ |  |  |  |


#### BIOSSettingsFlowState

_Underlying type:_ _string_





_Appears in:_
- [BIOSSettingsFlowStatus](#biossettingsflowstatus)

| Field | Description |
| --- | --- |
| `Pending` | BIOSSettingsFlowStatePending specifies that the BIOSSetting Controller is updating the settings for current Priority<br /> |
| `InProgress` | BIOSSettingsFlowStateInProgress specifies that the BIOSSetting Controller is updating the settings for current Priority<br /> |
| `Applied` | BIOSSettingsFlowStateApplied specifies that the bios setting has been completed for current Priority<br /> |
| `Failed` | BIOSSettingsFlowStateFailed specifies that the bios setting update has failed.<br /> |


#### BIOSSettingsFlowStatus







_Appears in:_
- [BIOSSettingsStatus](#biossettingsstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `flowState` _[BIOSSettingsFlowState](#biossettingsflowstate)_ | State represents the current state of the bios configuration task for current priority. |  |  |
| `name` _string_ | Name identifies current priority settings from the Spec |  |  |
| `priority` _integer_ | Priority identifies the settings priority from the Spec |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions represents the latest available observations of the BIOSSettings's current Flowstate. |  |  |
| `lastAppliedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastAppliedTime represents the timestamp when the last setting was successfully applied. |  |  |


#### BIOSSettingsSet



BIOSSettingsSet is the Schema for the biossettingssets API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BIOSSettingsSet` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BIOSSettingsSetSpec](#biossettingssetspec)_ |  |  |  |
| `status` _[BIOSSettingsSetStatus](#biossettingssetstatus)_ |  |  |  |


#### BIOSSettingsSetSpec



BIOSSettingsSetSpec defines the desired state of BIOSSettingsSet.



_Appears in:_
- [BIOSSettingsSet](#biossettingsset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `biosSettingsTemplate` _[BIOSSettingsTemplate](#biossettingstemplate)_ | BiosSettingsTemplate defines the template for the BIOSSettings Resource to be applied to the servers. |  |  |
| `serverSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#labelselector-v1-meta)_ | ServerSelector specifies a label selector to identify the servers that are to be selected. |  |  |


#### BIOSSettingsSetStatus



BIOSSettingsSetStatus defines the observed state of BIOSSettingsSet.



_Appears in:_
- [BIOSSettingsSet](#biossettingsset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `fullyLabeledServers` _integer_ | FullyLabeledServers is the number of server in the set. |  |  |
| `availableBIOSSettings` _integer_ | AvailableBIOSVersion is the number of Settings current created by the set. |  |  |
| `pendingBIOSSettings` _integer_ | PendingBIOSSettings is the total number of pending server in the set. |  |  |
| `inProgressBIOSSettings` _integer_ | InProgressBIOSSettings is the total number of server in the set that are currently in InProgress. |  |  |
| `completedBIOSSettings` _integer_ | CompletedBIOSSettings is the total number of completed server in the set. |  |  |
| `failedBIOSSettings` _integer_ | FailedBIOSSettings is the total number of failed server in the set. |  |  |


#### BIOSSettingsSpec



BIOSSettingsSpec defines the desired state of BIOSSettings.



_Appears in:_
- [BIOSSettings](#biossettings)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version contains software (eg: BIOS, BMC) version this settings applies to |  |  |
| `settingsFlow` _[SettingsFlowItem](#settingsflowitem) array_ | SettingsFlow contains BIOS settings sequence to apply on the BIOS in given order |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be enforced on the server. |  |  |
| `serverMaintenanceRef` _[ObjectReference](#objectreference)_ | ServerMaintenanceRef is a reference to a ServerMaintenance object that BiosSetting has requested for the referred server. |  |  |
| `serverRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | ServerRef is a reference to a specific server to apply bios setting on. |  |  |


#### BIOSSettingsState

_Underlying type:_ _string_

BIOSSettingsState specifies the current state of the BIOS Settings update.



_Appears in:_
- [BIOSSettingsStatus](#biossettingsstatus)

| Field | Description |
| --- | --- |
| `Pending` | BIOSSettingsStatePending specifies that the bios setting update is waiting<br /> |
| `InProgress` | BIOSSettingsStateInProgress specifies that the BIOSSetting Controller is updating the settings<br /> |
| `Applied` | BIOSSettingsStateApplied specifies that the bios setting update has been completed.<br /> |
| `Failed` | BIOSSettingsStateFailed specifies that the bios setting update has failed.<br /> |


#### BIOSSettingsStatus



BIOSSettingsStatus defines the observed state of BIOSSettings.



_Appears in:_
- [BIOSSettings](#biossettings)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `state` _[BIOSSettingsState](#biossettingsstate)_ | State represents the current state of the bios configuration task. |  |  |
| `flowState` _[BIOSSettingsFlowStatus](#biossettingsflowstatus) array_ | FlowState is a list of individual BIOSSettings operation flows. |  |  |
| `lastAppliedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastAppliedTime represents the timestamp when the last setting was successfully applied. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions represents the latest available observations of the BIOSSettings's current state. |  |  |


#### BIOSSettingsTemplate







_Appears in:_
- [BIOSSettingsSetSpec](#biossettingssetspec)
- [BIOSSettingsSpec](#biossettingsspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version contains software (eg: BIOS, BMC) version this settings applies to |  |  |
| `settingsFlow` _[SettingsFlowItem](#settingsflowitem) array_ | SettingsFlow contains BIOS settings sequence to apply on the BIOS in given order |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be enforced on the server. |  |  |


#### BIOSVersion



BIOSVersion is the Schema for the biosversions API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BIOSVersion` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BIOSVersionSpec](#biosversionspec)_ |  |  |  |
| `status` _[BIOSVersionStatus](#biosversionstatus)_ |  |  |  |


#### BIOSVersionSet



BIOSVersionSet is the Schema for the biosversionsets API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BIOSVersionSet` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BIOSVersionSetSpec](#biosversionsetspec)_ |  |  |  |
| `status` _[BIOSVersionSetStatus](#biosversionsetstatus)_ |  |  |  |


#### BIOSVersionSetSpec



BIOSVersionSetSpec defines the desired state of BIOSVersionSet.



_Appears in:_
- [BIOSVersionSet](#biosversionset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serverSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#labelselector-v1-meta)_ | ServerSelector specifies a label selector to identify the servers that are to be selected. |  |  |
| `biosVersionTemplate` _[BIOSVersionTemplate](#biosversiontemplate)_ | BIOSVersionTemplate defines the template for the BIOSversion Resource to be applied to the servers. |  |  |


#### BIOSVersionSetStatus



BIOSVersionSetStatus defines the observed state of BIOSVersionSet.



_Appears in:_
- [BIOSVersionSet](#biosversionset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `fullyLabeledServers` _integer_ | FullyLabeledServers is the number of servers in the set. |  |  |
| `availableBIOSVersion` _integer_ | AvailableBIOSVersion is the number of BIOSVersion created by the set. |  |  |
| `pendingBIOSVersion` _integer_ | PendingBIOSVersion is the total number of pending BIOSVersion in the set. |  |  |
| `inProgressBIOSVersion` _integer_ | InProgressBIOSVersion is the total number of BIOSVersion in the set that are currently in InProgress. |  |  |
| `completedBIOSVersion` _integer_ | CompletedBIOSVersion is the total number of completed BIOSVersion in the set. |  |  |
| `failedBIOSVersion` _integer_ | FailedBIOSVersion is the total number of failed BIOSVersion in the set. |  |  |


#### BIOSVersionSpec



BIOSVersionSpec defines the desired state of BIOSVersion.



_Appears in:_
- [BIOSVersion](#biosversion)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version contains a BIOS version to upgrade to |  |  |
| `updatePolicy` _[UpdatePolicy](#updatepolicy)_ | UpdatePolicy An indication of whether the server's upgrade service should bypass vendor update policies |  |  |
| `image` _[ImageSpec](#imagespec)_ | details regarding the image to use to upgrade to given BIOS version |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be enforced on the server. |  |  |
| `serverMaintenanceRef` _[ObjectReference](#objectreference)_ | ServerMaintenanceRef is a reference to a ServerMaintenance object that that Controller has requested for the referred server. |  |  |
| `serverRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | ServerRef is a reference to a specific server to apply bios upgrade on. |  |  |


#### BIOSVersionState

_Underlying type:_ _string_





_Appears in:_
- [BIOSVersionStatus](#biosversionstatus)

| Field | Description |
| --- | --- |
| `Pending` | BIOSVersionStatePending specifies that the bios upgrade maintenance is waiting<br /> |
| `Processing` | BIOSVersionStateInProgress specifies that upgrading bios is in progress.<br /> |
| `Completed` | BIOSVersionStateCompleted specifies that the bios upgrade maintenance has been completed.<br /> |
| `Failed` | BIOSVersionStateFailed specifies that the bios upgrade maintenance has failed.<br /> |


#### BIOSVersionStatus



BIOSVersionStatus defines the observed state of BIOSVersion.



_Appears in:_
- [BIOSVersion](#biosversion)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `state` _[BIOSVersionState](#biosversionstate)_ | State represents the current state of the bios configuration task. |  |  |
| `upgradeTask` _[Task](#task)_ | UpgradeTask contains the state of the Upgrade Task created by the BMC |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions represents the latest available observations of the Bios version upgrade state. |  |  |


#### BIOSVersionTemplate







_Appears in:_
- [BIOSVersionSetSpec](#biosversionsetspec)
- [BIOSVersionSpec](#biosversionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version contains a BIOS version to upgrade to |  |  |
| `updatePolicy` _[UpdatePolicy](#updatepolicy)_ | UpdatePolicy An indication of whether the server's upgrade service should bypass vendor update policies |  |  |
| `image` _[ImageSpec](#imagespec)_ | details regarding the image to use to upgrade to given BIOS version |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be enforced on the server. |  |  |


#### BMC



BMC is the Schema for the bmcs API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BMC` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
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
| `bmcSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | BMCSecretRef is a reference to the Kubernetes Secret object that contains the credentials<br />required to access the BMC. This secret includes sensitive information such as usernames and passwords. |  |  |


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


#### BMCSecret



BMCSecret is the Schema for the bmcsecrets API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BMCSecret` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `immutable` _boolean_ | Immutable, if set to true, ensures that data stored in the Secret cannot<br />be updated (only object metadata can be modified).<br />If not set to true, the field can be modified at any time.<br />Defaulted to nil. |  |  |
| `data` _object (keys:string, values:integer array)_ | Data contains the secret data. Each key must consist of alphanumeric<br />characters, '-', '_' or '.'. The serialized form of the secret data is a<br />base64 encoded string, representing the arbitrary (possibly non-string)<br />data value here. Described in https://tools.ietf.org/html/rfc4648#section-4 |  |  |
| `stringData` _object (keys:string, values:string)_ | stringData allows specifying non-binary secret data in string form.<br />It is provided as a write-only input field for convenience.<br />All keys and values are merged into the data field on write, overwriting any existing values.<br />The stringData field is never output when reading from the API. |  |  |
| `type` _[SecretType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#secrettype-v1-core)_ | Used to facilitate programmatic handling of secret data.<br />More info: https://kubernetes.io/docs/concepts/configuration/secret/#secret-types |  |  |


#### BMCSettings



BMCSettings is the Schema for the BMCSettings API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BMCSettings` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BMCSettingsSpec](#bmcsettingsspec)_ |  |  |  |
| `status` _[BMCSettingsStatus](#bmcsettingsstatus)_ |  |  |  |


#### BMCSettingsSpec



BMCSettingsSpec defines the desired state of BMCSettings.



_Appears in:_
- [BMCSettings](#bmcsettings)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version defines the BMC firmware for which the settings should be applied. |  |  |
| `settings` _object (keys:string, values:string)_ | SettingsMap contains bmc settings as map |  |  |
| `BMCRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | BMCRef is a reference to a specific BMC to apply setting to. |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be applied on the server. |  |  |
| `serverMaintenanceRefs` _[ServerMaintenanceRefItem](#servermaintenancerefitem) array_ | ServerMaintenanceRefs are references to ServerMaintenance objects which are created by the controller for each<br />server that needs to be updated with the BMC settings. |  |  |


#### BMCSettingsState

_Underlying type:_ _string_

BMCSettingsState specifies the current state of the server maintenance.



_Appears in:_
- [BMCSettingsStatus](#bmcsettingsstatus)

| Field | Description |
| --- | --- |
| `Pending` | BMCSettingsStatePending specifies that the BMC maintenance is waiting<br /> |
| `InProgress` | BMCSettingsStateInProgress specifies that the BMC setting changes are in progress<br /> |
| `Applied` | BMCSettingsStateApplied specifies that the BMC maintenance has been completed.<br /> |
| `Failed` | BMCSettingsStateFailed specifies that the BMC maintenance has failed.<br /> |


#### BMCSettingsStatus



BMCSettingsStatus defines the observed state of BMCSettings.



_Appears in:_
- [BMCSettings](#bmcsettings)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `state` _[BMCSettingsState](#bmcsettingsstate)_ | State represents the current state of the BMC configuration task. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions represents the latest available observations of the BMC Settings Resource state. |  |  |


#### BMCSpec



BMCSpec defines the desired state of BMC



_Appears in:_
- [BMC](#bmc)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bmcUUID` _string_ | BMCUUID is the unique identifier for the BMC as defined in Redfish API. |  | Optional: \{\} <br /> |
| `endpointRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | EndpointRef is a reference to the Kubernetes object that contains the endpoint information for the BMC.<br />This reference is typically used to locate the BMC endpoint within the cluster. |  | Optional: \{\} <br /> |
| `access` _[InlineEndpoint](#inlineendpoint)_ | Endpoint allows inline configuration of network access details for the BMC.<br />Use this field if access settings like address are to be configured directly within the BMC resource. |  | Optional: \{\} <br /> |
| `bmcSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | BMCSecretRef is a reference to the Kubernetes Secret object that contains the credentials<br />required to access the BMC. This secret includes sensitive information such as usernames and passwords. |  |  |
| `protocol` _[Protocol](#protocol)_ | Protocol specifies the protocol to be used for communicating with the BMC.<br />It could be a standard protocol such as IPMI or Redfish. |  |  |
| `consoleProtocol` _[ConsoleProtocol](#consoleprotocol)_ | ConsoleProtocol specifies the protocol to be used for console access to the BMC.<br />This field is optional and can be omitted if console access is not required. |  |  |
| `bmcSettingsRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | BMCSettingRef is a reference to a BMCSettings object that specifies<br />the BMC configuration for this BMC. |  |  |


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
| `macAddress` _string_ | MACAddress is the MAC address of the BMC.<br />The format is validated using a regular expression pattern. |  | Pattern: `^([0-9A-Fa-f]\{2\}[:-])\{5\}([0-9A-Fa-f]\{2\})$` <br /> |
| `ip` _[IP](#ip)_ | IP is the IP address of the BMC.<br />The type is specified as string and is schemaless. |  | Format: ip <br />Schemaless: \{\} <br />Type: string <br /> |
| `manufacturer` _string_ | Manufacturer is the name of the BMC manufacturer. |  |  |
| `model` _string_ | Model is the model number or name of the BMC. |  |  |
| `sku` _string_ | SKU is the stock keeping unit identifier for the BMC. |  |  |
| `serialNumber` _string_ | SerialNumber is the serial number of the BMC. |  |  |
| `firmwareVersion` _string_ | FirmwareVersion is the version of the firmware currently running on the BMC. |  |  |
| `state` _[BMCState](#bmcstate)_ | State represents the current state of the BMC.<br />kubebuilder:validation:Enum=Enabled;Error;Pending | Pending |  |
| `powerState` _[BMCPowerState](#bmcpowerstate)_ | PowerState represents the current power state of the BMC. |  |  |
| `lastResetTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastResetTime is the timestamp of the last reset operation performed on the BMC. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions represents the latest available observations of the BMC's current state. |  |  |


#### BMCVersion



BMCVersion is the Schema for the bmcversions API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BMCVersion` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BMCVersionSpec](#bmcversionspec)_ |  |  |  |
| `status` _[BMCVersionStatus](#bmcversionstatus)_ |  |  |  |


#### BMCVersionSet



BMCVersionSet is the Schema for the bmcversionsets API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `BMCVersionSet` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[BMCVersionSetSpec](#bmcversionsetspec)_ |  |  |  |
| `status` _[BMCVersionSetStatus](#bmcversionsetstatus)_ |  |  |  |


#### BMCVersionSetSpec



BMCVersionSetSpec defines the desired state of BMCVersionSet.



_Appears in:_
- [BMCVersionSet](#bmcversionset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bmcSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#labelselector-v1-meta)_ | BMCSelector specifies a label selector to identify the BMC that are to be selected. |  |  |
| `bmcVersionTemplate` _[BMCVersionTemplate](#bmcversiontemplate)_ | BMCVersionTemplate defines the template for the BMCversion Resource to be applied to the servers. |  |  |


#### BMCVersionSetStatus



BMCVersionSetStatus defines the observed state of BMCVersionSet.



_Appears in:_
- [BMCVersionSet](#bmcversionset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `fullyLabeledBMCs` _integer_ | FullyLabeledBMCs is the number of server in the set. |  |  |
| `availableBMCVersion` _integer_ | AvailableBMCVersion is the number of BMCVersion current created by the set. |  |  |
| `pendingBMCVersion` _integer_ | PendingBMCVersion is the total number of pending BMCVersion in the set. |  |  |
| `inProgressBMCVersion` _integer_ | InProgressBMCVersion is the total number of BMCVersion in the set that are currently in InProgress. |  |  |
| `completedBMCVersion` _integer_ | CompletedBMCVersion is the total number of completed BMCVersion in the set. |  |  |
| `failedBMCVersion` _integer_ | FailedBMCVersion is the total number of failed BMCVersion in the set. |  |  |


#### BMCVersionSpec



BMCVersionSpec defines the desired state of BMCVersion.



_Appears in:_
- [BMCVersion](#bmcversion)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version contains a BMC version to upgrade to |  |  |
| `updatePolicy` _[UpdatePolicy](#updatepolicy)_ | UpdatePolicy is an indication of whether the server's upgrade service should bypass vendor update policies |  |  |
| `image` _[ImageSpec](#imagespec)_ | details regarding the image to use to upgrade to given BMC version |  |  |
| `serverMaintenancePolicy` _[ServerMaintenancePolicy](#servermaintenancepolicy)_ | ServerMaintenancePolicy is a maintenance policy to be enforced on the server managed by referred BMC. |  |  |
| `serverMaintenanceRefs` _[ObjectReference](#objectreference) array_ | ServerMaintenanceRefs are references to a ServerMaintenance objects that Controller has requested for the each of the related server. |  |  |
| `bmcRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | BMCRef is a reference to a specific BMC to apply BMC upgrade on. |  |  |


#### BMCVersionState

_Underlying type:_ _string_





_Appears in:_
- [BMCVersionStatus](#bmcversionstatus)

| Field | Description |
| --- | --- |
| `Pending` | BMCVersionStatePending specifies that the BMC upgrade maintenance is waiting<br /> |
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
| `upgradeTask` _[Task](#task)_ | UpgradeTask contains the state of the Upgrade Task created by the BMC |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions represents the latest available observations of the BMC version upgrade state. |  |  |


#### BMCVersionTemplate







_Appears in:_
- [BMCVersionSetSpec](#bmcversionsetspec)
- [BMCVersionSpec](#bmcversionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Version contains a BMC version to upgrade to |  |  |
| `updatePolicy` _[UpdatePolicy](#updatepolicy)_ | UpdatePolicy is an indication of whether the server's upgrade service should bypass vendor update policies |  |  |
| `image` _[ImageSpec](#imagespec)_ | details regarding the image to use to upgrade to given BMC version |  |  |
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
| `name` _[ConsoleProtocolName](#consoleprotocolname)_ | Name specifies the name of the console protocol.<br />This could be a protocol such as "SSH", "Telnet", etc. |  | Enum: [IPMI SSH SSHLenovo] <br /> |
| `port` _integer_ | Port specifies the port number used for console access.<br />This port is used by the specified console protocol to establish connections. |  |  |


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
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
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
| `secretRef` _[SecretReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#secretreference-v1-core)_ | ImageSecretRef is a reference to the Kubernetes Secret (of type SecretTypeBasicAuth) object that contains the credentials<br />to access the ImageURI. This secret includes sensitive information such as usernames and passwords. |  |  |
| `transferProtocol` _string_ | The network protocol that the server's update service uses to retrieve 'ImageURI' |  |  |
| `URI` _string_ | The URI of the software image to update/install." |  |  |


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
| `uid` _[UID](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#uid-types-pkg)_ | UID is the uid of the referenced object. |  |  |


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
| `name` _[ProtocolName](#protocolname)_ | Name specifies the name of the protocol.<br />This could be a protocol such as "IPMI", "Redfish", etc. |  |  |
| `port` _integer_ | Port specifies the port number used for communication.<br />This port is used by the specified protocol to establish connections. |  |  |
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
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ServerSpec](#serverspec)_ |  |  |  |
| `status` _[ServerStatus](#serverstatus)_ |  |  |  |


#### ServerBootConfiguration



ServerBootConfiguration is the Schema for the serverbootconfigurations API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `ServerBootConfiguration` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ServerBootConfigurationSpec](#serverbootconfigurationspec)_ |  |  |  |
| `status` _[ServerBootConfigurationStatus](#serverbootconfigurationstatus)_ |  |  |  |


#### ServerBootConfigurationSpec



ServerBootConfigurationSpec defines the desired state of ServerBootConfiguration.



_Appears in:_
- [ServerBootConfiguration](#serverbootconfiguration)
- [ServerBootConfigurationTemplate](#serverbootconfigurationtemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serverRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | ServerRef is a reference to the server for which this boot configuration is intended. |  |  |
| `image` _string_ | Image specifies the boot image to be used for the server.<br />This field is optional and can be omitted if not specified. |  |  |
| `ignitionSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | IgnitionSecretRef is a reference to the Kubernetes Secret object that contains<br />the ignition configuration for the server. This field is optional and can be omitted if not specified. |  |  |


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
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions represents the latest available observations of the ServerBootConfig's current state. |  |  |


#### ServerBootConfigurationTemplate



ServerBootConfigurationTemplate defines the parameters to be used for rendering a boot configuration.



_Appears in:_
- [ServerMaintenanceSpec](#servermaintenancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name specifies the name of the boot configuration. |  |  |
| `spec` _[ServerBootConfigurationSpec](#serverbootconfigurationspec)_ | Parameters specify the parameters to be used for rendering the boot configuration. |  |  |


#### ServerClaim



ServerClaim is the Schema for the serverclaims API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `metal.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `ServerClaim` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ServerClaimSpec](#serverclaimspec)_ |  |  |  |
| `status` _[ServerClaimStatus](#serverclaimstatus)_ |  |  |  |


#### ServerClaimSpec



ServerClaimSpec defines the desired state of ServerClaim.



_Appears in:_
- [ServerClaim](#serverclaim)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `power` _[Power](#power)_ | Power specifies the desired power state of the server. |  |  |
| `serverRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | ServerRef is a reference to a specific server to be claimed.<br />This field is optional and can be omitted if the server is to be selected using ServerSelector. |  | Optional: \{\} <br /> |
| `serverSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#labelselector-v1-meta)_ | ServerSelector specifies a label selector to identify the server to be claimed.<br />This field is optional and can be omitted if a specific server is referenced using ServerRef. |  | Optional: \{\} <br /> |
| `ignitionSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | IgnitionSecretRef is a reference to the Kubernetes Secret object that contains<br />the ignition configuration for the server. This field is optional and can be omitted if not specified. |  |  |
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
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
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
| `serverRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | ServerRef is a reference to the server that is to be maintained. |  |  |
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
| `serverClaimRef` _[ObjectReference](#objectreference)_ | ServerClaimRef is a reference to a ServerClaim object that claims this server.<br />This field is optional and can be omitted if no claim is associated with this server. |  | Optional: \{\} <br /> |
| `serverMaintenanceRef` _[ObjectReference](#objectreference)_ | ServerMaintenanceRef is a reference to a ServerMaintenance object that maintains this server. |  |  |
| `bmcRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | BMCRef is a reference to the BMC object associated with this server.<br />This field is optional and can be omitted if no BMC is associated with this server. |  |  |
| `bmc` _[BMCAccess](#bmcaccess)_ | BMC contains the access details for the BMC.<br />This field is optional and can be omitted if no BMC access is specified. |  |  |
| `bootConfigurationRef` _[ObjectReference](#objectreference)_ | BootConfigurationRef is a reference to a BootConfiguration object that specifies<br />the boot configuration for this server. This field is optional and can be omitted<br />if no boot configuration is specified. |  |  |
| `maintenanceBootConfigurationRef` _[ObjectReference](#objectreference)_ | MaintenanceBootConfigurationRef is a reference to a BootConfiguration object that specifies<br />the boot configuration for this server during maintenance. This field is optional and can be omitted |  |  |
| `bootOrder` _[BootOrder](#bootorder) array_ | BootOrder specifies the boot order of the server. |  |  |
| `biosSettingsRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | BIOSSettingsRef is a reference to a biossettings object that specifies<br />the BIOS configuration for this server. |  |  |


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
| `totalSystemMemory` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#quantity-resource-api)_ | TotalSystemMemory is the total amount of memory in bytes available on the server. |  |  |
| `processors` _[Processor](#processor) array_ | Processors is a list of Processors associated with the server. |  |  |
| `storages` _[Storage](#storage) array_ | Storages is a list of storages associated with the server. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions represents the latest available observations of the server's current state. |  |  |


#### SettingsFlowItem







_Appears in:_
- [BIOSSettingsSpec](#biossettingsspec)
- [BIOSSettingsTemplate](#biossettingstemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the flow item |  | MaxLength: 1000 <br />MinLength: 1 <br /> |
| `settings` _object (keys:string, values:string)_ | Settings contains software (eg: BIOS, BMC) settings as map |  |  |
| `priority` _integer_ | Priority defines the order of applying the settings<br />any int greater than 0. lower number have higher Priority (ie; lower number is applied first) |  | Maximum: 2.147483645e+09 <br />Minimum: 1 <br /> |


#### Storage



Storage defines the details of one storage device



_Appears in:_
- [ServerStatus](#serverstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the storage interface. |  |  |
| `state` _[StorageState](#storagestate)_ | State specifies the state of the storage device. |  |  |
| `volumes` _[StorageVolume](#storagevolume) array_ | Volumes is a collection of volumes associated with this storage. |  |  |
| `drives` _[StorageDrive](#storagedrive) array_ | Drives is a collection of drives associated with this storage. |  |  |


#### StorageDrive



StorageDrive defines the details of one storage drive



_Appears in:_
- [Storage](#storage)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the storage interface. |  |  |
| `mediaType` _string_ | MediaType specifies the media type of the storage device. |  |  |
| `type` _string_ | Type specifies the type of the storage device. |  |  |
| `capacity` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#quantity-resource-api)_ | Capacity specifies the size of the storage device in bytes. |  |  |
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



StorageVolume defines the details of one storage volume



_Appears in:_
- [Storage](#storage)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the storage interface. |  |  |
| `capacity` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#quantity-resource-api)_ | Capacity specifies the size of the storage device in bytes. |  |  |
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


