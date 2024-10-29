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
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerSpec">ServerSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.ServerStatus">ServerStatus</a>)
</p>
<div>
<p>BIOSSettings represents the BIOS settings for a server.</p>
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
<p>Version specifies the version of the server BIOS for which the settings are defined.</p>
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
<p>Settings is a map of key-value pairs representing the BIOS settings.</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>EndpointRef is a reference to the Kubernetes object that contains the endpoint information for the BMC.
This reference is typically used to locate the BMC endpoint within the cluster.</p>
</td>
</tr>
<tr>
<td>
<code>bmcSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
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
<code>endpoint</code><br/>
<em>
string
</em>
</td>
<td>
<p>Endpoint is the address of the BMC endpoint.</p>
</td>
</tr>
<tr>
<td>
<code>bmcSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#secrettype-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>EndpointRef is a reference to the Kubernetes object that contains the endpoint information for the BMC.
This reference is typically used to locate the BMC endpoint within the cluster.</p>
</td>
</tr>
<tr>
<td>
<code>bmcSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta">
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
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerClaimSpec">ServerClaimSpec</a>, <a href="#metal.ironcore.dev/v1alpha1.ServerSpec">ServerSpec</a>)
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta">
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
<p>UUID is the unique identifier for the server.</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectreference-v1-core">
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
<code>bmcRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectreference-v1-core">
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
<code>BIOS</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettings">
[]BIOSSettings
</a>
</em>
</td>
<td>
<p>BIOS specifies the BIOS settings for the server.</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
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
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.ServerBootConfiguration">ServerBootConfiguration</a>)
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#labelselector-v1-meta">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#labelselector-v1-meta">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
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
<p>UUID is the unique identifier for the server.</p>
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectreference-v1-core">
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
<code>bmcRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectreference-v1-core">
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
<code>BIOS</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettings">
[]BIOSSettings
</a>
</em>
</td>
<td>
<p>BIOS specifies the BIOS settings for the server.</p>
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
<code>BIOS</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.BIOSSettings">
BIOSSettings
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta">
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
<code>rotational</code><br/>
<em>
bool
</em>
</td>
<td>
<p>Rotational specifies whether the storage device is rotational.</p>
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
<p>SizeBytes specifies the size of the storage device in bytes.</p>
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
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.Storage">Storage</a>)
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
<hr/>
<p><em>
Generated with <code>gen-crd-api-reference-docs</code>
</em></p>
