package saas

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/openshift/osdctl/cmd/promote/git"
	"github.com/openshift/osdctl/cmd/promote/iexec"
)

const (
	OSDSaasDirPath = "data/services/osd-operators/cicd/saas"
	BPSaasDirPath  = "data/services/backplane/cicd/saas"
	CADSaasDirPath = "data/services/configuration-anomaly-detection/cicd"
)

func listServiceNames(appInterfaceClone git.AppInterface) error {
	servicesRegistry, err := GetServicesRegistry(appInterfaceClone)
	if err != nil {
		return err
	}

	fmt.Println("### Available service names ###")
	for _, serviceName := range servicesRegistry.GetServiceNames() {
		fmt.Println(serviceName)
	}

	return nil
}

func servicePromotion(appInterfaceClone git.AppInterface, serviceName, gitHash string, namespaceRef string) error {
	servicesRegistry, err := GetServicesRegistry(appInterfaceClone)
	if err != nil {
		return err
	}

	serviceName, err = servicesRegistry.ValidateServiceName(serviceName)
	if err != nil {
		return err
	}

	saasFilePath, err := servicesRegistry.GetSaasFilePath(serviceName)
	if err != nil {
		return err
	}
	fmt.Printf("SAAS file: %v\n", saasFilePath)

	serviceObj, err := git.CreateServiceObjFromSaasFile(saasFilePath)
	if err != nil {
		return err
	}

	currentGitHash, err := serviceObj.GetCurrentGitHash(namespaceRef)
	if err != nil {
		return err
	}

	serviceRepo, err := serviceObj.GetRepoURL()
	if err != nil {
		return err
	}

	fmt.Printf("Current git hash: %v\nGit repo: %v\n\n", currentGitHash, serviceRepo)

	promotionGitHash, commitLog, err := git.CheckoutAndCompareGitHash(appInterfaceClone.GitExecutor, serviceRepo, gitHash, currentGitHash)
	if err != nil {
		return fmt.Errorf("failed to checkout and compare git hash: %v", err)
	} else if promotionGitHash == "" {
		fmt.Printf("Unable to find a git hash to promote. Exiting.\n")
		os.Exit(6)
	}
	fmt.Printf("Service: %s will be promoted to %s\n", serviceName, promotionGitHash)

	branchName := fmt.Sprintf("promote-%s-%s", serviceName, promotionGitHash)
	err = appInterfaceClone.UpdateAppInterface(branchName)
	if err != nil {
		fmt.Printf("FAILURE: %v\n", err)
	}

	err = serviceObj.SetGitHash(namespaceRef, promotionGitHash)
	if err != nil {
		fmt.Printf("FAILURE: %v\n", err)
	}
	err = serviceObj.Save()
	if err != nil {
		fmt.Printf("FAILURE: %v\n", err)
	}

	prefix := "saas-"
	operatorName := strings.TrimPrefix(serviceName, prefix)
	commitMessage := fmt.Sprintf("Promote %s to %s\n\nMonitor rollout status here https://inscope.corp.redhat.com/catalog/default/component/%s/rollout\n\n", serviceName, promotionGitHash, operatorName)
	commitMessage += fmt.Sprintf("See %s/compare/%s...%s for contents of the promotion. clog:\n\n%s", serviceRepo, currentGitHash, promotionGitHash, commitLog)

	// ovverriding appInterface.GitExecuter to iexec.Exec{}
	appInterfaceClone.GitExecutor = iexec.Exec{}
	err = appInterfaceClone.CommitSaasFile(saasFilePath, commitMessage)
	if err != nil {
		return fmt.Errorf("failed to commit changes to app-interface: %w", err)
	}
	fmt.Printf("commitMessage: %s\n", commitMessage)

	fmt.Printf("The branch %s is ready to be pushed\n", branchName)
	fmt.Println("")
	fmt.Println("service:", serviceName)
	fmt.Println("from:", currentGitHash)
	fmt.Println("to:", promotionGitHash)
	fmt.Println("READY TO PUSH,", serviceName, "promotion commit is ready locally")
	return nil
}

type ServicesRegistry struct {
	serviceNameToSaasFilePath map[string]string
}

func GetServicesRegistry(appInterfaceClone git.AppInterface) (*ServicesRegistry, error) {
	baseDirPath := appInterfaceClone.GitDirectory
	saasRelDirPaths := []string{OSDSaasDirPath, BPSaasDirPath, CADSaasDirPath}
	serviceNameToFilePath := make(map[string]string)
	isFile := func(fileName string) bool {
		if fileInfo, err := os.Stat(fileName); err == nil {
			return fileInfo.Mode().IsRegular()
		}
		return false
	}

	for _, saasRelDirPath := range saasRelDirPaths {
		saasDirPath := filepath.Join(baseDirPath, saasRelDirPath)
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

func (r *ServicesRegistry) ValidateServiceName(serviceName string) (string, error) {
	fmt.Printf("### Checking if service %s exists ###\n", serviceName)

	for _, candidateServiceName := range []string{serviceName, "saas-" + serviceName} {
		if _, ok := r.serviceNameToSaasFilePath[candidateServiceName]; ok {
			fmt.Printf("Service %s found\n", candidateServiceName)
			return candidateServiceName, nil
		}
	}

	return serviceName, fmt.Errorf("service %s not found", serviceName)
}

func (r *ServicesRegistry) GetSaasFilePath(serviceName string) (string, error) {
	if saasFilePath, ok := r.serviceNameToSaasFilePath[serviceName]; ok {
		return saasFilePath, nil
	}

	return "", fmt.Errorf("saas directory for service %s not found", serviceName)
}
