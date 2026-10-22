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

package compute

import (
	"strings"

	v1 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/api"
)

// viommuGPUSubstring selects the passthrough GPU device names that require a
// vIOMMU. RTX PRO 6000 (GB202GL) domains need an in-guest IOMMU for
// IOMMU-protected VFIO device assignment.
const viommuGPUSubstring = "GB202GL"

const (
	viommuIoapicDriverQemu = "qemu"
	viommuModelIntel       = "intel"
	viommuFlagOn           = "on"
)

// VIOMMUDomainConfigurator adds the in-guest vIOMMU (intel-iommu with
// intremap, caching-mode and eim) plus the userspace IOAPIC to GPU
// passthrough domains that require it.
//
// The flag set intremap=on + caching-mode=on + eim=on is the one measured to
// work for 8-GPU NCCL passthrough. caching-mode='on' is not optional: QEMU
// refuses VFIO device assignment behind an intel-iommu without it and aborts
// at boot ("We need to set caching-mode=on for intel-iommu to enable device
// assignment with IOMMU protection"). eim keeps x2APIC interrupt
// remapping available, which guests with a large vCPU count need.
type VIOMMUDomainConfigurator struct{}

func NewVIOMMUDomainConfigurator() VIOMMUDomainConfigurator {
	return VIOMMUDomainConfigurator{}
}

func (v VIOMMUDomainConfigurator) Configure(vmi *v1.VirtualMachineInstance, domain *api.Domain) error {
	if !requiresVIOMMU(vmi) {
		return nil
	}

	if domain.Spec.Features == nil {
		domain.Spec.Features = &api.Features{}
	}
	// Userspace IOAPIC. libvirt translates ioapic driver='qemu' to
	// -machine kernel_irqchip=split, which QEMU requires for intel-iommu
	// with intremap=on.
	domain.Spec.Features.Ioapic = &api.FeatureIoapic{Driver: viommuIoapicDriverQemu}

	domain.Spec.Devices.IOMMU = &api.IOMMU{
		Model: viommuModelIntel,
		Driver: &api.IOMMUDriver{
			Intremap:    viommuFlagOn,
			CachingMode: viommuFlagOn,
			Eim:         viommuFlagOn,
		},
	}
	return nil
}

func requiresVIOMMU(vmi *v1.VirtualMachineInstance) bool {
	for _, hostDevice := range vmi.Spec.Domain.Devices.HostDevices {
		if strings.Contains(hostDevice.DeviceName, viommuGPUSubstring) {
			return true
		}
	}
	return false
}
