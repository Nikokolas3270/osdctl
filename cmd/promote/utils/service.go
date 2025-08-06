package utils

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

const (
	canaryStr   = "-prod-canary"
	prodHiveStr = "hivep"
)

type Service struct {
	saasFilePath   string
	docNode        *ast.DocumentNode
	rootNode       ast.Node
	name           string
	allTargetNodes []ast.Node
}

func createServiceFromSaasFile(saasFilePath string) (*Service, error) {
	var docNode *ast.DocumentNode
	var rootNode ast.Node

	{
		saasFile, err := parser.ParseFile(saasFilePath, parser.ParseComments)
		if err != nil || saasFile == nil || len(saasFile.Docs) != 1 || saasFile.Docs[0] == nil || saasFile.Docs[0].Body == nil {
			return nil, fmt.Errorf("failed to read or parse '%s' file: %v", saasFilePath, err)
		}

		docNode = saasFile.Docs[0]
		rootNode = docNode.Body
	}

	serviceName := ""

	{
		rootData := struct {
			Name string `yaml:"name"`
		}{}

		if err := yaml.NodeToValue(rootNode, &rootData); err != nil {
			return nil, fmt.Errorf("path 'name' is not defined to a string in '%s': %v", saasFilePath, err)
		}

		serviceName = rootData.Name
	}

	allTargetNodes := []ast.Node{}

	{
		allTargetsPathExpr := "$.resourceTemplates[*].targets[*]"
		allTargetsPath, err := yaml.PathString(allTargetsPathExpr)
		if err != nil {
			return nil, err
		}

		allTargetsNode, err := allTargetsPath.FilterNode(rootNode)
		if err != nil {
			return nil, fmt.Errorf("failed to filter '%s' with '%s': %v", saasFilePath, allTargetsPathExpr, err)
		}

		allTargetsSequenceNode, ok := allTargetsNode.(*ast.SequenceNode)
		if !ok {
			return nil, fmt.Errorf("filter '%s' didn't yields a sequence", allTargetsPathExpr)
		}

		for _, node := range allTargetsSequenceNode.Values {
			allTargetsSubSequenceNode, ok := node.(*ast.SequenceNode)
			if !ok {
				return nil, fmt.Errorf("filter '%s' didn't yields a sequence of sequences", allTargetsPathExpr)
			}
			allTargetNodes = append(allTargetNodes, allTargetsSubSequenceNode.Values...)
		}
	}

	return &Service{saasFilePath, docNode, rootNode, serviceName, allTargetNodes}, nil
}

func (s *Service) GetSaasFilePath() string {
	return s.saasFilePath
}

func (s *Service) GetName() string {
	return s.name
}

func (s *Service) GetRepoURL() (string, error) {
	repoUrlPathExpr := "$.resourceTemplates[*].url"
	path, err := yaml.PathString(repoUrlPathExpr)
	if err != nil {
		return "", err
	}

	urlNodes, err := path.FilterNode(s.rootNode)
	if err != nil {
		return "", fmt.Errorf("failed to filter '%s' with '%s': %v", s.saasFilePath, repoUrlPathExpr, err)
	}

	urls := []string{}

	if err := yaml.NodeToValue(urlNodes, &urls); len(urls) == 0 || err != nil {
		return "", fmt.Errorf("path 'resourceTemplates[].url' is not always defined to a string in '%s' file: %v\n", s.saasFilePath, err)
	}

	url := urls[0]

	for k := 1; k < len(urls); k++ {
		if urls[k] != url {
			return "", fmt.Errorf("path 'resourceTemplates[].url' is not always set to the same value for all resource templates in this file: %s", s.saasFilePath)
		}
	}

	return url, nil
}

