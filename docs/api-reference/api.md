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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta">
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
<code>endpointRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>bmcSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
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
</td>
</tr>
<tr>
<td>
<code>endpoint</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>bmcSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#secrettype-v1-core">
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
<code>endpointRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>bmcSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
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
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Enabled&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;Error&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.BMCStatus">BMCStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMC">BMC</a>)
</p>
<div>
<p>BMCStatus defines the observed state of BMC</p>
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
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
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
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;IPMI&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;SSH&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;SSHLenovo&#34;</p></td>
<td></td>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta">
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
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCStatus">BMCStatus</a>, <a href="#metal.ironcore.dev/v1alpha1.EndpointSpec">EndpointSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.NetworkInterface">NetworkInterface</a>)
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
<h3 id="metal.ironcore.dev/v1alpha1.NetworkInterface">NetworkInterface
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerStatus">ServerStatus</a>)
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
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Bound&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;Unbound&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.Power">Power
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerClaimSpec">ServerClaimSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.ServerSpec">ServerSpec</a>)
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
<tbody><tr><td><p>&#34;Off&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;On&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.Protocol">Protocol
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.BMCAccess">BMCAccess</a>, <a href="#metal.ironcore.dev/v1alpha1.BMCSpec">BMCSpec</a>)
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
<a href="#metal.ironcore.dev/v1alpha1.ProtocolName">
ProtocolName
</a>
</em>
</td>
<td>
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
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;IPMI&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;Redfish&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;SSH&#34;</p></td>
<td></td>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta">
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
</td>
</tr>
<tr>
<td>
<code>serverClaimRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>bmcRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
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
</td>
</tr>
<tr>
<td>
<code>bootConfigurationRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
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
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
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
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerBootConfiguration">ServerBootConfiguration</a>)
</p>
<div>
<p>ServerBootConfigurationSpec defines the desired state of ServerBootConfiguration</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
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
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
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
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Error&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;Pending&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;Ready&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerBootConfigurationStatus">ServerBootConfigurationStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerBootConfiguration">ServerBootConfiguration</a>)
</p>
<div>
<p>ServerBootConfigurationStatus defines the observed state of ServerBootConfiguration</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta">
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
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>serverSelector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
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
<p>ServerClaimSpec defines the desired state of ServerClaim</p>
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
</td>
</tr>
<tr>
<td>
<code>serverRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>serverSelector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
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
<p>ServerClaimStatus defines the observed state of ServerClaim</p>
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
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Off&#34;</p></td>
<td><p>ServerOffPowerState the system is powered off, although some components may
continue to have AUX power such as management controller.</p>
</td>
</tr><tr><td><p>&#34;On&#34;</p></td>
<td><p>ServerOnPowerState the system is powered on.</p>
</td>
</tr><tr><td><p>&#34;Paused&#34;</p></td>
<td><p>ServerPausedPowerState the system is paused.</p>
</td>
</tr><tr><td><p>&#34;PoweringOff&#34;</p></td>
<td><p>ServerPoweringOffPowerState A temporary state between On and Off. The power
off action can take time while the OS is in the shutdown process.</p>
</td>
</tr><tr><td><p>&#34;PoweringOn&#34;</p></td>
<td><p>ServerPoweringOnPowerState A temporary state between Off and On. This
temporary state can be very short.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerSpec">ServerSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.Server">Server</a>)
</p>
<div>
<p>ServerSpec defines the desired state of Server</p>
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
</td>
</tr>
<tr>
<td>
<code>serverClaimRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>bmcRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
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
</td>
</tr>
<tr>
<td>
<code>bootConfigurationRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectreference-v1-core">
Kubernetes core/v1.ObjectReference
</a>
</em>
</td>
<td>
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
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Available&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;Error&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;Initial&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;Reserved&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.ServerStatus">ServerStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.Server">Server</a>)
</p>
<div>
<p>ServerStatus defines the observed state of Server</p>
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
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<hr/>
<p><em>
Generated with <code>gen-crd-api-reference-docs</code>
</em></p>
