package git

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift/osdctl/cmd/promote/iexec"

	"github.com/goccy/go-yaml"
)

const (
	canaryStr   = "-prod-canary"
	prodHiveStr = "hivep"
)

type Service struct {
	Name              string `yaml:"name"`
	ResourceTemplates []struct {
		Name    string `yaml:"name"`
		URL     string `yaml:"url"`
		Targets []struct {
			Name       string
			Namespace  map[string]string      `yaml:"namespace"`
			Ref        string                 `yaml:"ref"`
			Parameters map[string]interface{} `yaml:"parameters"`
		} `yaml:"targets"`
	} `yaml:"resourceTemplates"`
}

type AppInterface struct {
	GitDirectory string
	GitExecutor  iexec.IExec
}

type node yaml.Node

type ServiceObj struct {
	saasFilePath   string
	documentNode   *yaml.Node
	rootNode       *node
	name           string
	allTargetNodes []*node
}

func DefaultAppInterfaceDirectory() string {
	return filepath.Join(os.Getenv("HOME"), "git", "app-interface")
}

func BootstrapOsdCtlForAppInterfaceAndServicePromotions(appInterfaceCheckoutDir string, gitExecutor iexec.Exec) AppInterface {
	a := AppInterface{}
	a.GitExecutor = gitExecutor
	if appInterfaceCheckoutDir != "" {
		a.GitDirectory = appInterfaceCheckoutDir
		err := a.checkAppInterfaceCheckout()
		if err != nil {
			log.Fatalf("Provided directory %s is not an AppInterface directory: %v", a.GitDirectory, err)
		}
		return a
	}

	dir, err := getBaseDir(iexec.Exec{})
	if err == nil {
		a.GitDirectory = dir
		err = a.checkAppInterfaceCheckout()
		if err == nil {
			return a
		}
	}

	log.Printf("Not running in AppInterface directory: %v - Trying %s next\n", err, DefaultAppInterfaceDirectory())
	a.GitDirectory = DefaultAppInterfaceDirectory()
	err = a.checkAppInterfaceCheckout()
	if err != nil {
		log.Fatalf("%s is not an AppInterface directory: %v", DefaultAppInterfaceDirectory(), err)
	}

	log.Printf("Found AppInterface in %s.\n", a.GitDirectory)
	return a
}

// checkAppInterfaceCheckout checks if the script is running in the checkout of app-interface
func (a *AppInterface) checkAppInterfaceCheckout() error {
	output, err := a.GitExecutor.Output(a.GitDirectory, "git", "remote", "-v")
	if err != nil {
		return fmt.Errorf("error executing 'git remote -v': %v", err)
	}

	outputString := output

	// Check if the output contains the app-interface repository URL
	if !strings.Contains(outputString, "gitlab.cee.redhat.com") && !strings.Contains(outputString, "app-interface") {
		return fmt.Errorf("not running in checkout of app-interface")
	}

	return nil
}

func (a *AppInterface) UpdateAppInterface(branchName string) error {

	if err := a.GitExecutor.Run(a.GitDirectory, "git", "checkout", "master"); err != nil {
		return fmt.Errorf("failed to checkout master: branch %v", err)
	}

	if err := a.GitExecutor.Run(a.GitDirectory, "git", "branch", "-D", branchName); err != nil {
		fmt.Printf("failed to cleanup branch %s: %v, continuing to create it.\n", branchName, err)
	}

	if err := a.GitExecutor.Run(a.GitDirectory, "git", "checkout", "-b", branchName, "master"); err != nil {
		return fmt.Errorf("failed to create branch %s: %v, does it already exist? If so, please delete it with `git branch -D %s` first", branchName, err, branchName)
	}

	return nil
}

func (a *AppInterface) CommitSaasFile(saasFile, commitMessage string) error {
	// Commit the change
	if err := a.GitExecutor.Run(a.GitDirectory, "git", "add", saasFile); err != nil {
		return fmt.Errorf("failed to add file %s: %v", saasFile, err)
	}
	if err := a.GitExecutor.Run(a.GitDirectory, "git", "commit", "-m", commitMessage); err != nil {
		return fmt.Errorf("failed to commit changes: %v", err)
	}

	return nil
}

func (n *node) getValue(key string) *node {
	if n.Kind == yaml.MappingNode {
		for k := 0; k < len(n.Content)-1; k += 2 {
			keyNode := n.Content[k]
			valueNode := n.Content[k+1]

			if keyNode.Value == key {
				return (*node)(valueNode)
			}
		}
	}

	return nil
}

func (n *node) getStringValue(key string) string {
	nodeValue := n.getValue(key)
	if nodeValue != nil {
		if nodeValue.Kind == yaml.ScalarNode {
			return nodeValue.Value
		}
	}

	return ""
}

func (n *node) getSequenceValue(key string) []*node {
	nodeValue := n.getValue(key)
	if nodeValue != nil {
		if nodeValue.Kind == yaml.SequenceNode {
			sequenceNodes := []*node{}

			for _, rawSequenceNode := range nodeValue.Content {
				sequenceNodes = append(sequenceNodes, (*node)(rawSequenceNode))
			}

			return sequenceNodes
		}
	}

	return []*node{}
}

