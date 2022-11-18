/*
Copyright 2022 the Velero contributors.

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

package restore

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/vmware-tanzu/velero/pkg/plugin/framework/common"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

const (
	DELIMITER_VALUE = ","
)

// ChangeImageNameAction updates a deployment or Pod's image name
// if a mapping is found in the plugin's config map.
type ChangeImageNameAction struct {
	logger          logrus.FieldLogger
	configMapClient corev1client.ConfigMapInterface
}

// NewChangeImageNameAction is the constructor for ChangeImageNameAction.
func NewChangeImageNameAction(
	logger logrus.FieldLogger,
	configMapClient corev1client.ConfigMapInterface,
) *ChangeImageNameAction {
	return &ChangeImageNameAction{
		logger:          logger,
		configMapClient: configMapClient,
	}
}

// AppliesTo returns the resources that ChangeImageNameAction should
// be run for.
func (a *ChangeImageNameAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"deployments", "statefulsets", "daemonsets", "replicasets", "replicationcontrollers", "jobs", "cronjobs", "pods"},
	}, nil
}

// Execute updates the item's spec.containers' image if a mapping is found
// in the config map for the plugin.
func (a *ChangeImageNameAction) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	a.logger.Info("Executing ChangeImageNameAction")
	defer a.logger.Info("Done executing ChangeImageNameAction")

	config, err := getPluginConfig(common.PluginKindRestoreItemAction, "velero.io/change-image-repository", a.configMapClient)
	if err != nil {
		return nil, err
	}

	if config == nil || len(config.Data) == 0 {
		a.logger.Info("No image name mappings found")
		return velero.NewRestoreItemActionExecuteOutput(input.Item), nil
	}

	obj, ok := input.Item.(*unstructured.Unstructured)
	if !ok {
		return nil, errors.Errorf("object was of unexpected type %T", input.Item)
	}

	log := a.logger.WithFields(map[string]interface{}{
		"kind":      obj.GetKind(),
		"namespace": obj.GetNamespace(),
		"name":      obj.GetName(),
	})

	if obj.GetKind() == "Pod" {
		pod := new(corev1.Pod)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), pod); err != nil {
			a.logger.Infof("UnstructuredConverter meet error: %v", err)
			return nil, err
		}
		needUpdateObj := false

		if len(pod.Spec.Containers) > 0 {
			for i, container := range pod.Spec.Containers {
				if exists, newImageName, err := a.isImageReplaceRuleExist(log, container.Image, config); exists && err == nil {
					needUpdateObj = true
					a.logger.Infof("Updating item's image from %s to %s", container.Image, newImageName)
					pod.Spec.Containers[i].Image = newImageName
				}
			}
		}

		if len(pod.Spec.InitContainers) > 0 {
			for i, container := range pod.Spec.InitContainers {
				if exists, newImageName, err := a.isImageReplaceRuleExist(log, container.Image, config); exists && err == nil {
					needUpdateObj = true
					a.logger.Infof("Updating item's image from %s to %s", container.Image, newImageName)
					pod.Spec.InitContainers[i].Image = newImageName
				}
			}
		}
		if needUpdateObj {
			newObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pod)
			if err != nil {
				return nil, errors.Wrap(err, "convert obj to Pod failed")
			}
			obj.Object = newObj
		}

	} else if obj.GetKind() == "Job" || obj.GetKind() == "CronJob" {
		//handle containers
		needUpdateObj := false
		containers, _, err := unstructured.NestedSlice(obj.UnstructuredContent(), "spec", "jobTemplate", "spec", "template", "spec", "containers")
		if err != nil {
			a.logger.Infof("UnstructuredConverter meet error: %v", err)
			return nil, errors.Wrap(err, "error getting item's spec.containers")
		}
		for i, container := range containers {
			a.logger.Infoln("container:", container)
			if image, ok := container.(map[string]interface{})["image"]; ok {
				imageName := image.(string)
				if exists, newImageName, err := a.isImageReplaceRuleExist(log, imageName, config); exists && err == nil {
					needUpdateObj = true
					a.logger.Infof("Updating item's image from %s to %s", imageName, newImageName)
					container.(map[string]interface{})["image"] = newImageName
					containers[i] = container
				}
			}
		}

		if needUpdateObj {
			if err := unstructured.SetNestedField(obj.UnstructuredContent(), containers, "spec", "jobTemplate", "spec", "template", "spec", "containers"); err != nil {
				return nil, errors.Wrap(err, "unable to set item's container image")
			}
			a.logger.Infof("obj.Object %v", obj.Object)
		}

		//handle initContainers
		needUpdateObj = false
		initContainers, _, err := unstructured.NestedSlice(obj.UnstructuredContent(), "spec", "jobTemplate", "spec", "template", "spec", "initContainers")
		if err != nil {
			return nil, errors.Wrap(err, "error getting item's spec.initContainers")
		}
		for i, container := range initContainers {
			if image, ok := container.(map[string]interface{})["image"]; ok {
				imageName := image.(string)
				if exists, newImageName, err := a.isImageReplaceRuleExist(log, imageName, config); exists && err == nil {
					needUpdateObj = true
					a.logger.Infof("Updating item's image from %s to %s", imageName, newImageName)
					container.(map[string]interface{})["image"] = newImageName
					initContainers[i] = container
				}
			}
		}

		if needUpdateObj {
			if err := unstructured.SetNestedField(obj.UnstructuredContent(), initContainers, "spec", "jobTemplate", "spec", "template", "spec", "initContainers"); err != nil {
				return nil, errors.Wrap(err, "unable to set item's initContainer image")
			}
		}

	} else {
		//handle containers
		needUpdateObj := false
		containers, _, err := unstructured.NestedSlice(obj.UnstructuredContent(), "spec", "template", "spec", "containers")
		if err != nil {
			a.logger.Infof("UnstructuredConverter meet error: %v", err)
			return nil, errors.Wrap(err, "error getting item's spec.containers")
		}
		for i, container := range containers {
			a.logger.Infoln("container:", container)
			if image, ok := container.(map[string]interface{})["image"]; ok {
				imageName := image.(string)
				if exists, newImageName, err := a.isImageReplaceRuleExist(log, imageName, config); exists && err == nil {
					needUpdateObj = true
					a.logger.Infof("Updating item's image from %s to %s", imageName, newImageName)
					container.(map[string]interface{})["image"] = newImageName
					containers[i] = container
				}
			}
		}

		if needUpdateObj {
			if err := unstructured.SetNestedField(obj.UnstructuredContent(), containers, "spec", "template", "spec", "containers"); err != nil {
				return nil, errors.Wrap(err, "unable to set item's container image")
			}
			a.logger.Infof("obj.Object %v", obj.Object)
		}

		//handle initContainers
		needUpdateObj = false
		initContainers, _, err := unstructured.NestedSlice(obj.UnstructuredContent(), "spec", "template", "spec", "initContainers")
		if err != nil {
			return nil, errors.Wrap(err, "error getting item's spec.initContainers")
		}
		for i, container := range initContainers {
			if image, ok := container.(map[string]interface{})["image"]; ok {
				imageName := image.(string)
				if exists, newImageName, err := a.isImageReplaceRuleExist(log, imageName, config); exists && err == nil {
					needUpdateObj = true
					a.logger.Infof("Updating item's image from %s to %s", imageName, newImageName)
					container.(map[string]interface{})["image"] = newImageName
					initContainers[i] = container
				}
			}
		}

		if needUpdateObj {
			if err := unstructured.SetNestedField(obj.UnstructuredContent(), initContainers, "spec", "template", "spec", "initContainers"); err != nil {
				return nil, errors.Wrap(err, "unable to set item's initContainer image")
			}
		}

	}
	return velero.NewRestoreItemActionExecuteOutput(obj), nil
}

func (a *ChangeImageNameAction) isImageReplaceRuleExist(log *logrus.Entry, oldImageName string, cm *corev1.ConfigMap) (exists bool, newImageName string, err error) {
	if oldImageName == "" {
		log.Infoln("Item has no old image name specified")
		return false, "", nil
	}
	log.Debug("oldImageName: ", oldImageName)

	//how to use: "<old_image_name_sub_part><delimiter><new_image_name_sub_part>"
	//for current implementation the <delimiter> value can only be ","
	//e.x: in case your old image name is 1.1.1.1:5000/abc:test
	//"case1":"1.1.1.1:5000,2.2.2.2:3000"
	//"case2":"5000,3000"
	//"case3":"abc:test,edf:test"
	//"case4":"1.1.1.1:5000/abc:test,2.2.2.2:3000/edf:test"
	for _, row := range cm.Data {
		if !strings.Contains(row, DELIMITER_VALUE) {
			continue
		}
		if strings.Contains(oldImageName, strings.TrimSpace(row[0:strings.Index(row, DELIMITER_VALUE)])) && len(row[strings.Index(row, DELIMITER_VALUE):]) > len(DELIMITER_VALUE) {
			log.Infoln("match specific case:", row)
			oldImagePart := strings.TrimSpace(row[0:strings.Index(row, DELIMITER_VALUE)])
			newImagePart := strings.TrimSpace(row[strings.Index(row, DELIMITER_VALUE)+len(DELIMITER_VALUE):])
			newImageName = strings.Replace(oldImageName, oldImagePart, newImagePart, -1)
			return true, newImageName, nil
		}
	}
	return false, "", errors.Errorf("No mapping rule found for image: %s", oldImageName)
}
