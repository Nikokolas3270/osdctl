package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/stretchr/testify/assert"
)

const (
	saasFileDefaultCommonContent = `
resourceTemplates:
  - name: stage
    url: @repoURL@
    targets:
    - name: hives01
      namespace:
        $ref: /services/osd-operators/namespaces/hives01/cluster-scope.yml
      ref: master
  - name: prod
    url: @repoURL@
    targets:
    - name: hivep01
      namespace:
        $ref: /services/osd-operators/namespaces/hivep01/cluster-scope.yml
      ref: @gitHash@
    - name: hivep02
      namespace:
        $ref: /services/osd-operators/namespaces/hivep02/cluster-scope.yml
      ref: @gitHash@`

	service1SaasFileRelPath = "data/services/osd-operators/cicd/saas/saas-service-1.yaml"
	bpApiSaasFileRelPath    = "data/services/backplane/cicd/saas/saas-backplane-api.yaml"
)

func getSaasFileDefaultContent(serviceName string) string {
	return fmt.Sprintf("name: %s", serviceName) + saasFileDefaultCommonContent
}

var appInterfaceContent = map[string]string{
	service1SaasFileRelPath: getSaasFileDefaultContent("saas-service-1"),

	bpApiSaasFileRelPath: `
name: saas-backplane-api
resourceTemplates:
  - name: backplane-api
    url: @repoURL@
    targets:
    - namespace:
        $ref: /services/backplane/namespaces/backplane-prod-backplanep04uw2.yml
      ref: @gitHash@
      parameters:
        MANAGED_SCRIPTS_GIT_SHA: @managedScriptsGitHash@
    - namespace:
        $ref: /services/backplane/namespaces/backplane-prod-backplanep05ue1.yml
      ref: @gitHash@
      parameters:
        MANAGED_SCRIPTS_GIT_SHA: @managedScriptsGitHash@`,

	"data/services/osd-operators/cicd/saas/service-without-saas-suffix.yaml": getSaasFileDefaultContent("service-without-saas-suffix"),
	"data/services/hcm-ai/cicd/saas-dashboards.yaml":                         "",
}

var defaultSignature = object.Signature{
	Name:  "John Doe",
	Email: "john.doe@company.com",
}

type testData struct {
	testRepoPath         string
	testRepoHashes       [10]string
	appInterfacePath     string
	service1SaasFilePath string
}

func (d *testData) GenerateSaasFileContent(saasFileRelPath string, hashIndex, managedScriptsHashIndex int) string {
	saasFileContent := appInterfaceContent[saasFileRelPath]

	saasFileContent = strings.ReplaceAll(saasFileContent, "@repoURL@", d.testRepoPath)
	saasFileContent = strings.ReplaceAll(saasFileContent, "@gitHash@", d.testRepoHashes[hashIndex])
	saasFileContent = strings.ReplaceAll(saasFileContent, "@managedScriptsGitHash@", d.testRepoHashes[managedScriptsHashIndex])

	return saasFileContent
}

func (d *testData) ReadSaasFileContent(saasFileRelPath string) string {
	saasFilePath := filepath.Join(d.appInterfacePath, saasFileRelPath)

	saasFileContent, err := os.ReadFile(saasFilePath)
	if err != nil {
		return err.Error()
	}

	return string(saasFileContent)
}

const commitTemplate = `commit %s
Author: John Doe <john.doe@company.com>
Date:   Thu Jan 01 00:00:00 1970 +0000

    Commit #%d
`

func (d *testData) TestRepoFormattedLog(startHashIndex, endHashIndex int) string {
	var sb strings.Builder

	for idx := endHashIndex; idx >= startHashIndex; idx-- {
		sb.WriteString(fmt.Sprintf(commitTemplate, d.testRepoHashes[idx], idx))
	}

	return sb.String()
}

