package managedscripts

import (
	"fmt"
	"os"
	"strings"

	"github.com/openshift/osdctl/cmd/promote/git"
	"github.com/openshift/osdctl/cmd/promote/iexec"
	"github.com/openshift/osdctl/cmd/promote/saas"
	"github.com/spf13/cobra"
)

type managedScriptsOptions struct {
	namespaceRef            string
	gitHash                 string
	appInterfaceCheckoutDir string
}

// NewCmdManagedScripts implements the command promoting https://github.com/openshift/managed-scripts
func NewCmdManagedScripts() *cobra.Command {
	ops := &managedScriptsOptions{}
	cmd := &cobra.Command{
		Use:               "managedscripts",
		Short:             "Promote https://github.com/openshift/managed-scripts",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Example: `
		# Promote managed-scripts repo
		osdctl promote managedscripts --gitHash <git-hash>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appInterface := git.BootstrapOsdCtlForAppInterfaceAndServicePromotions(ops.appInterfaceCheckoutDir, iexec.Exec{})

			err := promoteManagedScripts(appInterface, ops.gitHash, ops.namespaceRef)
			if err != nil {
				fmt.Printf("Error while promoting service: %v\n", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "Git hash of the managed-scripts repo commit getting promoted")
	cmd.Flags().StringVarP(&ops.namespaceRef, "namespaceRef", "n", "", "SaaS target namespace reference name")
	cmd.Flags().StringVarP(&ops.appInterfaceCheckoutDir, "appInterfaceDir", "", "", "location of app-interface checkout. Falls back to current working directory")

	return cmd
}

const (
	serviceName = "saas-backplane-api"
)

func promoteManagedScripts(appInterfaceClone git.AppInterface, gitHash string, namespaceRef string) error {
	servicesRegistry, err := saas.GetServicesRegistry(appInterfaceClone)
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
