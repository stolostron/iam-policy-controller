// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Define the OpenShift Group kind for tests in order to not add a dependency
// on OpenShift libraries
var groupGV = schema.GroupVersion{Group: "user.openshift.io", Version: "v1"}

type group struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Users             []string `json:"users" protobuf:"bytes,2,rep,name=users"`
}

func (in *group) DeepCopy() *group {
	if in == nil {
		return nil
	}
	out := new(group)
	in.DeepCopyInto(out)
	return out
}

func (in *group) DeepCopyInto(out *group) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Users != nil {
		in, out := &in.Users, &out.Users
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

func (in *group) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
