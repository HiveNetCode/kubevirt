package services

import (
	"strconv"
	"strings"

	k8sv1 "k8s.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/pointer"

	v1 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/util"
	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/api"
	"kubevirt.io/kubevirt/pkg/vmitrait"
)

const (
	cacheHomeEnvVarName  = "XDG_CACHE_HOME"
	configHomeEnvVarName = "XDG_CONFIG_HOME"
	runtimeDirEnvVarName = "XDG_RUNTIME_DIR"
)

type ContainerSpecRenderer struct {
	imgPullPolicy            k8sv1.PullPolicy
	launcherImg              string
	name                     string
	userID                   int64
	volumeDevices            []k8sv1.VolumeDevice
	volumeMounts             []k8sv1.VolumeMount
	sharedFilesystems        []string
	resources                k8sv1.ResourceRequirements
	liveninessProbe          *k8sv1.Probe
	readinessProbe           *k8sv1.Probe
	ports                    []k8sv1.ContainerPort
	capabilities             *k8sv1.Capabilities
	allowPrivilegeEscalation *bool
	appArmorProfile          *k8sv1.AppArmorProfile
	args                     []string
	extraEnvVars             []k8sv1.EnvVar
}

type Option func(*ContainerSpecRenderer)

func NewContainerSpecRenderer(containerName string, launcherImg string, imgPullPolicy k8sv1.PullPolicy, opts ...Option) *ContainerSpecRenderer {
	computeContainerSpec := &ContainerSpecRenderer{
		imgPullPolicy: imgPullPolicy,
		launcherImg:   launcherImg,
		name:          containerName,
	}
	for _, opt := range opts {
		opt(computeContainerSpec)
	}
	return computeContainerSpec
}

func (csr *ContainerSpecRenderer) Render(cmd []string) k8sv1.Container {
	container := k8sv1.Container{
		Name:                     csr.name,
		Image:                    csr.launcherImg,
		ImagePullPolicy:          csr.imgPullPolicy,
		SecurityContext:          securityContext(csr.userID, csr.capabilities, csr.allowPrivilegeEscalation),
		Command:                  cmd,
		VolumeDevices:            csr.volumeDevices,
		VolumeMounts:             csr.volumeMounts,
		Resources:                csr.resources,
		Ports:                    csr.ports,
		Env:                      csr.envVars(),
		LivenessProbe:            csr.liveninessProbe,
		ReadinessProbe:           csr.readinessProbe,
		Args:                     csr.args,
		TerminationMessagePolicy: k8sv1.TerminationMessageFallbackToLogsOnError,
	}

	if csr.appArmorProfile != nil {
		container.SecurityContext.AppArmorProfile = csr.appArmorProfile
	}

	return container
}

func (csr *ContainerSpecRenderer) envVars() []k8sv1.EnvVar {
	var env []k8sv1.EnvVar

	if csr.userID != 0 {
		env = append(env, xdgEnvironmentVariables()...)
	}

	if len(csr.sharedFilesystems) != 0 {
		env = append(env, k8sv1.EnvVar{
			Name:  util.ENV_VAR_SHARED_FILESYSTEM_PATHS,
			Value: strings.Join(csr.sharedFilesystems, ":"),
		})
	}

	env = append(env, csr.extraEnvVars...)

	return env
}

func WithNonRoot(userID int64) Option {
	return func(renderer *ContainerSpecRenderer) {
		renderer.userID = userID
	}
}

func WithCapabilities(vmi *v1.VirtualMachineInstance) Option {
	return func(renderer *ContainerSpecRenderer) {
		if renderer.capabilities == nil {
			renderer.capabilities = &k8sv1.Capabilities{
				Add: requiredCapabilities(vmi),
			}
		} else {
			renderer.capabilities.Add = requiredCapabilities(vmi)
		}
	}
}

// WithAllowPrivilegeEscalation overrides the default allowPrivilegeEscalation
// of the rendered container. For non-root containers the default is false,
// which makes the container runtime enable no-new-privs and suppress the
// file capabilities set on launcher image files (see rpm/BUILD.bazel).
func WithAllowPrivilegeEscalation(allow bool) Option {
	return func(renderer *ContainerSpecRenderer) {
		renderer.allowPrivilegeEscalation = pointer.P(allow)
	}
}

