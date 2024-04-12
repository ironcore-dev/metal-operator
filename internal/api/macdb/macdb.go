/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package macdb

type MacPrefixes struct {
	MacPrefixes []MacPrefix `json:"macPrefixes"`
}

type Console struct {
	Type string `json:"type"`
	Port int32  `json:"port"`
}

type MacPrefix struct {
	MacPrefix          string       `json:"macPrefix"`
	Manufacturer       string       `json:"manufacturer"`
	Protocol           string       `json:"protocol"`
	Port               int32        `json:"port"`
	Type               string       `json:"type"`
	DefaultCredentials []Credential `json:"defaultCredentials"`
	Console            Console      `json:"console,omitempty"`
}

type Credential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
