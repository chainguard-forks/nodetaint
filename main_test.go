package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"nodetaint/config"
	"testing"
	"time"

	v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const testAnnotation = "nodetaint.example.com/ready"

// resetGlobals clears package-level state between tests.
func resetGlobals() {
	dsList = make(map[string]*v1.DaemonSet)
	podStore = make(map[string]*cache.Indexer)
}

// newTestIndexer creates a cache.Indexer pre-populated with the given objects.
func newTestIndexer(objs ...interface{}) cache.Indexer {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, obj := range objs {
		_ = indexer.Add(obj)
	}
	return indexer
}

func newReadyPod(name, namespace, nodeName, dsOwner string, annotated bool) *core_v1.Pod {
	pod := &core_v1.Pod{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []meta_v1.OwnerReference{
				{Name: dsOwner},
			},
		},
		Spec: core_v1.PodSpec{
			NodeName: nodeName,
		},
		Status: core_v1.PodStatus{
			Conditions: []core_v1.PodCondition{
				{Type: core_v1.PodReady, Status: "True"},
			},
		},
	}
	if annotated {
		pod.Annotations = map[string]string{testAnnotation: "true"}
	}
	return pod
}

func newNotReadyPod(name, namespace, nodeName, dsOwner string) *core_v1.Pod {
	return &core_v1.Pod{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				testAnnotation: "true",
			},
			OwnerReferences: []meta_v1.OwnerReference{
				{Name: dsOwner},
			},
		},
		Spec: core_v1.PodSpec{
			NodeName: nodeName,
		},
		Status: core_v1.PodStatus{
			Conditions: []core_v1.PodCondition{
				{Type: core_v1.PodReady, Status: "False"},
			},
		},
	}
}

func newDaemonSet(name string) *v1.DaemonSet {
	return &v1.DaemonSet{
		ObjectMeta: meta_v1.ObjectMeta{Name: name},
	}
}

func newNode(name string, taints ...core_v1.Taint) *core_v1.Node {
	return &core_v1.Node{
		ObjectMeta: meta_v1.ObjectMeta{Name: name},
		Spec:       core_v1.NodeSpec{Taints: taints},
	}
}

