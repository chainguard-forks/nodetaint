package main

import (
	"reflect"
	"testing"

	core_v1 "k8s.io/api/core/v1"
)

func TestTaintExists(t *testing.T) {
	want := &core_v1.Taint{Key: "not-ready", Effect: core_v1.TaintEffectNoSchedule}

	tests := []struct {
		name   string
		taints []core_v1.Taint
		exists bool
	}{
		{"no taints", nil, false},
		{
			"same key and effect",
			[]core_v1.Taint{{Key: "not-ready", Effect: core_v1.TaintEffectNoSchedule}},
			true,
		},
		{
			"same key and effect, other value",
			[]core_v1.Taint{{Key: "not-ready", Value: "true", Effect: core_v1.TaintEffectNoSchedule}},
			true,
		},
		{
			"same key, other effect",
			[]core_v1.Taint{{Key: "not-ready", Effect: core_v1.TaintEffectNoExecute}},
			false,
		},
		{
			"other key, same effect",
			[]core_v1.Taint{{Key: "other", Effect: core_v1.TaintEffectNoSchedule}},
			false,
		},
		{
			"among other taints",
			[]core_v1.Taint{
				{Key: "other", Effect: core_v1.TaintEffectNoSchedule},
				{Key: "not-ready", Effect: core_v1.TaintEffectNoSchedule},
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := taintExists(tt.taints, want); got != tt.exists {
				t.Fatalf("taintExists() = %v, want %v", got, tt.exists)
			}
		})
	}
}

func TestRemoveTaint(t *testing.T) {
	notReady := core_v1.Taint{Key: "not-ready", Effect: core_v1.TaintEffectNoSchedule}
	otherEffect := core_v1.Taint{Key: "not-ready", Effect: core_v1.TaintEffectNoExecute}
	keepA := core_v1.Taint{Key: "keep-a", Effect: core_v1.TaintEffectNoSchedule}
	keepB := core_v1.Taint{Key: "keep-b", Effect: core_v1.TaintEffectPreferNoSchedule}

	tests := []struct {
		name    string
		taints  []core_v1.Taint
		want    []core_v1.Taint
		removed bool
	}{
		{"no taints", nil, nil, false},
		{"taint absent", []core_v1.Taint{keepA, keepB}, []core_v1.Taint{keepA, keepB}, false},
		{"same key, other effect", []core_v1.Taint{otherEffect}, []core_v1.Taint{otherEffect}, false},
		{"only taint", []core_v1.Taint{notReady}, []core_v1.Taint{}, true},
		{"keeps the others in order", []core_v1.Taint{keepA, notReady, keepB}, []core_v1.Taint{keepA, keepB}, true},
		{"removes every match", []core_v1.Taint{notReady, keepA, notReady}, []core_v1.Taint{keepA}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := newNode("node-1", tt.taints...)
			original := node.DeepCopy()

			got, removed := removeTaint(node, &notReady)

			if removed != tt.removed {
				t.Fatalf("removed = %v, want %v", removed, tt.removed)
			}
			if len(got.Spec.Taints) != len(tt.want) {
				t.Fatalf("taints = %v, want %v", got.Spec.Taints, tt.want)
			}
			for i := range tt.want {
				if got.Spec.Taints[i] != tt.want[i] {
					t.Fatalf("taints = %v, want %v", got.Spec.Taints, tt.want)
				}
			}
			if got == node {
				t.Fatal("removeTaint returned the node it was given, not a copy")
			}
			if !reflect.DeepEqual(node, original) {
				t.Fatalf("removeTaint changed the node it was given: %v", node.Spec.Taints)
			}
		})
	}
}
