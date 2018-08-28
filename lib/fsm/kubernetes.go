package fsm

import (
	"github.com/gravitational/gravity/lib/app/resources"

	"github.com/gravitational/rigging"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/extensions/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

// GetUpsertBootstrapResourceFunc returns a function that takes a Kubernetes
// object representing a bootstrap resource (ClusterRole, ClusterRoleBinding
// or PodSecurityPolicy) and creates or updates it using the provided client
func GetUpsertBootstrapResourceFunc(client *kubernetes.Clientset) resources.ResourceFunc {
	return func(object runtime.Object) (err error) {
		switch resource := object.(type) {
		case *rbacv1.ClusterRole:
			_, err = client.Rbac().ClusterRoles().Create(resource)
			if err == nil {
				log.Debugf("Created ClusterRole %q.", resource.Name)
				return nil
			}
			if !trace.IsAlreadyExists(rigging.ConvertError(err)) {
				return trace.Wrap(rigging.ConvertError(err))
			}
			_, err = client.Rbac().ClusterRoles().Update(resource)
			if err != nil {
				return trace.Wrap(rigging.ConvertError(err))
			}
			log.Debugf("Updated ClusterRole %q.", resource.Name)
		case *rbacv1.ClusterRoleBinding:
			_, err = client.Rbac().ClusterRoleBindings().Create(resource)
			if err == nil {
				log.Debugf("Created ClusterRoleBinding %q.", resource.Name)
				return nil
			}
			if !trace.IsAlreadyExists(rigging.ConvertError(err)) {
				return trace.Wrap(rigging.ConvertError(err))
			}
			_, err = client.Rbac().ClusterRoleBindings().Update(resource)
			if err != nil {
				return trace.Wrap(rigging.ConvertError(err))
			}
			log.Debugf("Updated ClusterRoleBinding %q.", resource.Name)
		case *v1beta1.PodSecurityPolicy:
			_, err = client.Extensions().PodSecurityPolicies().Create(resource)
			if err == nil {
				log.Debugf("Created PodSecurityPolicy %q.", resource.Name)
				return nil
			}
			if !trace.IsAlreadyExists(rigging.ConvertError(err)) {
				return trace.Wrap(rigging.ConvertError(err))
			}
			_, err = client.Extensions().PodSecurityPolicies().Update(resource)
			if err != nil {
				return trace.Wrap(rigging.ConvertError(err))
			}
			log.Debugf("Updated PodSecurityPolicy %q.", resource.Name)
		default:
			log.Warnf("Unsupported bootstrap resource: %#v.", resource)
			return trace.BadParameter("Unsupported bootstrap resource: %#v.", resource)
		}
		return nil
	}
}
