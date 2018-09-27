// Code generated by pegomock. DO NOT EDIT.
package matchers

import (
	"reflect"
	"github.com/petergtz/pegomock"
	kubernetes "k8s.io/client-go/kubernetes"
)

func AnyKubernetesInterface() kubernetes.Interface {
	pegomock.RegisterMatcher(pegomock.NewAnyMatcher(reflect.TypeOf((*(kubernetes.Interface))(nil)).Elem()))
	var nullValue kubernetes.Interface
	return nullValue
}

func EqKubernetesInterface(value kubernetes.Interface) kubernetes.Interface {
	pegomock.RegisterMatcher(&pegomock.EqMatcher{Value: value})
	var nullValue kubernetes.Interface
	return nullValue
}
