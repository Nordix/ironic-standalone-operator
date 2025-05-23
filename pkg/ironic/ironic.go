package ironic

import (
	"context"
	"errors"

	metal3api "github.com/metal3-io/ironic-standalone-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	defaultExposedPort = 80
	httpsExposedPort   = 443
)

func ironicDeploymentName(ironic *metal3api.Ironic) string {
	return ironic.Name + "-service"
}

func ensureIronicDaemonSet(cctx ControllerContext, resources Resources) (Status, error) {
	template, err := newIronicPodTemplate(cctx, resources)
	if err != nil {
		return transientError(err)
	}

	deploy := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ironicDeploymentName(resources.Ironic),
			Namespace: resources.Ironic.Namespace,
		},
	}
	result, err := controllerutil.CreateOrUpdate(cctx.Context, cctx.Client, deploy, func() error {
		if deploy.ObjectMeta.CreationTimestamp.IsZero() {
			cctx.Logger.Info("creating a new ironic daemon set")
		}
		if deploy.Labels == nil {
			deploy.Labels = make(map[string]string, 2)
		}
		deploy.Labels[metal3api.IronicServiceLabel] = resources.Ironic.Name
		deploy.Labels[metal3api.IronicVersionLabel] = cctx.VersionInfo.InstalledVersion.String()

		matchLabels := map[string]string{metal3api.IronicAppLabel: ironicDeploymentName(resources.Ironic)}
		deploy.Spec.Selector = &metav1.LabelSelector{MatchLabels: matchLabels}
		mergePodTemplates(&deploy.Spec.Template, template)
		return controllerutil.SetControllerReference(resources.Ironic, deploy, cctx.Scheme)
	})
	if err != nil {
		return transientError(err)
	}
	if result != controllerutil.OperationResultNone {
		cctx.Logger.Info("ironic daemon set", "DaemonSet", deploy.Name, "Status", result)
		return updated()
	}

	return getDaemonSetStatus(cctx, deploy)
}

func ensureIronicDeployment(cctx ControllerContext, resources Resources) (Status, error) {
	template, err := newIronicPodTemplate(cctx, resources)
	if err != nil {
		return transientError(err)
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ironicDeploymentName(resources.Ironic),
			Namespace: resources.Ironic.Namespace,
		},
	}
	result, err := controllerutil.CreateOrUpdate(cctx.Context, cctx.Client, deploy, func() error {
		if deploy.ObjectMeta.CreationTimestamp.IsZero() {
			cctx.Logger.Info("creating a new ironic deployment")
		}
		if deploy.Labels == nil {
			deploy.Labels = make(map[string]string, 2)
		}
		deploy.Labels[metal3api.IronicServiceLabel] = resources.Ironic.Name
		deploy.Labels[metal3api.IronicVersionLabel] = cctx.VersionInfo.InstalledVersion.String()

		matchLabels := map[string]string{metal3api.IronicAppLabel: ironicDeploymentName(resources.Ironic)}
		deploy.Spec.Selector = &metav1.LabelSelector{MatchLabels: matchLabels}
		deploy.Spec.Replicas = ptr.To(int32(1))
		mergePodTemplates(&deploy.Spec.Template, template)
		// We cannot run two copies of Ironic in parallel
		deploy.Spec.Strategy = appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
		}
		return controllerutil.SetControllerReference(resources.Ironic, deploy, cctx.Scheme)
	})
	if err != nil {
		return transientError(err)
	}
	if result != controllerutil.OperationResultNone {
		cctx.Logger.Info("ironic deployment", "Deployment", deploy.Name, "Status", result)
		return updated()
	}

	return getDeploymentStatus(cctx, deploy)
}

