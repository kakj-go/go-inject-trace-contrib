// k8s client-go scenario: runs a shared pod informer against a k3s cluster,
// creates/updates/deletes a test pod, and reports readiness after the delete
// event. The informer delta processing creates
// "k8s.informer.objects.process" INTERNAL spans; each event handler callback
// creates "k8s.informer.pod.add|update|delete" INTERNAL spans.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var port = flag.String("port", "8080", "The HTTP health port")

var ready atomic.Bool

const eventTimeout = 30 * time.Second

func main() {
	flag.Parse()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", fmt.Sprintf(":%s", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	go func() {
		if err := http.Serve(ln, nil); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	go func() {
		time.Sleep(2 * time.Second)
		runInformer()
		time.Sleep(2 * time.Second)
		ready.Store(true)
	}()

	select {}
}

func runInformer() {
	kubeConfigYaml := fetchKubeconfig()
	if kubeConfigYaml == "" {
		log.Print("KUBECONFIG_YAML not set")
		return
	}

	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeConfigYaml))
	if err != nil {
		log.Printf("Failed to build kubeconfig: %v", err)
		return
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Failed to create Kubernetes client: %v", err)
		return
	}

	stopCh := make(chan struct{})
	defer func() { close(stopCh) }()

	addedCh := make(chan struct{}, 1)
	updatedCh := make(chan struct{}, 1)
	deletedCh := make(chan struct{}, 1)

	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		0,
		informers.WithNamespace(corev1.NamespaceDefault),
	)

	podInformer := factory.Core().V1().Pods()
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			pod := obj.(*corev1.Pod)
			if pod.Name != "test-pod" {
				return
			}
			log.Printf("Added Pod: %s", pod.Name)
			select {
			case addedCh <- struct{}{}:
			default:
			}
		},
		UpdateFunc: func(_, newObj any) {
			pod := newObj.(*corev1.Pod)
			if pod.Name != "test-pod" {
				return
			}
			log.Printf("Updated Pod: %s", pod.Name)
			select {
			case updatedCh <- struct{}{}:
			default:
			}
		},
		DeleteFunc: func(obj any) {
			var pod *corev1.Pod
			switch t := obj.(type) {
			case *corev1.Pod:
				pod = t
			case cache.DeletedFinalStateUnknown:
				pod = t.Obj.(*corev1.Pod)
			}
			if pod == nil || pod.Name != "test-pod" {
				return
			}
			log.Printf("Deleted Pod: %s", pod.Name)
			select {
			case deletedCh <- struct{}{}:
			default:
			}
		},
	})

	factory.Start(stopCh)

	if !cache.WaitForCacheSync(stopCh, podInformer.Informer().HasSynced) {
		log.Print("Failed to wait for caches to sync")
		return
	}

	ctx := context.Background()
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: corev1.NamespaceDefault,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            "test-container",
					Image:           "registry.k8s.io/pause",
					ImagePullPolicy: corev1.PullNever,
				},
			},
		},
	}

	// create a pod
	_, err = clientset.CoreV1().Pods(corev1.NamespaceDefault).Create(ctx, &pod, metav1.CreateOptions{})
	if err != nil {
		log.Printf("Failed to create pod: %v", err)
		return
	}

	select {
	case <-addedCh:
	case <-time.After(eventTimeout):
		log.Print("Timed out waiting for pod creation event")
		return
	}

	// update the pod
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latestPod, err := clientset.CoreV1().Pods(corev1.NamespaceDefault).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		latestPod.Labels = map[string]string{"updated": "true"}
		_, err = clientset.CoreV1().Pods(corev1.NamespaceDefault).Update(ctx, latestPod, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		log.Printf("Failed to update pod: %v", err)
		return
	}

	select {
	case <-updatedCh:
	case <-time.After(eventTimeout):
		log.Print("Timed out waiting for pod update event")
		return
	}

	// delete the pod
	err = clientset.CoreV1().Pods(corev1.NamespaceDefault).Delete(ctx, pod.Name, metav1.DeleteOptions{
		GracePeriodSeconds: new(int64),
	})
	if err != nil {
		log.Printf("Failed to delete pod: %v", err)
		return
	}

	select {
	case <-deletedCh:
	case <-time.After(eventTimeout):
		log.Print("Timed out waiting for pod deletion event")
		return
	}

	factory.Shutdown()
}

// fetchKubeconfig reads KUBECONFIG_YAML set by the runner from the k3s
// dependency, or falls back to a file for local development.
func fetchKubeconfig() string {
	if v := os.Getenv("KUBECONFIG_YAML"); v != "" {
		return v
	}
	if b, err := os.ReadFile("/ws/kubeconfig.yaml"); err == nil {
		return string(b)
	}
	return ""
}
