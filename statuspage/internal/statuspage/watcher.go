package statuspage

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// Watcher watches Kubernetes resources in a namespace and produces status snapshots.
type Watcher struct {
	clientset kubernetes.Interface
	namespace string
	hub       *SSEHub
	factory   informers.SharedInformerFactory

	mu       sync.RWMutex
	snapshot *StatusSnapshot

	debounceCh chan struct{}
}

// NewWatcher creates a Kubernetes resource watcher.
func NewWatcher(namespace string, hub *SSEHub) (*Watcher, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig for local development
		cfg, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			return nil, fmt.Errorf("unable to build k8s config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to create k8s clientset: %w", err)
	}

	w := &Watcher{
		clientset:  clientset,
		namespace:  namespace,
		hub:        hub,
		debounceCh: make(chan struct{}, 1),
	}

	return w, nil
}

// CurrentSnapshot returns the latest snapshot (thread-safe).
func (w *Watcher) CurrentSnapshot() *StatusSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.snapshot
}

// Start begins watching resources and broadcasting updates.
func (w *Watcher) Start(ctx context.Context) {
	w.factory = informers.NewSharedInformerFactoryWithOptions(
		w.clientset,
		30*time.Second,
		informers.WithNamespace(w.namespace),
	)

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc:    func(_ interface{}) { w.triggerUpdate() },
		UpdateFunc: func(_, _ interface{}) { w.triggerUpdate() },
		DeleteFunc: func(_ interface{}) { w.triggerUpdate() },
	}

	// Register informers
	w.factory.Apps().V1().Deployments().Informer().AddEventHandler(handler)
	w.factory.Apps().V1().StatefulSets().Informer().AddEventHandler(handler)
	w.factory.Apps().V1().DaemonSets().Informer().AddEventHandler(handler)
	w.factory.Core().V1().Pods().Informer().AddEventHandler(handler)
	w.factory.Core().V1().Services().Informer().AddEventHandler(handler)
	w.factory.Networking().V1().Ingresses().Informer().AddEventHandler(handler)
	w.factory.Core().V1().PersistentVolumeClaims().Informer().AddEventHandler(handler)

	w.factory.Start(ctx.Done())
	w.factory.WaitForCacheSync(ctx.Done())

	log.Printf("watcher: informers synced for namespace %s", w.namespace)

	// Build initial snapshot
	w.buildSnapshot()

	// Debounce loop
	go w.debounceLoop(ctx)
}

func (w *Watcher) triggerUpdate() {
	select {
	case w.debounceCh <- struct{}{}:
	default:
	}
}

func (w *Watcher) debounceLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.debounceCh:
			time.Sleep(time.Second) // debounce window
			// Drain any extra triggers
			for {
				select {
				case <-w.debounceCh:
				default:
					goto build
				}
			}
		build:
			w.buildSnapshot()
		}
	}
}

