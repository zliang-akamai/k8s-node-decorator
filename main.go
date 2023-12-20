package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	metadata "github.com/linode/go-metadata"
)

var version string

func init() {
	_ = flag.Set("logtostderr", "true")
}

func GetCurrentNode(clientset kubernetes.Clientset) (*corev1.Node, error) {
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		return nil, errors.New("Environment variable POD_NAME is not set")
	}

	node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	return node, nil
}

func UpdateNodeLabels(node *corev1.Node, instanceData metadata.InstanceData) {
	klog.Infof("Updating node labels with Linode instance data: %v", instanceData)

	node.Labels["linode_label"] = instanceData.Label
	node.Labels["linode_id"] = strconv.Itoa(instanceData.ID)
	node.Labels["linode_region"] = instanceData.Region
	node.Labels["linode_type"] = instanceData.Type
	node.Labels["linode_host"] = instanceData.HostUUID
}

func main() {
	pollingIntervalSeconds := flag.Int("poll-interval", 60, "The interval (in seconds) to poll and update node information")
	flag.Parse()

	interval := time.Duration(*pollingIntervalSeconds) * time.Second

	klog.Infof("Starting Linode Kubernetes Node Decorator: version %s", version)
	klog.Infof("The poll interval is set to %v seconds", interval)

	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	node, err := GetCurrentNode(*clientset)
	if err != nil {
		panic(err.Error())
	}

	client, err := metadata.NewClient(
		context.Background(),
		metadata.ClientWithManagedToken(),
	)
	if err != nil {
		panic(err)
	}

	instanceData, err := client.GetInstance(context.Background())
	if err != nil {
		klog.Errorf("Failed to get the initial instance data: %s", err.Error())
	}

	if instanceData != nil {
		UpdateNodeLabels(node, *instanceData)
	}

	instanceWatcher := client.NewInstanceWatcher(
		metadata.WatcherWithInterval(interval),
	)

	go instanceWatcher.Start(context.Background())

	for {
		select {
		case data := <-instanceWatcher.Updates:
			if instanceData != nil {
				klog.Infof("Change to instance detected.\nNew data: %v\n", data)
				UpdateNodeLabels(node, *data)
			}

		case err := <-instanceWatcher.Errors:
			klog.Infof("Got error from instance watcher: %s", err)
		}
		klog.Infof("For loop?")
	}
}
