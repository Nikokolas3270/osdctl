package saas

import (
	"fmt"

	"github.com/openshift/osdctl/cmd/promote/utils"
	"github.com/spf13/cobra"
)

type saasOptions struct {
	list bool

	appInterfaceProvidedPath string
	serviceName              string
	gitHash                  string
	namespaceRef             string
}

// NewCmdSaas implementes the saas command to interact with promoting SaaS services/operators
func NewCmdSaas() *cobra.Command {
	ops := &saasOptions{}
	saasCmd := &cobra.Command{
		Use:               "saas",
		Short:             "Utilities to promote SaaS services/operators",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Example: `
		# List all SaaS services/operators
		osdctl promote saas --list

		# Promote a SaaS service/operator
		osdctl promote saas --serviceName <service-name> --gitHash <git-hash>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if ops.list {
				if ops.serviceName != "" || ops.gitHash != "" {
					fmt.Printf("Error: --list cannot be used with any other flags\n\n")

					return cmd.Help()
				}
				return listServiceNames(ops.appInterfaceProvidedPath)
			}

			utils.Promote(&utils.DefaultPromoteCallbacks{}, ops.appInterfaceProvidedPath, ops.serviceName, ops.gitHash, ops.namespaceRef)

			return nil
		},
	}

	saasCmd.Flags().BoolVarP(&ops.list, "list", "l", false, "List all SaaS services/operators")
	saasCmd.Flags().StringVarP(&ops.serviceName, "serviceName", "", "", "SaaS service/operator getting promoted")
	saasCmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "Git hash of the SaaS service/operator commit getting promoted")
	saasCmd.Flags().StringVarP(&ops.namespaceRef, "namespaceRef", "n", "", "SaaS target namespace reference name")
	saasCmd.Flags().StringVarP(&ops.appInterfaceProvidedPath, "appInterfaceDir", "", "", "location of app-interface checkout. Falls back to current working directory")

	return saasCmd
}

func listServiceNames(appInterfaceProvidedPath string) error {
	appInterfaceClone := utils.FindAppInterfaceClone(appInterfaceProvidedPath)
	servicesRegistry, err := utils.GetServicesRegistry(appInterfaceClone)
	if err != nil {
		return err
	}

	fmt.Println("### Available service names ###")
	for _, serviceName := range servicesRegistry.GetServiceNames() {
		fmt.Println(serviceName)
	}

	return nil
}
