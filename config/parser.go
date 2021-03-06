/* walter: a deployment pipeline template
 * Copyright (C) 2014 Recruit Technologies Co., Ltd. and contributors
 * (see CONTRIBUTORS.md)
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package config

import (
	"container/list"
	"fmt"
	"reflect"
	"strings"

	"github.com/recruit-tech/walter/log"
	"github.com/recruit-tech/walter/messengers"
	"github.com/recruit-tech/walter/pipelines"
	"github.com/recruit-tech/walter/services"
	"github.com/recruit-tech/walter/stages"
)

func getStageTypeModuleName(stageType string) string {
	return strings.ToLower(stageType)
}

func Parse(configData *map[interface{}]interface{}) (*pipelines.Resources, error) {
	envs := NewEnvVariables()
	return ParseWithSpecifiedEnvs(configData, envs)
}

// TODO: make parser process a struct (for simplifying redundant functions and reducing the number of function parameters)
func ParseWithSpecifiedEnvs(configData *map[interface{}]interface{},
	envs *EnvVariables) (*pipelines.Resources, error) {
	// parse service block
	serviceOps, ok := (*configData)["service"].(map[interface{}]interface{})
	var repoService services.Service
	var err error
	if ok == true {
		log.Info("found \"service\" block")
		repoService, err = mapService(serviceOps, envs)
		if err != nil {
			return nil, err
		}
	} else {
		log.Info("not found \"service\" block")
		repoService, err = services.InitService("local")
		if err != nil {
			return nil, err
		}
	}

	// parse messenger block
	messengerOps, ok := (*configData)["messenger"].(map[interface{}]interface{})
	var messenger messengers.Messenger
	if ok == true {
		log.Info("found messenger block")
		messenger, err = mapMessenger(messengerOps, envs)
		if err != nil {
			return nil, err
		}
	} else {
		log.Info("not found messenger block")
		messenger, err = messengers.InitMessenger("fake")
		if err != nil {
			return nil, err
		}
	}

	// parse cleanup block
	var cleanup *pipelines.Pipeline = &pipelines.Pipeline{}
	cleanupData, ok := (*configData)["cleanup"].([]interface{})
	if ok == true {
		log.Info("found cleanup block")
		cleanupList, err := convertYamlMapToStages(cleanupData, envs)
		if err != nil {
			return nil, err
		}
		for stageItem := cleanupList.Front(); stageItem != nil; stageItem = stageItem.Next() {
			cleanup.AddStage(stageItem.Value.(stages.Stage))
		}
	} else {
		log.Info("not found cleanup block in the input file")
	}

	// parse pipeline block
	var pipeline *pipelines.Pipeline = &pipelines.Pipeline{}

	pipelineData, ok := (*configData)["pipeline"].([]interface{})
	if ok == false {
		return nil, fmt.Errorf("no pipeline block in the input file")
	}
	stageList, err := convertYamlMapToStages(pipelineData, envs)
	if err != nil {
		return nil, err
	}
	for stageItem := stageList.Front(); stageItem != nil; stageItem = stageItem.Next() {
		pipeline.AddStage(stageItem.Value.(stages.Stage))
	}
	var resources = &pipelines.Resources{Pipeline: pipeline, Cleanup: cleanup, Reporter: messenger, RepoService: repoService}

	return resources, nil
}

func mapMessenger(messengerMap map[interface{}]interface{}, envs *EnvVariables) (messengers.Messenger, error) {
	messengerType := messengerMap["type"].(string)
	log.Info("type of reporter is " + messengerType)
	messenger, err := messengers.InitMessenger(messengerType)
	if err != nil {
		return nil, err
	}
	newMessengerValue := reflect.ValueOf(messenger).Elem()
	newMessengerType := reflect.TypeOf(messenger).Elem()
	for i := 0; i < newMessengerType.NumField(); i++ {
		tagName := newMessengerType.Field(i).Tag.Get("config")
		for messengerOptKey, messengerOptVal := range messengerMap {
			if tagName == messengerOptKey {
				fieldVal := newMessengerValue.Field(i)
				if fieldVal.Type() == reflect.ValueOf("string").Type() {
					fieldVal.SetString(envs.Replace(messengerOptVal.(string)))
				}
			}
		}
	}

	return messenger, nil
}

func mapService(serviceMap map[interface{}]interface{}, envs *EnvVariables) (services.Service, error) {
	serviceType := serviceMap["type"].(string)
	log.Info("type of service is " + serviceType)
	service, err := services.InitService(serviceType)
	if err != nil {
		return nil, err
	}

	newServiceValue := reflect.ValueOf(service).Elem()
	newServiceType := reflect.TypeOf(service).Elem()
	for i := 0; i < newServiceType.NumField(); i++ {
		tagName := newServiceType.Field(i).Tag.Get("config")
		for serviceOptKey, serviceOptVal := range serviceMap {
			if tagName == serviceOptKey {
				fieldVal := newServiceValue.Field(i)
				if fieldVal.Type() == reflect.ValueOf("string").Type() {
					fieldVal.SetString(envs.Replace(serviceOptVal.(string)))
				}
			}
		}
	}
	return service, nil
}

func convertYamlMapToStages(yamlStageList []interface{}, envs *EnvVariables) (*list.List, error) {
	stages := list.New()
	for _, stageDetail := range yamlStageList {
		stage, err := mapStage(stageDetail.(map[interface{}]interface{}), envs)
		if err != nil {
			return nil, err
		}
		stages.PushBack(stage)
	}
	return stages, nil
}

func mapStage(stageMap map[interface{}]interface{}, envs *EnvVariables) (stages.Stage, error) {
	log.Debugf("%v", stageMap["run_after"])

	var stageType string = "command"
	if stageMap["type"] != nil {
		stageType = stageMap["type"].(string)
	} else if stageMap["stage_type"] != nil {
		log.Warn("found property \"stage_type\"")
		log.Warn("property \"stage_type\" is deprecated. please use \"type\" instead.")
		stageType = stageMap["stage_type"].(string)
	}
	stage, err := stages.InitStage(stageType)
	if err != nil {
		return nil, err
	}
	newStageValue := reflect.ValueOf(stage).Elem()
	newStageType := reflect.TypeOf(stage).Elem()

	if stageName := stageMap["name"]; stageName != nil {
		stage.SetStageName(stageMap["name"].(string))
	} else if stageName := stageMap["stage_name"]; stageName != nil {
		log.Warn("found property \"stage_name\"")
		log.Warn("property \"stage_name\" is deprecated. please use \"stage\" instead.")
		stage.SetStageName(stageMap["stage_name"].(string))
	}

	stageOpts := stages.NewStageOpts()

	if reportingFullOutput := stageMap["report_full_output"]; reportingFullOutput != nil {
		stageOpts.ReportingFullOutput = true
	}

	stage.SetStageOpts(*stageOpts)

	for i := 0; i < newStageType.NumField(); i++ {
		tagName := newStageType.Field(i).Tag.Get("config")
		is_replace := newStageType.Field(i).Tag.Get("is_replace")
		for stageOptKey, stageOptVal := range stageMap {
			if tagName == stageOptKey {
				if stageOptVal == nil {
					log.Warnf("stage option \"%s\" is not specified", stageOptKey)
				} else {
					setFieldVal(newStageValue.Field(i), stageOptVal, is_replace, envs)
				}
			}
		}
	}

	if runAfters := stageMap["run_after"]; runAfters != nil {
		for _, runAfter := range runAfters.([]interface{}) {
			childStage, err := mapStage(runAfter.(map[interface{}]interface{}), envs)
			if err != nil {
				return nil, err
			}
			stage.AddChildStage(childStage)
		}
	}
	return stage, nil
}

func setFieldVal(fieldVal reflect.Value, stageOptVal interface{}, is_replace string, envs *EnvVariables) {
	if fieldVal.Type() == reflect.ValueOf("string").Type() {
		if is_replace == "true" {
			fieldVal.SetString(envs.Replace(stageOptVal.(string)))
		} else {
			fieldVal.SetString(stageOptVal.(string))
		}
	}
}