// newFakeAPIServer creates a test HTTP server that simulates a Kubernetes API
// server for node get/patch operations. It returns the clientset and a cleanup function.
func newFakeAPIServer(t *testing.T, node *core_v1.Node) (*kubernetes.Clientset, func()) {
	t.Helper()

	currentNode := node.DeepCopy()

	mux := http.NewServeMux()

	// Handle GET /api/v1/nodes/{name}
	mux.HandleFunc("/api/v1/nodes/"+node.Name, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(currentNode)
		case http.MethodPatch:
			var patch map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				t.Logf("failed to decode patch: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// Apply taint changes from the strategic merge patch
			if spec, ok := patch["spec"].(map[string]interface{}); ok {
				if taintsRaw, ok := spec["taints"]; ok {
					taintsJSON, _ := json.Marshal(taintsRaw)
					var taints []core_v1.Taint
					json.Unmarshal(taintsJSON, &taints)
					currentNode.Spec.Taints = taints
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(currentNode)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	server := httptest.NewServer(mux)

	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("failed to create clientset: %v", err)
	}

	return clientset, server.Close
}

// --- checkDSStatus tests ---

func TestCheckDSStatus_NodeNotInPodStore(t *testing.T) {
	resetGlobals()

	node := newNode("node-1")
	opts := config.Ops{DaemonSetAnnotation: testAnnotation}

	ready, err := checkDSStatus(node, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatal("expected false when node is not in podStore")
	}
}

func TestCheckDSStatus_NilIndexer(t *testing.T) {
	resetGlobals()

	node := newNode("node-1")
	podStore[node.Name] = nil
	opts := config.Ops{DaemonSetAnnotation: testAnnotation}

	ready, err := checkDSStatus(node, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatal("expected false when indexer is nil")
	}
}

func TestCheckDSStatus_EmptyIndexer(t *testing.T) {
	resetGlobals()

	node := newNode("node-1")
	indexer := newTestIndexer()
	podStore[node.Name] = &indexer
	opts := config.Ops{DaemonSetAnnotation: testAnnotation}

	ready, err := checkDSStatus(node, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatal("expected false when indexer is empty")
	}
}

func TestCheckDSStatus_AllDaemonSetsReady(t *testing.T) {
	resetGlobals()

	node := newNode("node-1")
	dsList["ds-a"] = newDaemonSet("ds-a")
	dsList["ds-b"] = newDaemonSet("ds-b")

	indexer := newTestIndexer(
		newReadyPod("pod-a", "default", node.Name, "ds-a", true),
		newReadyPod("pod-b", "default", node.Name, "ds-b", true),
	)
	podStore[node.Name] = &indexer
	opts := config.Ops{DaemonSetAnnotation: testAnnotation}

	ready, err := checkDSStatus(node, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Fatal("expected true when all daemonset pods are ready")
	}
}

func TestCheckDSStatus_PartialDaemonSetsReady(t *testing.T) {
	resetGlobals()

	node := newNode("node-1")
	dsList["ds-a"] = newDaemonSet("ds-a")
	dsList["ds-b"] = newDaemonSet("ds-b")

	indexer := newTestIndexer(
		newReadyPod("pod-a", "default", node.Name, "ds-a", true),
	)
	podStore[node.Name] = &indexer
	opts := config.Ops{DaemonSetAnnotation: testAnnotation}

	ready, err := checkDSStatus(node, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatal("expected false when only some daemonset pods are ready")
	}
}

func TestCheckDSStatus_PodNotReady(t *testing.T) {
	resetGlobals()

	node := newNode("node-1")
	dsList["ds-a"] = newDaemonSet("ds-a")

	indexer := newTestIndexer(
		newNotReadyPod("pod-a", "default", node.Name, "ds-a"),
	)
	podStore[node.Name] = &indexer
	opts := config.Ops{DaemonSetAnnotation: testAnnotation}

	ready, err := checkDSStatus(node, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatal("expected false when pod is not ready")
	}
}

func TestCheckDSStatus_PodMissingAnnotation(t *testing.T) {
	resetGlobals()

	node := newNode("node-1")
	dsList["ds-a"] = newDaemonSet("ds-a")

	indexer := newTestIndexer(
		newReadyPod("pod-a", "default", node.Name, "ds-a", false),
	)
	podStore[node.Name] = &indexer
	opts := config.Ops{DaemonSetAnnotation: testAnnotation}

	ready, err := checkDSStatus(node, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatal("expected false when pod lacks the required annotation")
	}
}

func TestCheckDSStatus_PodWithNoOwnerReferences(t *testing.T) {
	resetGlobals()

	node := newNode("node-1")
	dsList["ds-a"] = newDaemonSet("ds-a")

	pod := &core_v1.Pod{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:        "orphan-pod",
			Namespace:   "default",
			Annotations: map[string]string{testAnnotation: "true"},
		},
		Status: core_v1.PodStatus{
			Conditions: []core_v1.PodCondition{
				{Type: core_v1.PodReady, Status: "True"},
			},
		},
	}
	indexer := newTestIndexer(pod)
	podStore[node.Name] = &indexer
	opts := config.Ops{DaemonSetAnnotation: testAnnotation}

	ready, err := checkDSStatus(node, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatal("expected false when pod has no owner references")
	}
}

func TestCheckDSStatus_PodOwnedByUnknownDaemonSet(t *testing.T) {
	resetGlobals()

	node := newNode("node-1")
	dsList["ds-a"] = newDaemonSet("ds-a")

	indexer := newTestIndexer(
		newReadyPod("pod-x", "default", node.Name, "ds-unknown", true),
	)
	podStore[node.Name] = &indexer
	opts := config.Ops{DaemonSetAnnotation: testAnnotation}

	ready, err := checkDSStatus(node, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatal("expected false when pod is owned by an untracked daemonset")
	}
}

func TestCheckDSStatus_NoDaemonSetsRequired(t *testing.T) {
	resetGlobals()

	node := newNode("node-1")
	// dsList is empty — no daemonsets required

	indexer := newTestIndexer(
		newReadyPod("pod-a", "default", node.Name, "ds-a", true),
	)
	podStore[node.Name] = &indexer
	opts := config.Ops{DaemonSetAnnotation: testAnnotation}

	ready, err := checkDSStatus(node, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Fatal("expected true when no daemonsets are required (0 == 0)")
	}
}

// --- RemoveTaintOffNode tests ---

func TestRemoveTaintOffNode_RemovesTaint(t *testing.T) {
	taint := core_v1.Taint{
		Key:    "node.kubernetes.io/not-ready",
		Effect: core_v1.TaintEffectNoSchedule,
	}
	node := newNode("node-1", taint)
	node.ResourceVersion = "1"

	client, cleanup := newFakeAPIServer(t, node)
	defer cleanup()

	ctx := context.Background()
	err := RemoveTaintOffNode(ctx, client, node, &taint)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, err := client.CoreV1().Nodes().Get(ctx, "node-1", meta_v1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	for _, t2 := range updated.Spec.Taints {
		if t2.Key == taint.Key && t2.Effect == taint.Effect {
			t.Fatal("taint was not removed from the node")
		}
	}
}

func TestRemoveTaintOffNode_NoOpWhenTaintAbsent(t *testing.T) {
	node := newNode("node-1")
	node.ResourceVersion = "1"

	patchCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/nodes/"+node.Name, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			patchCalled = true
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(node)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("failed to create clientset: %v", err)
	}

	taint := &core_v1.Taint{
		Key:    "node.kubernetes.io/not-ready",
		Effect: core_v1.TaintEffectNoSchedule,
	}

	err = RemoveTaintOffNode(context.Background(), client, node, taint)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patchCalled {
		t.Fatal("expected no patch when taint is already absent")
	}
}

func TestRemoveTaintOffNode_MultiTaintRemoval(t *testing.T) {
	taintA := core_v1.Taint{Key: "taint-a", Effect: core_v1.TaintEffectNoSchedule}
	taintB := core_v1.Taint{Key: "taint-b", Effect: core_v1.TaintEffectNoExecute}
	keepTaint := core_v1.Taint{Key: "keep-me", Effect: core_v1.TaintEffectPreferNoSchedule}

	node := newNode("node-1", taintA, taintB, keepTaint)
	node.ResourceVersion = "1"

	client, cleanup := newFakeAPIServer(t, node)
	defer cleanup()

	ctx := context.Background()
	err := RemoveTaintOffNode(ctx, client, node, &taintA, &taintB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, err := client.CoreV1().Nodes().Get(ctx, "node-1", meta_v1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	if len(updated.Spec.Taints) != 1 {
		t.Fatalf("expected 1 remaining taint, got %d", len(updated.Spec.Taints))
	}
	if updated.Spec.Taints[0].Key != "keep-me" {
		t.Fatalf("expected keep-me taint to remain, got %s", updated.Spec.Taints[0].Key)
	}
}

// --- PatchNodeTaints tests ---

func TestPatchNodeTaints_AppliesPatch(t *testing.T) {
	taint := core_v1.Taint{Key: "test-taint", Effect: core_v1.TaintEffectNoSchedule}
	oldNode := newNode("node-1", taint)
	oldNode.ResourceVersion = "1"

	newNodeObj := oldNode.DeepCopy()
	newNodeObj.Spec.Taints = nil

	client, cleanup := newFakeAPIServer(t, oldNode)
	defer cleanup()

	ctx := context.Background()
	err := PatchNodeTaints(client, "node-1", oldNode, newNodeObj, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, err := client.CoreV1().Nodes().Get(ctx, "node-1", meta_v1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	if len(updated.Spec.Taints) != 0 {
		t.Fatalf("expected 0 taints after patch, got %d", len(updated.Spec.Taints))
	}
}

func TestPatchNodeTaints_NoChangeIsNoOp(t *testing.T) {
	node := newNode("node-1")
	node.ResourceVersion = "1"

	client, cleanup := newFakeAPIServer(t, node)
	defer cleanup()

	err := PatchNodeTaints(client, "node-1", node, node, context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- contextForChannel tests ---

func TestContextForChannel_CancelledByParent(t *testing.T) {
	parentCh := make(chan struct{})
	ctx, cancel := contextForChannel(parentCh)
	defer cancel()

	close(parentCh)

	select {
	case <-ctx.Done():
		// expected
	case <-time.After(time.Second):
		t.Fatal("context was not cancelled after parent channel closed")
	}
}

func TestContextForChannel_CancelledByFunc(t *testing.T) {
	parentCh := make(chan struct{})
	ctx, cancel := contextForChannel(parentCh)

	cancel()

	select {
	case <-ctx.Done():
		// expected
	case <-time.After(time.Second):
		t.Fatal("context was not cancelled after cancel() called")
	}
}

func TestContextForChannel_ActiveUntilSignalled(t *testing.T) {
	parentCh := make(chan struct{})
	ctx, cancel := contextForChannel(parentCh)
	defer cancel()

	select {
	case <-ctx.Done():
		t.Fatal("context should not be cancelled yet")
	case <-time.After(50 * time.Millisecond):
		// expected — context still active
	}
}

// --- setupLogging tests ---

func TestSetupLogging_ValidLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}
	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			setupLogging(level)
		})
	}
}
