package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/trilogy-group/cloudfix-linter-developer/logger"
)
type StackDetail struct {
	DisplayName string
	AccountId   string
	Region      string
	StackName   string
}

type Manifest struct {
	Version   string              `json:"version"`
	Artifacts map[string]Artifact `json:"artifacts"`
}

type Artifact struct {
	Type         string                     `json:"type"`
	Environment  string                     `json:"environment,omitempty"`
	Properties   map[string]interface{}     `json:"properties,omitempty"`
	Dependencies []string                   `json:"dependencies,omitempty"`
	Metadata     map[string][]MetadataEntry `json:"metadata,omitempty"`
	DisplayName  string                     `json:"displayName,omitempty"`
}

type MetadataEntry struct {
	Type string      `json:"type"`
	Data string `json:"data"`
}

type LogicalIdToPath struct{
	LogicalID string
	Path 	string
}
var LogicalIdsToPath = []LogicalIdToPath{}

func getStacksDataFromJson(jsondata []byte) ([]StackDetail,[]LogicalIdToPath, error) {
	var stacksData []StackDetail
	var manifest Manifest
	err := json.Unmarshal(jsondata, &manifest)
	if err != nil {
		logger.DevLogger().Info("Unable to unmarshell the manifest.json file")
		return nil,nil,errors.New("CANNOTUNMARSHELMANIFEST")
	}

	var accountId = ""
	var stackData StackDetail
	for name, artifact := range manifest.Artifacts {
		if !strings.Contains(name, ".") && !(name == "Tree" && artifact.Type == "cdk:tree") {
			stackData.StackName = name
			stackData.DisplayName = artifact.DisplayName
			if artifact.Environment == "" {
				logger.DevLogger().Error("Stack ", stackData.DisplayName, "is environment agnostic, STACKDATA : ", stackData)
				return []StackDetail{stackData},nil, errors.New("ENVAGNOSTICSTACK")
			}
			envs := strings.Split(artifact.Environment, "/")
			stackData.AccountId = envs[2]
			stackData.Region = envs[3]
			if stackData.AccountId == "unknown-account" || stackData.Region == "unknown-region" {
				logger.DevLogger().Error("Stack ", stackData.DisplayName, "is environment agnostic, STACKDATA : ", stackData)
				return []StackDetail{stackData},nil, errors.New("ENVAGNOSTICSTACK")
			}
			if accountId == "" {
				accountId = stackData.AccountId
			} else {
				if accountId != stackData.AccountId {
					logger.DevLogger().Error("Multiple accounts found in manifest.json. AccountID1:", accountId, " AccountID2:", stackData.AccountId)
					return nil,nil, errors.New("MULTIPLEACCOUNTIDFOUND")
				}
			}
			stacksData = append(stacksData, stackData)
			for path,metaData := range artifact.Metadata{
				if metaData[0].Type == "aws:cdk:logicalId" && metaData[0].Data != "CDKMetadata" && metaData[0].Data != "BootstrapVersion" && metaData[0].Data != "CheckBootstrapVersion" {
					var logicalIdToPath LogicalIdToPath
					logicalIdToPath.LogicalID = metaData[0].Data
					logicalIdToPath.Path = path
					LogicalIdsToPath = append(LogicalIdsToPath,logicalIdToPath )

					fmt.Print("\n Meta data",path," => ",metaData)
				}

				
			}
			fmt.Print("\n metadata ",LogicalIdsToPath)
			

		}

	}
	if len(stacksData) == 0 {
		return nil,nil, errors.New("NOSTACKSFOUND")
	}

	return stacksData, nil,nil
}
func PrepareMappings() error {
	wrkspaceDir,_ := os.Getwd()
	manifestPath := wrkspaceDir + "/.cdkout/manifest.json"
	manifestjson, err := os.Open(manifestPath)
	if err != nil {
		logger.DevLogger().Error("Could not find manifest.json")
		return errors.New("MANIFESTNOTFOUND")
	}
	defer manifestjson.Close()

	byteManifest, err := ioutil.ReadAll(manifestjson)
	if err != nil {
		logger.DevLogger().Error("unable to read manifest.json in .cdkout file in workspacedir : :", wrkspaceDir, "Error: ", err)
		return errors.New("CANNOTREADMANIFEST")
	}
	stacksData,LogicalIdsToPath,err := getStacksDataFromJson(byteManifest)
	fmt.Print("\n StackDATa",stacksData)
	fmt.Print("\n LogicalIdToPat ",LogicalIdsToPath)
	return nil
}
