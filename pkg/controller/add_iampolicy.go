package controller

import (
	"github.com/open-cluster-management/iam-policy-controller/pkg/controller/iampolicy"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, iampolicy.Add)
}
