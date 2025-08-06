package utils

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
)

type PromoteCallbacks interface {
	GetRepoURL(service *Service) (string, error)
	GetCurrentGitHash(service *Service, namespaceRef string) (string, error)
	SetGitHash(service *Service, namespaceRef, newGitHash string) error
}

type DefaultPromoteCallbacks struct{}

func (_ *DefaultPromoteCallbacks) GetRepoURL(service *Service) (string, error) {
	return service.GetRepoURL()
}

func (_ *DefaultPromoteCallbacks) GetCurrentGitHash(service *Service, namespaceRef string) (string, error) {
	return service.GetCurrentGitHash(namespaceRef)
}

func (_ *DefaultPromoteCallbacks) SetGitHash(service *Service, namespaceRef, newGitHash string) error {
	return service.SetGitHash(namespaceRef, newGitHash)
}

func promote(callbacks PromoteCallbacks, appInterfaceProvidedPath, serviceName, newGitHash, namespaceRef string) error {
	appInterfaceClone := FindAppInterfaceClone(appInterfaceProvidedPath)

	servicesRegistry, err := GetServicesRegistry(appInterfaceClone)
	if err != nil {
		return err
	}

	service, err := servicesRegistry.GetService(serviceName)
	if err != nil {
		return err
	}

	fmt.Printf("SAAS file                    : %s\n", service.GetSaasFilePath())
	if serviceName != service.GetName() {
		fmt.Printf("Requested service name       : %s\n", serviceName)
		serviceName = service.GetName()
		fmt.Printf("Resolved service name        : %s\n", serviceName)
	}
	fmt.Printf("Service name                 : %s\n", serviceName)

	repoURL, err := callbacks.GetRepoURL(service)
	if err != nil {
		return err
	}

	currentGitHash, err := callbacks.GetCurrentGitHash(service, namespaceRef)
	if err != nil {
		return err
	}

	fmt.Printf("URL of the repo to promote   : %s\n", repoURL)
	fmt.Printf("Repo current hash            : %v\n", currentGitHash)

	serviceRepo, err := GetRepo(repoURL)
	if err != nil {
		return err
	}
	if newGitHash == "" {
		newGitHash, err = serviceRepo.GetHeadHash()

		if err != nil {
			return err
		}
	}

	fmt.Printf("Repo new hash                : %v\n", newGitHash)
	fmt.Println("Promoting service to the new Git hash...")

	changeLog, err := serviceRepo.FormattedLog(currentGitHash, newGitHash)
	if err != nil {
		return err
	}

	branchName := fmt.Sprintf("promote-%s-%s", serviceName, newGitHash)
	err = appInterfaceClone.CheckoutNewBranch(branchName)
	if err != nil {
		return err
	}

	err = callbacks.SetGitHash(service, namespaceRef, newGitHash)
	if err != nil {
		return err
	}
	err = service.Save()
	if err != nil {
		return err
	}

	prefix := "saas-"
	operatorName := strings.TrimPrefix(serviceName, prefix)
	commitMessage := fmt.Sprintf("Promote %s to %s\n\nMonitor rollout status here https://inscope.corp.redhat.com/catalog/default/component/%s/rollout\n\n", serviceName, newGitHash, operatorName)
	commitMessage += fmt.Sprintf("See %s/compare/%s...%s for contents of the promotion. clog:\n\n%s", repoURL, currentGitHash, newGitHash, changeLog)

	saasFileRelPath, err := filepath.Rel(appInterfaceClone.GetPath(), service.GetSaasFilePath())
	if err != nil {
		return err
	}
	err = appInterfaceClone.CommitSaasFile(saasFileRelPath, commitMessage)
	if err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("-------------    Commit message     -------------")
	fmt.Println(commitMessage)
	fmt.Println("------------- End of commit message -------------")
	fmt.Println("")

	fmt.Println("SUCCESS!")
	fmt.Printf("Push the following branch on your fork and create a MR from it: %s\n", branchName)
	fmt.Println("")
	fmt.Printf("(reminder: the push has to be run from the following Git clone: %s)\n", appInterfaceClone.path)

	return nil
}

func Promote(callbacks PromoteCallbacks, appInterfaceProvidedPath, serviceName, newGitHash, namespaceRef string) {
	err := promote(callbacks, appInterfaceProvidedPath, serviceName, newGitHash, namespaceRef)

	if err != nil {
		log.Fatalf("Error while promoting: %v\n", err)
	}
}