func ensureIronicService(cctx ControllerContext, ironic *metal3api.Ironic) (Status, error) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: ironic.Name, Namespace: ironic.Namespace},
	}
	exposedPort := int32(defaultExposedPort)
	if ironic.Spec.TLS.CertificateName != "" {
		exposedPort = httpsExposedPort
	}
	result, err := controllerutil.CreateOrUpdate(cctx.Context, cctx.Client, service, func() error {
		if service.ObjectMeta.Labels == nil {
			cctx.Logger.Info("creating a new ironic service")
			service.ObjectMeta.Labels = make(map[string]string, 2)
		}
		service.Labels[metal3api.IronicServiceLabel] = ironic.Name
		service.Labels[metal3api.IronicVersionLabel] = cctx.VersionInfo.InstalledVersion.String()

		service.Spec.Selector = map[string]string{metal3api.IronicAppLabel: ironicDeploymentName(ironic)}
		service.Spec.Ports = []corev1.ServicePort{
			{
				Protocol:   corev1.ProtocolTCP,
				Port:       exposedPort,
				TargetPort: intstr.FromString(ironicPortName),
			},
		}
		service.Spec.Type = corev1.ServiceTypeClusterIP

		return controllerutil.SetControllerReference(ironic, service, cctx.Scheme)
	})
	if result != controllerutil.OperationResultNone {
		cctx.Logger.Info("ironic service", "Service", service.Name, "Status", result)
		return updated()
	}
	if err != nil {
		return transientError(err)
	}

	return getServiceStatus(service)
}

func removeIronicDaemonSet(cctx ControllerContext, ironic *metal3api.Ironic) error {
	err := cctx.KubeClient.AppsV1().DaemonSets(ironic.Namespace).
		Delete(context.Background(), ironicDeploymentName(ironic), metav1.DeleteOptions{})
	return client.IgnoreNotFound(err)
}

func removeIronicDeployment(cctx ControllerContext, ironic *metal3api.Ironic) error {
	err := cctx.KubeClient.AppsV1().Deployments(ironic.Namespace).
		Delete(context.Background(), ironicDeploymentName(ironic), metav1.DeleteOptions{})
	return client.IgnoreNotFound(err)
}

// EnsureIronic deploys Ironic either as a Deployment or as a DaemonSet.
func EnsureIronic(cctx ControllerContext, resources Resources) (status Status, err error) {
	if validationErr := ValidateIronic(&resources.Ironic.Spec, nil); validationErr != nil {
		status = Status{Fatal: validationErr}
		return
	}

	if resources.Ironic.Spec.HighAvailability && cctx.VersionInfo.InstalledVersion.Compare(versionWithoutAuthConfig) < 0 {
		err = errors.New("using HA is only possible for Ironic 28.0 or newer")
		status = Status{Fatal: err}
		return
	}

	if resources.Ironic.Spec.Database != nil {
		jobStatus, err := ensureIronicUpgradeJob(cctx, resources, preUpgrade)
		if err != nil || !jobStatus.IsReady() {
			return jobStatus, err
		}
	}

	if resources.Ironic.Spec.HighAvailability {
		err = removeIronicDeployment(cctx, resources.Ironic)
		if err != nil {
			return
		}
		status, err = ensureIronicDaemonSet(cctx, resources)
	} else {
		err = removeIronicDaemonSet(cctx, resources.Ironic)
		if err != nil {
			return
		}
		status, err = ensureIronicDeployment(cctx, resources)
	}

	if err != nil || status.IsError() {
		return
	}

	// Let the service be created while Ironic is being deployed, but do
	// not report overall success until both are done.
	serviceStatus, err := ensureIronicService(cctx, resources.Ironic)
	if err != nil || !serviceStatus.IsReady() {
		return serviceStatus, err
	}

	if resources.Ironic.Spec.Database != nil {
		jobStatus, err := ensureIronicUpgradeJob(cctx, resources, postUpgrade)
		if err != nil || !jobStatus.IsReady() {
			return jobStatus, err
		}
	}

	return
}

// RemoveIronic removes all bits of the Ironic deployment.
func RemoveIronic(cctx ControllerContext, ironic *metal3api.Ironic) error {
	return nil // rely on ownership-based clean up
}