// WithAppArmorProfile overrides the AppArmor profile of the rendered
// container. The compute container of passt VMIs runs unconfined because
// the node's CRI profile (cri-containerd.apparmor.d) denies the mount(2)
// call the passt backend's self-sandbox uses to remount / as unbindable
// (see template.go).
func WithAppArmorProfile(profile k8sv1.AppArmorProfile) Option {
	return func(renderer *ContainerSpecRenderer) {
		renderer.appArmorProfile = &profile
	}
}

func WithDropALLCapabilities() Option {
	return func(renderer *ContainerSpecRenderer) {
		if renderer.capabilities == nil {
			renderer.capabilities = &k8sv1.Capabilities{
				Drop: []k8sv1.Capability{"ALL"},
			}
		} else {
			renderer.capabilities.Drop = []k8sv1.Capability{"ALL"}
		}
	}
}

func WithNoCapabilities() Option {
	return func(renderer *ContainerSpecRenderer) {
		renderer.capabilities = &k8sv1.Capabilities{
			Drop: []k8sv1.Capability{"ALL"},
		}
	}
}

func WithVolumeDevices(devices ...k8sv1.VolumeDevice) Option {
	return func(renderer *ContainerSpecRenderer) {
		renderer.volumeDevices = devices
	}
}

func WithVolumeMounts(mounts ...k8sv1.VolumeMount) Option {
	return func(renderer *ContainerSpecRenderer) {
		renderer.volumeMounts = mounts
	}
}

func WithSharedFilesystems(paths ...string) Option {
	return func(renderer *ContainerSpecRenderer) {
		renderer.sharedFilesystems = paths
	}
}

func WithResourceRequirements(resources k8sv1.ResourceRequirements) Option {
	return func(renderer *ContainerSpecRenderer) {
		renderer.resources = resources
	}
}

func WithPorts(vmi *v1.VirtualMachineInstance) Option {
	return func(renderer *ContainerSpecRenderer) {
		renderer.ports = containerPortsFromVMI(vmi)
	}
}

func WithArgs(args []string) Option {
	return func(renderer *ContainerSpecRenderer) {
		renderer.args = args
	}
}

func WithLivelinessProbe(vmi *v1.VirtualMachineInstance) Option {
	return func(renderer *ContainerSpecRenderer) {
		v1.SetDefaults_Probe(vmi.Spec.LivenessProbe)
		renderer.liveninessProbe = copyProbe(vmi.Spec.LivenessProbe)
		updateLivenessProbe(vmi, renderer.liveninessProbe)
	}
}

func WithReadinessProbe(vmi *v1.VirtualMachineInstance) Option {
	return func(renderer *ContainerSpecRenderer) {
		v1.SetDefaults_Probe(vmi.Spec.ReadinessProbe)
		renderer.readinessProbe = copyProbe(vmi.Spec.ReadinessProbe)
		updateReadinessProbe(vmi, renderer.readinessProbe)
	}
}

func WithExtraEnvVars(envVars []k8sv1.EnvVar) Option {
	return func(renderer *ContainerSpecRenderer) {
		renderer.extraEnvVars = append(renderer.extraEnvVars, envVars...)
	}
}

func xdgEnvironmentVariables() []k8sv1.EnvVar {
	const varRun = "/var/run"
	return []k8sv1.EnvVar{
		{
			Name:  cacheHomeEnvVarName,
			Value: util.VirtPrivateDir,
		},
		{
			Name:  configHomeEnvVarName,
			Value: util.VirtPrivateDir,
		},
		{
			Name:  runtimeDirEnvVarName,
			Value: varRun,
		},
	}
}

func securityContext(userId int64, requiredCapabilities *k8sv1.Capabilities, allowPrivilegeEscalation *bool) *k8sv1.SecurityContext {
	isNonRoot := userId != 0
	context := &k8sv1.SecurityContext{
		RunAsUser:    &userId,
		RunAsNonRoot: &isNonRoot,
		Capabilities: requiredCapabilities,
	}

	if isNonRoot {
		context.RunAsGroup = &userId
		context.AllowPrivilegeEscalation = pointer.P(false)
	}

	// The kernel refuses to grant file capabilities (e.g. those on
	// /usr/bin/passt, see rpm/BUILD.bazel) to processes running with
	// no-new-privs, which the container runtime enables for containers
	// with allowPrivilegeEscalation=false. An explicit override is
	// required for the passt backend to reach its file capabilities.
	if allowPrivilegeEscalation != nil {
		context.AllowPrivilegeEscalation = allowPrivilegeEscalation
	}

	return context
}