func (s *Service) getFilteredTargets(namespaceRef string) []ast.Node {
	filteredTargetsNodes := []ast.Node{}

	if len(s.allTargetNodes) > 0 {
		if namespaceRef == "" {
			serviceNameToDefaultNamespaceRef := map[string]string{
				"saas-configuration-anomaly-detection-db": "app-sre-observability-production-int.yml",
				"saas-configuration-anomaly-detection":    "configuration-anomaly-detection-production",
				"saas-osd-rhobs-rules-and-dashboards":     "production",
				"saas-backplane-api":                      "backplanep",
			}

			namespaceRef = serviceNameToDefaultNamespaceRef[s.name]

			if namespaceRef == "" {
				// look for canary targets
				for _, targetNode := range s.allTargetNodes {
					targetData := struct {
						Name string `yaml:"name"`
					}{}

					if err := yaml.NodeToValue(targetNode, &targetData); err != nil {
						fmt.Printf("path 'resourceTemplates[].targets[].name' is not always defined to a string in '%s' file: %v\n", s.saasFilePath, err)
					}

					if strings.HasSuffix(targetData.Name, canaryStr) {
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

		// look for targets based on their destination / namespace
		for _, targetNode := range s.allTargetNodes {
			targetData := struct {
				Namespace struct {
					Ref string `yaml:"$ref"`
				} `yaml:"namespace"`
			}{}

			if err := yaml.NodeToValue(targetNode, &targetData); targetData.Namespace.Ref == "" || err != nil {
				fmt.Printf("path 'resourceTemplates[].targets[].namespace.$ref' is not always defined to a non-empty string in '%s' file: %v\n", s.saasFilePath, err)
			}

			if strings.Contains(targetData.Namespace.Ref, namespaceRef) {
				filteredTargetsNodes = append(filteredTargetsNodes, targetNode)
			}
		}

		if len(filteredTargetsNodes) == 0 {
			fmt.Printf("targets in '%s' file were all filtered out; "+
				"strings in 'resourceTemplates[].targets[].namespace.$ref' path didn't contain this substring: %s\n",
				s.saasFilePath, namespaceRef)
		}
	}

	return filteredTargetsNodes
}

func (s *Service) getTargetValues(namespaceRef, relValuePath string) ([]*ast.StringNode, error) {
	valuesPath := "resourceTemplates[].targets[]" + relValuePath
	valueNodes := []*ast.StringNode{}
	filteredTargetsNodes := s.getFilteredTargets(namespaceRef)
	if len(filteredTargetsNodes) == 0 {
		return valueNodes, fmt.Errorf("nothing in '%s' path as all targets got filtered out", valuesPath)
	}

	path, err := yaml.PathString("$" + relValuePath)
	if err != nil {
		return valueNodes, err
	}

	for _, targetNode := range filteredTargetsNodes {
		rawValueNode, err := path.FilterNode(targetNode)
		if err != nil {
			return valueNodes, fmt.Errorf("path '%s' does not exist for one of the filtered targets in '%s' : %v", valuesPath, s.saasFilePath, err)
		}

		valueNode, ok := rawValueNode.(*ast.StringNode)
		if !ok || valueNode.Value == "" {
			return valueNodes, fmt.Errorf("path '%s' is not always defined to a non-empty string in in '%s': %v", valuesPath, s.saasFilePath, err)
		}

		valueNodes = append(valueNodes, valueNode)
	}

	return valueNodes, nil
}

func (s *Service) GetTargetValue(namespaceRef, relValuePath string) (string, error) {
	valuesPath := "resourceTemplates[].targets[]" + relValuePath
	valueNodes, err := s.getTargetValues(namespaceRef, relValuePath)

	if err != nil {
		return "", err
	}

	commonValue := ""

	for _, valueNode := range valueNodes {
		if commonValue == "" {
			commonValue = valueNode.Value
		} else if commonValue != valueNode.Value {
			return "", fmt.Errorf("path '%s' does have its value set to '%s' "+
				"for all the retained targets in this file: %s", valuesPath, commonValue, s.saasFilePath)
		}
	}

	return commonValue, nil
}

func (s *Service) SetTargetValue(namespaceRef, relValuePath, value string) error {
	valueNodes, err := s.getTargetValues(namespaceRef, relValuePath)

	if err != nil {
		return err
	}

	for _, valueNode := range valueNodes {
		valueNode.Value = value
	}

	return nil
}

func (s *Service) GetCurrentGitHash(namespaceRef string) (string, error) {
	return s.GetTargetValue(namespaceRef, ".ref")
}

func (s *Service) SetGitHash(namespaceRef, value string) error {
	return s.SetTargetValue(namespaceRef, ".ref", value)
}

func (s *Service) Save() error {
	serviceData, err := s.docNode.MarshalYAML()

	if err != nil {
		return err
	}

	if err := os.WriteFile(s.saasFilePath, serviceData, 0600); err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}

	return nil
}
