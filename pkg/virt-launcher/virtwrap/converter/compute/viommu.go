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
	v1 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/api"
)

// libvirtMaxVCPUsWithoutEIM mirrors the QEMU_MAX_VCPUS_WITHOUT_EIM constant
// in the libvirt 11.9.0 builds used by the Hive images: above this vCPU count
// qemuDomainDefAddDefaultDevices auto-adds an intel-iommu (intremap+eim, no
// caching-mode) to the domain definition. QEMU refuses VFIO device assignment
// behind such a vIOMMU and aborts at boot, so those domains crash-loop unless
// the definition itself carries a complete vIOMMU.
const libvirtMaxVCPUsWithoutEIM uint32 = 255

const (
	viommuIoapicDriverQemu = "qemu"
	viommuModelIntel       = "intel"
	viommuFlagOn           = "on"
)

// VIOMMUDomainConfigurator adds the in-guest vIOMMU (intel-iommu with
// intremap, caching-mode and eim) plus the userspace IOAPIC to VFIO
// passthrough domains whose vCPU count exceeds the libvirt EIM threshold.
//
// The trigger is the vCPU count, not the GPU model: libvirt adds the
// intel-iommu to every domain above libvirtMaxVCPUsWithoutEIM vCPUs, and QEMU
// refuses VFIO assignment behind it without caching-mode. Domains at or below
// the threshold are left untouched: libvirt adds no iommu there and VFIO works
// without one.
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

	domain.Spec.Devices.IOMMU = append(domain.Spec.Devices.IOMMU, api.IOMMUDevice{
		Model: viommuModelIntel,
		Driver: &api.IOMMUDriver{
			Intremap:    viommuFlagOn,
			CachingMode: viommuFlagOn,
			Eim:         viommuFlagOn,
		},
	})
	return nil
}

// requiresVIOMMU reports whether the domain definition needs the vIOMMU:
// the VMI has VFIO passthrough (host devices) and its vCPU count exceeds the
// libvirt EIM threshold. GPU model is not a factor: libvirt's trigger is the
// vCPU count, so any passthrough domain above the threshold needs the fix,
// and no domain at or below it does.
func requiresVIOMMU(vmi *v1.VirtualMachineInstance) bool {
	if len(vmi.Spec.Domain.Devices.HostDevices) == 0 {
		return false
	}
	return domainVCPUs(vmi) > libvirtMaxVCPUsWithoutEIM
}

// domainVCPUs is the total vCPU count of the VMI: cores * threads * sockets,
// with unset fields defaulting to 1.
func domainVCPUs(vmi *v1.VirtualMachineInstance) uint32 {
	cpu := vmi.Spec.Domain.CPU
	if cpu == nil {
		return 1
	}
	cores := cpu.Cores
	if cores == 0 {
		cores = 1
	}
	threads := cpu.Threads
	if threads == 0 {
		threads = 1
	}
	sockets := cpu.Sockets
	if sockets == 0 {
		sockets = 1
	}
	return cores * threads * sockets
}
