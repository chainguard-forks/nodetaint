package main

import (
	"slices"

	core_v1 "k8s.io/api/core/v1"
)

// taintExists reports whether taints holds a taint with the key and the
// effect of taint. The value of a taint is not compared.
func taintExists(taints []core_v1.Taint, taint *core_v1.Taint) bool {
	return slices.ContainsFunc(taints, func(t core_v1.Taint) bool {
		return t.MatchTaint(taint)
	})
}

// removeTaint returns a copy of node without the taints that have the key and
// the effect of taint, and whether a taint was removed. It leaves node as it
// is.
func removeTaint(node *core_v1.Node, taint *core_v1.Taint) (*core_v1.Node, bool) {
	newNode := node.DeepCopy()
	before := len(newNode.Spec.Taints)
	newNode.Spec.Taints = slices.DeleteFunc(newNode.Spec.Taints, func(t core_v1.Taint) bool {
		return t.MatchTaint(taint)
	})
	return newNode, len(newNode.Spec.Taints) != before
}
