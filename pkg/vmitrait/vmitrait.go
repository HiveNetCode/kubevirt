/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright The KubeVirt Authors.
 *
 */

package vmitrait

import (
	v1 "kubevirt.io/api/core/v1"
)

func IsNonRoot(vmi *v1.VirtualMachineInstance) bool {
	_, ok := vmi.Annotations[v1.DeprecatedNonRootVMIAnnotation]
	nonRoot := vmi.Status.RuntimeUser != 0
	return ok || nonRoot
}

// UsesPasstBinding reports whether the VMI requests the passt backend for any
// of its interfaces, either via the core passtBinding field or the passt
// binding plugin. The passt backend sandboxes itself (remount / read-only,
// drop capabilities and supplementary groups, drop to "nobody"), which
// needs CAP_SYS_ADMIN, CAP_SETUID and CAP_SETGID in the process's effective
// set. The launcher runs as non-root, where Kubernetes only places requested
// capabilities into the bounding set, so the image grants the capabilities
// to the passt binary via file capabilities (see the launcherbase targets in
// rpm/BUILD.bazel); the bounding set must contain them for the grant to
// take effect.
func UsesPasstBinding(vmi *v1.VirtualMachineInstance) bool {
	for _, iface := range vmi.Spec.Domain.Devices.Interfaces {
		if iface.PasstBinding != nil {
			return true
		}
		// The passt binding plugin drives the same libvirt passt backend.
		if iface.Binding != nil && iface.Binding.Name == "passt" {
			return true
		}
	}
	return false
}

// HasVFIO reports whether the VMI requests any VFIO device.
func HasVFIO(vmi *v1.VirtualMachineInstance) bool {
	return hasHostDev(vmi) || hasGPU(vmi) || hasSRIOV(vmi)
}

func hasHostDev(vmi *v1.VirtualMachineInstance) bool {
	return len(vmi.Spec.Domain.Devices.HostDevices) > 0
}

func hasGPU(vmi *v1.VirtualMachineInstance) bool {
	return len(vmi.Spec.Domain.Devices.GPUs) > 0
}

func hasSRIOV(vmi *v1.VirtualMachineInstance) bool {
	for _, iface := range vmi.Spec.Domain.Devices.Interfaces {
		if iface.SRIOV != nil {
			return true
		}
	}
	return false
}
