package saas

import (
	"fmt"

	"github.com/openshift/osdctl/cmd/promote/git"
	"github.com/openshift/osdctl/cmd/promote/iexec"
	"github.com/spf13/cobra"
)

type saasOptions struct {
	list bool

	appInterfaceCheckoutDir string
	serviceName             string
	gitHash                 string
	namespaceRef            string
}

// newCmdSaas implementes the saas command to interact with promoting SaaS services/operators
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
		osdctl promote saas --serviceName <service-name> --gitHash <git-hash> --osd
		or
		osdctl promote saas --serviceName <service-name> --gitHash <git-hash> --hcp`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ops.validateSaasFlow()
			appInterfaceClone := git.BootstrapOsdCtlForAppInterfaceAndServicePromotions(ops.appInterfaceCheckoutDir, iexec.Exec{})
			if ops.list {
				if ops.serviceName != "" || ops.gitHash != "" {
					fmt.Printf("Error: --list cannot be used with any other flags\n\n")

					return cmd.Help()
				}
				return listServiceNames(appInterfaceClone)
			}

			err := servicePromotion(appInterfaceClone, ops.serviceName, ops.gitHash, ops.namespaceRef)
			if err != nil {
				fmt.Printf("Error while promoting service: %v\n", err)
			}

			return nil
		},
	}

	saasCmd.Flags().BoolVarP(&ops.list, "list", "l", false, "List all SaaS services/operators")
	saasCmd.Flags().StringVarP(&ops.serviceName, "serviceName", "", "", "SaaS service/operator getting promoted")
	saasCmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "Git hash of the SaaS service/operator commit getting promoted")
	saasCmd.Flags().StringVarP(&ops.namespaceRef, "namespaceRef", "n", "", "SaaS target namespace reference name")
	saasCmd.Flags().StringVarP(&ops.appInterfaceCheckoutDir, "appInterfaceDir", "", "", "location of app-interface checkout. Falls back to current working directory")

	return saasCmd
}

func (o *saasOptions) validateSaasFlow() {
	if o.serviceName == "" && o.gitHash == "" {
		fmt.Printf("Usage: For SaaS services/operators, please provide --serviceName and (optional) --gitHash\n")
		fmt.Printf("--serviceName is the name of the service, i.e. saas-managed-cluster-config\n")
		fmt.Printf("--gitHash is the target git commit in the service, if not specified defaults to HEAD of master\n\n")
		return
	}
}
