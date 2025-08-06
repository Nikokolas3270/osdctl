package utils

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	OSDSaasDirPath = "data/services/osd-operators/cicd/saas"
	BPSaasDirPath  = "data/services/backplane/cicd/saas"
	CADSaasDirPath = "data/services/configuration-anomaly-detection/cicd"
)

type ServicesRegistry struct {
	serviceNameToSaasFilePath map[string]string
}

func GetServicesRegistry(appInterfaceClone *AppInterfaceClone) (*ServicesRegistry, error) {
	saasRelDirPaths := []string{OSDSaasDirPath, BPSaasDirPath, CADSaasDirPath}
	serviceNameToFilePath := make(map[string]string)
	isFile := func(fileName string) bool {
		if fileInfo, err := os.Stat(fileName); err == nil {
			return fileInfo.Mode().IsRegular()
		}
		return false
	}

	for _, saasRelDirPath := range saasRelDirPaths {
		saasDirPath := filepath.Join(appInterfaceClone.path, saasRelDirPath)
		saasPaths, err := filepath.Glob(filepath.Join(saasDirPath, "saas-*"))
		if err != nil {
			return nil, err
		}
		for _, saasPath := range saasPaths {
			serviceName := strings.TrimSuffix(filepath.Base(saasPath), filepath.Ext(saasPath))

			if strings.HasSuffix(saasPath, ".yaml") && isFile(saasPath) {
				serviceNameToFilePath[serviceName] = saasPath
			} else {
				for _, fileName := range []string{"deploy.yaml", "hypershift-deploy.yaml"} {
					filePath := filepath.Join(saasPath, fileName)
					if isFile(filePath) {
						serviceNameToFilePath[serviceName] = filePath
						break
					}
				}
			}
		}
	}

	return &ServicesRegistry{serviceNameToFilePath}, nil
}

func (r *ServicesRegistry) GetServiceNames() []string {
	return slices.Sorted(maps.Keys(r.serviceNameToSaasFilePath))
}

func (r *ServicesRegistry) GetService(requestedServiceName string) (*Service, error) {
	for _, qualifiedServiceName := range []string{requestedServiceName, "saas-" + requestedServiceName} {
		if saasFilePath, ok := r.serviceNameToSaasFilePath[qualifiedServiceName]; ok {
			return createServiceFromSaasFile(saasFilePath)
		}
	}

	return nil, fmt.Errorf("no SAAS file for the following service: %s", requestedServiceName)
}