func (w *Watcher) buildSnapshot() {
	snap := &StatusSnapshot{
		Timestamp: time.Now(),
		Namespace: w.namespace,
	}

	// Deployments
	deployments, _ := w.factory.Apps().V1().Deployments().Lister().Deployments(w.namespace).List(labelEverything())
	for _, d := range deployments {
		snap.Deployments = append(snap.Deployments, deploymentStatus(d))
	}
	sort.Slice(snap.Deployments, func(i, j int) bool { return snap.Deployments[i].Name < snap.Deployments[j].Name })

	// StatefulSets
	statefulSets, _ := w.factory.Apps().V1().StatefulSets().Lister().StatefulSets(w.namespace).List(labelEverything())
	for _, s := range statefulSets {
		snap.StatefulSets = append(snap.StatefulSets, statefulSetStatus(s))
	}
	sort.Slice(snap.StatefulSets, func(i, j int) bool { return snap.StatefulSets[i].Name < snap.StatefulSets[j].Name })

	// DaemonSets
	daemonSets, _ := w.factory.Apps().V1().DaemonSets().Lister().DaemonSets(w.namespace).List(labelEverything())
	for _, ds := range daemonSets {
		snap.DaemonSets = append(snap.DaemonSets, daemonSetStatus(ds))
	}
	sort.Slice(snap.DaemonSets, func(i, j int) bool { return snap.DaemonSets[i].Name < snap.DaemonSets[j].Name })

	// Build owner version map from workloads for pod cross-referencing
	ownerVersions := make(map[string]string)
	for _, r := range snap.Deployments {
		ownerVersions[r.Name] = r.Version
	}
	for _, r := range snap.StatefulSets {
		ownerVersions[r.Name] = r.Version
	}
	for _, r := range snap.DaemonSets {
		ownerVersions[r.Name] = r.Version
	}

	// Pods
	pods, _ := w.factory.Core().V1().Pods().Lister().Pods(w.namespace).List(labelEverything())
	for _, p := range pods {
		ps := podStatus(p)
		// Check if pod is running an older version than its owner workload
		if ps.Owner != "" {
			if desired, ok := ownerVersions[ps.Owner]; ok && desired != "" && ps.Version != "" && ps.Version != desired {
				ps.Outdated = true
			}
		}
		snap.Pods = append(snap.Pods, ps)
	}
	sort.Slice(snap.Pods, func(i, j int) bool { return snap.Pods[i].Name < snap.Pods[j].Name })

	// Services
	services, _ := w.factory.Core().V1().Services().Lister().Services(w.namespace).List(labelEverything())
	for _, s := range services {
		snap.Services = append(snap.Services, serviceStatus(s))
	}
	sort.Slice(snap.Services, func(i, j int) bool { return snap.Services[i].Name < snap.Services[j].Name })

	// Ingresses
	ingresses, _ := w.factory.Networking().V1().Ingresses().Lister().Ingresses(w.namespace).List(labelEverything())
	for _, ing := range ingresses {
		snap.Ingresses = append(snap.Ingresses, ingressStatus(ing))
	}
	sort.Slice(snap.Ingresses, func(i, j int) bool { return snap.Ingresses[i].Name < snap.Ingresses[j].Name })

	// Build PVC-to-owner map by checking which pods mount each PVC
	pvcOwners := make(map[string]string)
	for _, p := range pods {
		owner := ""
		for _, ref := range p.OwnerReferences {
			switch ref.Kind {
			case "ReplicaSet":
				parts := strings.Split(ref.Name, "-")
				if len(parts) > 1 {
					owner = strings.Join(parts[:len(parts)-1], "-")
				}
			case "StatefulSet", "DaemonSet":
				owner = ref.Name
			}
		}
		if owner == "" {
			owner = p.Name
		}
		for _, vol := range p.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				pvcOwners[vol.PersistentVolumeClaim.ClaimName] = owner
			}
		}
	}

	// PVCs
	pvcs, _ := w.factory.Core().V1().PersistentVolumeClaims().Lister().PersistentVolumeClaims(w.namespace).List(labelEverything())
	for _, pvc := range pvcs {
		ps := pvcStatus(pvc)
		if owner, ok := pvcOwners[pvc.Name]; ok {
			ps.Owner = owner
		}
		snap.PVCs = append(snap.PVCs, ps)
	}
	sort.Slice(snap.PVCs, func(i, j int) bool { return snap.PVCs[i].Name < snap.PVCs[j].Name })

	w.mu.Lock()
	w.snapshot = snap
	w.mu.Unlock()

	w.hub.Broadcast(snap)
}