func createTestData(t *testing.T) *testData {
	rootPath := t.TempDir()
	data := testData{
		testRepoPath:     filepath.Join(rootPath, "repo"),
		appInterfacePath: filepath.Join(rootPath, "app-interface"),
	}

	// Initializing test repo

	err := os.Mkdir(data.testRepoPath, 0700)
	assert.NoError(t, err)

	testRepo, err := git.PlainInit(data.testRepoPath, false)
	assert.NoError(t, err)
	assert.NotNil(t, testRepo)

	testWorkTree, err := testRepo.Worktree()
	assert.NoError(t, err)
	assert.NotNil(t, testWorkTree)

	for k := 0; k < 10; k++ {
		hash, err := testWorkTree.Commit(fmt.Sprintf("Commit #%d", k), &git.CommitOptions{
			AllowEmptyCommits: true,
			Author:            &defaultSignature,
		})
		assert.NoError(t, err)

		data.testRepoHashes[k] = hash.String()
	}

	// Initializing app-interface clone

	err = os.Mkdir(data.appInterfacePath, 0700)
	assert.NoError(t, err)

	appInterfaceRepo, err := git.PlainInit(data.appInterfacePath, false)
	assert.NoError(t, err)
	assert.NotNil(t, appInterfaceRepo)

	appInterfaceRepoConfig := config.NewConfig()
	assert.NotNil(t, appInterfaceRepoConfig)
	appInterfaceRepoConfig.Author.Name = defaultSignature.Name
	appInterfaceRepoConfig.Author.Email = defaultSignature.Email
	err = appInterfaceRepo.SetConfig(appInterfaceRepoConfig)
	assert.NoError(t, err)

	appInterfaceWorkTree, err := appInterfaceRepo.Worktree()
	assert.NoError(t, err)
	assert.NotNil(t, appInterfaceWorkTree)

	for saasFileRelPath := range appInterfaceContent {
		saasFilePath := filepath.Join(data.appInterfacePath, saasFileRelPath)
		saasDirPath := filepath.Dir(saasFilePath)

		err = os.MkdirAll(saasDirPath, 0700)
		assert.NoError(t, err)

		saasFileContent := data.GenerateSaasFileContent(saasFileRelPath, 0, 1)

		err = os.WriteFile(saasFilePath, []byte(saasFileContent), 0600)
		assert.NoError(t, err)
	}

	data.service1SaasFilePath = filepath.Join(data.appInterfacePath, service1SaasFileRelPath)

	err = appInterfaceWorkTree.AddGlob("*")
	assert.NoError(t, err)

	_, err = appInterfaceWorkTree.Commit("Initial commit", &git.CommitOptions{})
	assert.NoError(t, err)

	_, err = appInterfaceRepo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"git@gitlab.cee.redhat.com:service/app-interface.git"},
	})
	assert.NoError(t, err)

	return &data
}

func TestFindAppInterfaceClone(t *testing.T) {
	data := createTestData(t)

	// Call tested here
	appInterfaceClone := FindAppInterfaceClone(data.appInterfacePath)

	assert.NotNil(t, appInterfaceClone)
	assert.Equal(t, data.appInterfacePath, appInterfaceClone.path)
	assert.Equal(t, data.appInterfacePath, appInterfaceClone.GetPath())
}

func TestFindAppInterfaceCloneInCwd(t *testing.T) {
	data := createTestData(t)

	err := os.Chdir(data.appInterfacePath)
	assert.NoError(t, err)

	// Call tested here
	appInterfaceClone := FindAppInterfaceClone("")

	assert.NotNil(t, appInterfaceClone)

	expectedAppInterfacePath, err := filepath.EvalSymlinks(data.appInterfacePath)
	assert.NoError(t, err)
	assert.Equal(t, expectedAppInterfacePath, appInterfaceClone.path)
}

func TestFindAppInterfaceCloneInCwdParentDir(t *testing.T) {
	data := createTestData(t)

	err := os.Chdir(filepath.Join(data.appInterfacePath, "data"))
	assert.NoError(t, err)

	// Call tested here
	appInterfaceClone := FindAppInterfaceClone("")

	assert.NotNil(t, appInterfaceClone)

	expectedAppInterfacePath, err := filepath.EvalSymlinks(data.appInterfacePath)
	assert.NoError(t, err)
	assert.Equal(t, expectedAppInterfacePath, appInterfaceClone.path)
}

