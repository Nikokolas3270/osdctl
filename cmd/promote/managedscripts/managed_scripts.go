package managedscripts

import (
	"github.com/openshift/osdctl/cmd/promote/utils"
	"github.com/spf13/cobra"
)

type managedScriptsOptions struct {
	namespaceRef             string
	gitHash                  string
	appInterfaceProvidedPath string
}

const (
	serviceName            = "saas-backplane-api"
	repoURL                = "https://github.com/openshift/managed-scripts"
	managedScriptsHashPath = ".parameters.MANAGED_SCRIPTS_GIT_SHA"
)

type promoteCallbacks struct{}

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
			utils.Promote(&promoteCallbacks{}, ops.appInterfaceProvidedPath, serviceName, ops.gitHash, ops.namespaceRef)

			return nil
		},
	}

	cmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "Git hash of the managed-scripts repo commit getting promoted")
	cmd.Flags().StringVarP(&ops.namespaceRef, "namespaceRef", "n", "", "SaaS target namespace reference name")
	cmd.Flags().StringVarP(&ops.appInterfaceProvidedPath, "appInterfaceDir", "", "", "location of app-interface checkout. Falls back to current working directory")

	return cmd
}

func (_ *promoteCallbacks) GetRepoURL(service *utils.Service) (string, error) {
	return repoURL, nil
}

func (_ *promoteCallbacks) GetCurrentGitHash(service *utils.Service, namespaceRef string) (string, error) {
	return service.GetTargetValue(namespaceRef, managedScriptsHashPath)
}

func (_ *promoteCallbacks) SetGitHash(service *utils.Service, namespaceRef, newGitHash string) error {
	return service.SetTargetValue(namespaceRef, managedScriptsHashPath, newGitHash)
}