func labelEverything() labels.Selector {
	return labels.Everything()
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// extractImageTag returns the tag portion of a container image reference.
func extractImageTag(image string) string {
	// Handle digest references like image@sha256:...
	if idx := strings.LastIndex(image, "@"); idx != -1 {
		digest := image[idx+1:]
		if len(digest) > 12 {
			digest = digest[:12]
		}
		return digest
	}
	// Handle tag references like image:tag
	if idx := strings.LastIndex(image, ":"); idx != -1 {
		return image[idx+1:]
	}
	return "latest"
}

// primaryContainerImage returns the image tag of the first container in a pod spec.
func primaryContainerImage(containers []corev1.Container) string {
	if len(containers) == 0 {
		return ""
	}
	return extractImageTag(containers[0].Image)
}

func deploymentStatus(d *appsv1.Deployment) ResourceStatus {
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	ready := d.Status.ReadyReplicas

	status := StatusHealthy
	msg := ""
	if ready == 0 && desired > 0 {
		status = StatusUnhealthy
		msg = "No ready replicas"
	} else if ready < desired {
		status = StatusDegraded
		msg = fmt.Sprintf("%d/%d replicas ready", ready, desired)
	}

	return ResourceStatus{
		Name:       d.Name,
		Kind:       "Deployment",
		Status:     status,
		Ready:      fmt.Sprintf("%d/%d", ready, desired),
		Message:    msg,
		Age:        formatAge(d.CreationTimestamp.Time),
		AgeSeconds: time.Since(d.CreationTimestamp.Time).Seconds(),
		Version:    primaryContainerImage(d.Spec.Template.Spec.Containers),
	}
}

func statefulSetStatus(s *appsv1.StatefulSet) ResourceStatus {
	desired := int32(1)
	if s.Spec.Replicas != nil {
		desired = *s.Spec.Replicas
	}
	ready := s.Status.ReadyReplicas

	status := StatusHealthy
	msg := ""
	if ready == 0 && desired > 0 {
		status = StatusUnhealthy
		msg = "No ready replicas"
	} else if ready < desired {
		status = StatusDegraded
		msg = fmt.Sprintf("%d/%d replicas ready", ready, desired)
	}

	return ResourceStatus{
		Name:       s.Name,
		Kind:       "StatefulSet",
		Status:     status,
		Ready:      fmt.Sprintf("%d/%d", ready, desired),
		Message:    msg,
		Age:        formatAge(s.CreationTimestamp.Time),
		AgeSeconds: time.Since(s.CreationTimestamp.Time).Seconds(),
		Version:    primaryContainerImage(s.Spec.Template.Spec.Containers),
	}
}

func daemonSetStatus(ds *appsv1.DaemonSet) ResourceStatus {
	desired := ds.Status.DesiredNumberScheduled
	ready := ds.Status.NumberReady

	status := StatusHealthy
	msg := ""
	if ready == 0 && desired > 0 {
		status = StatusUnhealthy
		msg = "No ready pods"
	} else if ready < desired {
		status = StatusDegraded
		msg = fmt.Sprintf("%d/%d pods ready", ready, desired)
	}

	return ResourceStatus{
		Name:       ds.Name,
		Kind:       "DaemonSet",
		Status:     status,
		Ready:      fmt.Sprintf("%d/%d", ready, desired),
		Message:    msg,
		Age:        formatAge(ds.CreationTimestamp.Time),
		AgeSeconds: time.Since(ds.CreationTimestamp.Time).Seconds(),
		Version:    primaryContainerImage(ds.Spec.Template.Spec.Containers),
	}
}

func podStatus(p *corev1.Pod) ResourceStatus {
	status := StatusUnknown
	msg := string(p.Status.Phase)
	readyCount := 0
	total := len(p.Spec.Containers)

	switch p.Status.Phase {
	case corev1.PodRunning:
		status = StatusHealthy
		for _, cs := range p.Status.ContainerStatuses {
			if cs.Ready {
				readyCount++
			}
		}
		if readyCount < total {
			status = StatusDegraded
			msg = fmt.Sprintf("%d/%d containers ready", readyCount, total)
		}
	case corev1.PodSucceeded:
		status = StatusHealthy
		readyCount = total
	case corev1.PodPending:
		status = StatusDegraded
		msg = "Pending"
	case corev1.PodFailed:
		status = StatusUnhealthy
		msg = "Failed"
		if p.Status.Reason != "" {
			msg = p.Status.Reason
		}
	}

	// Determine the running image version from container statuses (actual), fall back to spec
	version := ""
	if len(p.Status.ContainerStatuses) > 0 {
		version = extractImageTag(p.Status.ContainerStatuses[0].Image)
	} else {
		version = primaryContainerImage(p.Spec.Containers)
	}

	// Find owner workload name (Deployment via ReplicaSet, StatefulSet, DaemonSet)
	owner := ""
	for _, ref := range p.OwnerReferences {
		switch ref.Kind {
		case "ReplicaSet":
			// Strip the ReplicaSet hash suffix to get the Deployment name
			// ReplicaSet names are like "deployment-name-6b7f8c9d5f"
			parts := strings.Split(ref.Name, "-")
			if len(parts) > 1 {
				owner = strings.Join(parts[:len(parts)-1], "-")
			}
		case "StatefulSet", "DaemonSet":
			owner = ref.Name
		}
	}

	return ResourceStatus{
		Name:       p.Name,
		Kind:       "Pod",
		Status:     status,
		Ready:      fmt.Sprintf("%d/%d", readyCount, total),
		Message:    msg,
		Age:        formatAge(p.CreationTimestamp.Time),
		AgeSeconds: time.Since(p.CreationTimestamp.Time).Seconds(),
		Version:    version,
		Owner:      owner,
	}
}

func serviceStatus(s *corev1.Service) ResourceStatus {
	st := StatusHealthy
	msg := string(s.Spec.Type)

	return ResourceStatus{
		Name:       s.Name,
		Kind:       "Service",
		Status:     st,
		Ready:      string(s.Spec.Type),
		Message:    msg,
		Age:        formatAge(s.CreationTimestamp.Time),
		AgeSeconds: time.Since(s.CreationTimestamp.Time).Seconds(),
	}
}

func ingressStatus(ing *networkingv1.Ingress) ResourceStatus {
	st := StatusHealthy
	hosts := ""
	for i, rule := range ing.Spec.Rules {
		if i > 0 {
			hosts += ", "
		}
		hosts += rule.Host
	}
	msg := hosts

	if len(ing.Status.LoadBalancer.Ingress) == 0 {
		st = StatusDegraded
		msg = "No load balancer assigned"
	}

	return ResourceStatus{
		Name:       ing.Name,
		Kind:       "Ingress",
		Status:     st,
		Ready:      hosts,
		Message:    msg,
		Age:        formatAge(ing.CreationTimestamp.Time),
		AgeSeconds: time.Since(ing.CreationTimestamp.Time).Seconds(),
	}
}

func pvcStatus(pvc *corev1.PersistentVolumeClaim) ResourceStatus {
	st := StatusHealthy
	phase := string(pvc.Status.Phase)

	switch pvc.Status.Phase {
	case corev1.ClaimBound:
		st = StatusHealthy
	case corev1.ClaimPending:
		st = StatusDegraded
	case corev1.ClaimLost:
		st = StatusUnhealthy
	}

	capacity := ""
	if storage, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
		capacity = storage.String()
	}

	return ResourceStatus{
		Name:       pvc.Name,
		Kind:       "PVC",
		Status:     st,
		Ready:      phase,
		Message:    capacity,
		Age:        formatAge(pvc.CreationTimestamp.Time),
		AgeSeconds: time.Since(pvc.CreationTimestamp.Time).Seconds(),
	}
}