func containerPortsFromVMI(vmi *v1.VirtualMachineInstance) []k8sv1.ContainerPort {
	var ports []k8sv1.ContainerPort

	for _, iface := range vmi.Spec.Domain.Devices.Interfaces {
		if iface.Ports != nil {
			for _, port := range iface.Ports {
				if port.Protocol == "" {
					port.Protocol = "TCP"
				}

				ports = append(ports, k8sv1.ContainerPort{Protocol: k8sv1.Protocol(port.Protocol), Name: port.Name, ContainerPort: port.Port})
			}
		}
	}

	return ports
}

func updateReadinessProbe(vmi *v1.VirtualMachineInstance, computeProbe *k8sv1.Probe) {
	if vmi.Spec.ReadinessProbe.GuestAgentPing != nil {
		wrapGuestAgentPingWithVirtProbe(vmi, computeProbe)
		computeProbe.InitialDelaySeconds = computeProbe.InitialDelaySeconds + LibvirtStartupDelay
		return
	}
	wrapExecProbeWithVirtProbe(vmi, computeProbe)
	computeProbe.InitialDelaySeconds = computeProbe.InitialDelaySeconds + LibvirtStartupDelay
}

func updateLivenessProbe(vmi *v1.VirtualMachineInstance, computeProbe *k8sv1.Probe) {
	if vmi.Spec.LivenessProbe.GuestAgentPing != nil {
		wrapGuestAgentPingWithVirtProbe(vmi, computeProbe)
		computeProbe.InitialDelaySeconds = computeProbe.InitialDelaySeconds + LibvirtStartupDelay
		return
	}
	wrapExecProbeWithVirtProbe(vmi, computeProbe)
	computeProbe.InitialDelaySeconds = computeProbe.InitialDelaySeconds + LibvirtStartupDelay
}

func wrapExecProbeWithVirtProbe(vmi *v1.VirtualMachineInstance, probe *k8sv1.Probe) {
	if probe == nil || probe.ProbeHandler.Exec == nil {
		return
	}

	originalCommand := probe.ProbeHandler.Exec.Command
	if len(originalCommand) < 1 || originalCommand[0] == "virt-probe" {
		return
	}

	wrappedCommand := []string{
		"virt-probe",
		"--domainName", api.VMINamespaceKeyFunc(vmi),
		"--timeoutSeconds", strconv.FormatInt(int64(probe.TimeoutSeconds), 10),
		"--command", originalCommand[0],
		"--",
	}
	wrappedCommand = append(wrappedCommand, originalCommand[1:]...)

	probe.ProbeHandler.Exec.Command = wrappedCommand
	// we add 1s to the pod probe to compensate for the additional steps in probing
	probe.TimeoutSeconds += 1
}

func requiredCapabilities(vmi *v1.VirtualMachineInstance) []k8sv1.Capability {
	// These capabilies are always required because we set them on virt-launcher binary
	capabilities := []k8sv1.Capability{CAP_NET_BIND_SERVICE}

	if !vmitrait.IsNonRoot(vmi) {
		// add a CAP_SYS_NICE capability to allow setting cpu affinity
		capabilities = append(capabilities, CAP_SYS_NICE)
	}

	// passt sandboxes itself: remount / read-only (CAP_SYS_ADMIN), drop
	// capabilities and supplementary groups, and drop to "nobody"
	// (CAP_SETUID, CAP_SETGID). The launcher runs as non-root, where
	// Kubernetes only adds requested capabilities to the bounding set, so
	// these caps reach the passt process through the file capabilities set
	// on /usr/bin/passt in the launcher image (effective = file caps ∩
	// bounding). Without them passt exits with "Failed to remount /:
	// Permission denied" and the domain never starts.
	if vmitrait.UsesPasstBinding(vmi) {
		capabilities = append(capabilities, CAP_SYS_ADMIN, CAP_SETUID, CAP_SETGID)
	}

	return capabilities
}
