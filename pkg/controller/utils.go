/*
Copyright 2019 Gravitational, Inc.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"os"

	"github.com/gravitational/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func runningInPod() bool {
	return os.Getenv("POD_NAME") != ""
}

func (c *controller) detectNodeName() error {
	c.Debug("Attempting to detect nodename.")
	defer func() { c.Info("Detected hostname: ", c.config.NodeName) }()
	// if we're running inside a pod
	// find the node the pod is assigned to
	podName := os.Getenv("POD_NAME")
	podNamespace := os.Getenv("POD_NAMESPACE")
	if podName != "" && podNamespace != "" {
		return trace.Wrap(c.updateNodeNameFromPod(podName, podNamespace))
	}

	nodeName, err := os.Hostname()
	if err != nil {
		return trace.Wrap(err)
	}
	// TODO(knisbet) we should probably validate here, a node object exists that matches our node name
	c.config.NodeName = nodeName
	return nil
}

func (c *controller) updateNodeNameFromPod(podName, podNamespace string) error {
	pod, err := c.client.CoreV1().Pods(podNamespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return trace.Wrap(err)
	}
	c.config.NodeName = pod.Spec.NodeName
	if c.config.NodeName == "" {
		return trace.BadParameter("node name not present in pod spec %v/%v", podNamespace, podName)
	}
	return nil
}