func getAppInterfaceHeadCommit(t *testing.T, data *testData, expectedBranchName string) *object.Commit {
	appInterfaceRepo, err := git.PlainOpen(data.appInterfacePath)
	assert.NoError(t, err)
	assert.NotNil(t, appInterfaceRepo)

	appInterfaceWorkTree, err := appInterfaceRepo.Worktree()
	assert.NoError(t, err)
	assert.NotNil(t, appInterfaceWorkTree)

	appInterfaceStatus, err := appInterfaceWorkTree.Status()
	assert.NoError(t, err)
	assert.True(t, appInterfaceStatus.IsClean())

	appInterfaceHead, err := appInterfaceRepo.Head()
	assert.NoError(t, err)
	assert.NotNil(t, appInterfaceHead)
	assert.Equal(t, "refs/heads/"+expectedBranchName, appInterfaceHead.Name().String())

	appInterfaceHeadCommit, err := appInterfaceRepo.CommitObject(appInterfaceHead.Hash())
	assert.NoError(t, err)
	assert.NotNil(t, appInterfaceHeadCommit)

	return appInterfaceHeadCommit
}

func TestAppInterfaceCloneCheckoutNewBranch(t *testing.T) {
	data := createTestData(t)

	appInterfaceClone := FindAppInterfaceClone(data.appInterfacePath)
	assert.NotNil(t, appInterfaceClone)

	// Call tested here
	err := appInterfaceClone.CheckoutNewBranch("test-branch")

	assert.NoError(t, err)

	commit := getAppInterfaceHeadCommit(t, data, "test-branch")
	assert.Equal(t, "Initial commit", commit.Message)
}

func TestAppInterfaceCloneCheckoutNewBranchWhileBranchAlreadyExist(t *testing.T) {
	data := createTestData(t)

	// Creating the branch
	appInterfaceRepo, err := git.PlainOpen(data.appInterfacePath)
	assert.NoError(t, err)
	assert.NotNil(t, appInterfaceRepo)

	appInterfaceWorkTree, err := appInterfaceRepo.Worktree()
	assert.NoError(t, err)
	assert.NotNil(t, appInterfaceWorkTree)

	err = appInterfaceWorkTree.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("test-branch"), Create: true})
	assert.NoError(t, err)

	appInterfaceClone := FindAppInterfaceClone(data.appInterfacePath)
	assert.NotNil(t, appInterfaceClone)

	// Call tested here
	err = appInterfaceClone.CheckoutNewBranch("test-branch")

	assert.NoError(t, err)

	commit := getAppInterfaceHeadCommit(t, data, "test-branch")
	assert.Equal(t, "Initial commit", commit.Message)
}

func TestGetServicesRegistry(t *testing.T) {
	data := createTestData(t)

	appInterfaceClone := FindAppInterfaceClone(data.appInterfacePath)
	assert.NotNil(t, appInterfaceClone)

	// Call tested here
	servicesRegistry, err := GetServicesRegistry(appInterfaceClone)

	assert.NoError(t, err)
	assert.NotNil(t, servicesRegistry)

	assert.Equal(t,
		len(appInterfaceContent)-2, // Minus the paths which are not considered
		len(servicesRegistry.serviceNameToSaasFilePath))
}

func getServiceRegistry(t *testing.T, data *testData) *ServicesRegistry {
	appInterfaceClone := FindAppInterfaceClone(data.appInterfacePath)
	assert.NotNil(t, appInterfaceClone)

	servicesRegistry, err := GetServicesRegistry(appInterfaceClone)
	assert.NoError(t, err)
	assert.NotNil(t, servicesRegistry)

	return servicesRegistry
}

func TestGetServiceNames(t *testing.T) {
	data := createTestData(t)
	servicesRegistry := getServiceRegistry(t, data)

	// Call tested here
	serviceNames := servicesRegistry.GetServiceNames()

	assert.Equal(t, []string{
		"saas-backplane-api", "saas-service-1",
	}, serviceNames)
}

func TestGetService1(t *testing.T) {
	data := createTestData(t)
	servicesRegistry := getServiceRegistry(t, data)

	// Call tested here
	service, err := servicesRegistry.GetService("saas-service-1")

	assert.NoError(t, err)
	assert.NotNil(t, service)

	assert.Equal(t, data.service1SaasFilePath, service.saasFilePath)
	assert.Equal(t, "saas-service-1", service.name)
}

