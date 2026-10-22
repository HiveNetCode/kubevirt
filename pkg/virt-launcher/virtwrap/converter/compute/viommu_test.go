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

package compute_test

import (
	"encoding/xml"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/libvmi"
	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/api"
	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/converter/compute"
)

var _ = Describe("VIOMMU Domain Configurator", func() {
	It("should not add a vIOMMU to domains without GB202GL host devices", func() {
		vmi := libvmi.New()
		vmi.Spec.Domain.Devices.HostDevices = []v1.HostDevice{{
			DeviceName: "GB202_GEFORCE_RTX_5090",
		}}

		var domain api.Domain
		configurator := compute.NewVIOMMUDomainConfigurator()
		Expect(configurator.Configure(vmi, &domain)).To(Succeed())

		Expect(domain.Spec.Devices.IOMMU).To(BeEmpty())
		Expect(domain.Spec.Features).To(BeNil())
	})

	It("should add the vIOMMU with caching-mode and the userspace IOAPIC for GB202GL host devices", func() {
		vmi := libvmi.New()
		vmi.Spec.Domain.Devices.HostDevices = []v1.HostDevice{{
			DeviceName: "GB202GL_RTX_PRO_6000_BLACKWELL_SERVER_EDITION",
		}}

		var domain api.Domain
		configurator := compute.NewVIOMMUDomainConfigurator()
		Expect(configurator.Configure(vmi, &domain)).To(Succeed())

		Expect(domain.Spec.Devices.IOMMU).To(HaveLen(1))
		Expect(domain.Spec.Devices.IOMMU[0]).To(Equal(api.IOMMUDevice{
			Model: "intel",
			Driver: &api.IOMMUDriver{
				Intremap:    "on",
				CachingMode: "on",
				Eim:         "on",
			},
		}))
		Expect(domain.Spec.Features.Ioapic).To(Equal(&api.FeatureIoapic{Driver: "qemu"}))
	})

	It("should serialize the vIOMMU as libvirt expects", func() {
		vmi := libvmi.New()
		vmi.Spec.Domain.Devices.HostDevices = []v1.HostDevice{{
			DeviceName: "GB202GL_RTX_PRO_6000_BLACKWELL_SERVER_EDITION",
		}}

		var domain api.Domain
		configurator := compute.NewVIOMMUDomainConfigurator()
		Expect(configurator.Configure(vmi, &domain)).To(Succeed())

		raw, err := xml.Marshal(&domain.Spec)
		Expect(err).ToNot(HaveOccurred())

		Expect(strings.Contains(string(raw), "<iommu model=\"intel\"><driver intremap=\"on\" caching_mode=\"on\" eim=\"on\"></driver></iommu>")).To(BeTrue())
		Expect(strings.Contains(string(raw), "<ioapic driver=\"qemu\"></ioapic>")).To(BeTrue())
	})
})