func CreateServiceObjFromSaasFile(saasFilePath string) (*ServiceObj, error) {
	serviceData, err := os.ReadFile(saasFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SAAS file: %v", err)
	}

	var documentNode yaml.Node

	if err := yaml.Unmarshal(serviceData, &documentNode); err != nil {
		return nil, fmt.Errorf("does not store YAML content: %v", err)
	}

	if len(documentNode.Content) != 1 || documentNode.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("no root YAML node in this SAAS file: %v", saasFilePath)
	}
	rootNode := (*node)(documentNode.Content[0])

	serviceName := rootNode.getStringValue("name")
	if serviceName == "" {
		fmt.Printf("path 'name' is not defined as a non-empty string in this SAAS file: %s\n", saasFilePath)
	}

	serviceTargetNodes := []*node{}
	for _, resourceTemplateNode := range rootNode.getSequenceValue("resourceTemplates") {
		serviceTargetNodes = append(serviceTargetNodes, resourceTemplateNode.getSequenceValue("targets")...)
	}
	if len(serviceTargetNodes) == 0 {
		fmt.Printf("path 'resourceTemplates[].targets' is not defined: %s\n", saasFilePath)
	}

	return &ServiceObj{saasFilePath, &documentNode, rootNode, serviceName, serviceTargetNodes}, nil
}

func (s *ServiceObj) GetRepoURL() (string, error) {
	repoURL := ""

	for _, resourceTemplateNode := range s.rootNode.getSequenceValue("resourceTemplates") {
		resourceTemplateRepoURL := resourceTemplateNode.getStringValue("url")

		if len(repoURL) == 0 {
			repoURL = resourceTemplateRepoURL
		} else if resourceTemplateRepoURL != repoURL {
			return "", fmt.Errorf("path 'resourceTemplates[].url' does have its value set to '%s' "+
				"for all the resource templates in this SAAS file: %s", repoURL, s.saasFilePath)
		}
	}

	return repoURL, nil
}

func (s *ServiceObj) getFilteredTargets(namespaceRef string) []*node {
	filteredTargetsNodes := []*node{}

	if len(s.allTargetNodes) > 0 {
		if namespaceRef == "" {
			serviceNameToDefaultNamespaceRef := map[string]string{
				"saas-configuration-anomaly-detection-db": "app-sre-observability-production-int.yml",
				"saas-configuration-anomaly-detection":    "configuration-anomaly-detection-production",
				"saas-osd-rhobs-rules-and-dashboards":     "production",
				"saas-backplane-api":                      "backplanep",
			}

			namespaceRef = serviceNameToDefaultNamespaceRef[s.name]

			if namespaceRef == "" { // look for canary targets
				for _, targetNode := range s.allTargetNodes {
					if strings.HasSuffix(targetNode.getStringValue("name"), canaryStr) {
						filteredTargetsNodes = append(filteredTargetsNodes, targetNode)
					}
				}

				if len(filteredTargetsNodes) > 0 {
					return filteredTargetsNodes
				}

				fmt.Println("no canary target detected")

				namespaceRef = prodHiveStr
			}
		}

		for _, targetNode := range s.allTargetNodes {
			targetNamespaceNode := targetNode.getValue("namespace")

			if targetNamespaceNode != nil && strings.Contains(targetNamespaceNode.getStringValue("$ref"), namespaceRef) {
				filteredTargetsNodes = append(filteredTargetsNodes, targetNode)
			}
		}

		if len(filteredTargetsNodes) == 0 {
			fmt.Printf("targets in '%s' SAAS file were all filtered out; "+
				"strings in 'resourceTemplates[].targets[].namespace.$ref' path didn't match this regexp: .*%s.*\n",
				s.saasFilePath, namespaceRef)
		}
	}

	return filteredTargetsNodes
}

func (s *ServiceObj) GetCurrentGitHash(namespaceRef string) (string, error) {
	filteredTargetsNodes := s.getFilteredTargets(namespaceRef)
	if len(filteredTargetsNodes) == 0 {
		return "", errors.New("cannot retrieve the current git hash as all targets got filtered out")
	}

	currentGitHash := ""

	for _, targetNode := range filteredTargetsNodes {
		targetGitHash := targetNode.getStringValue("ref")

		if targetGitHash == "" {
			return "", fmt.Errorf("path 'resourceTemplates[].targets[].ref' is not defined as a non-empty string "+
				"for all the retained targets in this SAAS file: %s", s.saasFilePath)
		}

		if len(currentGitHash) == 0 {
			currentGitHash = targetGitHash
		} else if targetGitHash != currentGitHash {
			return "", fmt.Errorf("path 'resourceTemplates[].targets[].ref' does have its value set to '%s' "+
				"for all the retained targets in this SAAS file: %s", currentGitHash, s.saasFilePath)
		}
	}

	return currentGitHash, nil
}

func (s *ServiceObj) SetGitHash(namespaceRef, gitHash string) error {
	filteredTargetsNodes := s.getFilteredTargets(namespaceRef)
	if len(filteredTargetsNodes) == 0 {
		return errors.New("there is no target on which to change the git hash")
	}

	for _, targetNode := range filteredTargetsNodes {
		targetRefNode := targetNode.getValue("ref")

		if targetRefNode == nil {
			return fmt.Errorf("path 'resourceTemplates[].targets[].ref' is not defined "+
				"for all the retained targets in this SAAS file: %s", s.saasFilePath)
		}

		(*yaml.Node)(targetRefNode).SetString(gitHash)
	}

	return nil
}

func (s *ServiceObj) Save() error {
	serviceData, err := yaml.Marshal(s.documentNode)

	if err != nil {
		return err
	}

	if err := os.WriteFile(s.saasFilePath, serviceData, 0600); err != nil {
		return fmt.Errorf("failed to write SAAS file: %v", err)
	}

	return nil
}