func TestGetService1WithoutSaasPrefix(t *testing.T) {
	data := createTestData(t)
	servicesRegistry := getServiceRegistry(t, data)

	// Call tested here
	service, err := servicesRegistry.GetService("service-1")

	assert.NoError(t, err)
	assert.NotNil(t, service)

	assert.Equal(t, data.service1SaasFilePath, service.saasFilePath)
	assert.Equal(t, service.name, "saas-service-1")
}

func getService(t *testing.T, data *testData, serviceName string) *Service {
	servicesRegistry := getServiceRegistry(t, data)

	service, err := servicesRegistry.GetService("saas-service-1")
	assert.NoError(t, err)
	assert.NotNil(t, service)

	return service
}

func getService1(t *testing.T, data *testData) *Service {
	return getService(t, data, "saas-service-1")
}

func TestServiceGetters(t *testing.T) {
	data := createTestData(t)
	service := getService1(t, data)

	// Getters tested here
	assert.Equal(t, data.service1SaasFilePath, service.GetSaasFilePath())
	assert.Equal(t, "saas-service-1", service.GetName())

	repoURL, err := service.GetRepoURL()
	assert.NoError(t, err)
	assert.Equal(t, data.testRepoPath, repoURL)

	currentGitHash, err := service.GetCurrentGitHash("")
	assert.NoError(t, err)
	assert.Equal(t, data.testRepoHashes[0], currentGitHash)
}

func TestServiceSetGitHashAndSave(t *testing.T) {
	data := createTestData(t)
	service := getService1(t, data)

	// Setter tested here
	err := service.SetGitHash("", data.testRepoHashes[7])
	assert.NoError(t, err)

	// Save tested here
	err = service.Save()
	assert.NoError(t, err)

	assert.Equal(t, data.GenerateSaasFileContent(service1SaasFileRelPath, 7, 0), data.ReadSaasFileContent(service1SaasFileRelPath))
}

func TestGetRepo(t *testing.T) {
	data := createTestData(t)

	// Call tested here
	testRepo, err := GetRepo(data.testRepoPath)

	assert.NoError(t, err)
	assert.NotNil(t, testRepo)

	assert.Equal(t, data.testRepoPath, testRepo.url)
	assert.NotNil(t, testRepo.rawRepo)
}

func TestGitRepoGetHeadHash(t *testing.T) {
	data := createTestData(t)
	testRepo, err := GetRepo(data.testRepoPath)

	assert.NoError(t, err)
	assert.NotNil(t, testRepo)

	// Getter tested here
	headHash, err := testRepo.GetHeadHash()
	assert.NoError(t, err)
	assert.Equal(t, headHash, data.testRepoHashes[9])
}

func TestGitRepoFormattedLog(t *testing.T) {
	data := createTestData(t)
	testRepo, err := GetRepo(data.testRepoPath)

	assert.NoError(t, err)
	assert.NotNil(t, testRepo)

	// Call tested here
	log, err := testRepo.FormattedLog(data.testRepoHashes[2], data.testRepoHashes[8])
	assert.NoError(t, err)
	assert.Equal(t, data.TestRepoFormattedLog(3, 8), log)
}

func checkPromoteCommit(t *testing.T, data *testData, serviceName string, oldHashIndex, newHashIndex int) {
	branchName := fmt.Sprintf("promote-%s-%s", serviceName, data.testRepoHashes[newHashIndex])
	commit := getAppInterfaceHeadCommit(t, data, branchName)

	assert.Contains(t, commit.Message, data.TestRepoFormattedLog(oldHashIndex+1, newHashIndex))
}

func TestPromote(t *testing.T) {
	data := createTestData(t)

	// Call tested here
	err := promote(&DefaultPromoteCallbacks{}, data.appInterfacePath, "saas-service-1", data.testRepoHashes[4], "")

	assert.NoError(t, err)

	assert.Equal(t, data.GenerateSaasFileContent(service1SaasFileRelPath, 4, 0), data.ReadSaasFileContent(service1SaasFileRelPath))
	checkPromoteCommit(t, data, "saas-service-1", 0, 4)
}
