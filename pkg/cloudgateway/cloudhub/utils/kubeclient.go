package utils

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/cloudgateway/cloudhub/config"
)

func KubeClient() (*kubernetes.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags(config.Config.KubeAPIConfig.Master, config.Config.KubeAPIConfig.KubeConfig)
	if err != nil {
		klog.Fatalf("Error building kubeConfig, err: %v", err)
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}
