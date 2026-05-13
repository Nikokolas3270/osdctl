package rhobs

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newCmdAlerts() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alerts",
		Short: "List oe silence RHOBS alerts",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(newCmdAlertsGet())

	return cmd
}

func newCmdAlertsGet() *cobra.Command {
	var outputFormatStr string
	var isPrintingClusterResultsOnly bool

	cmd := &cobra.Command{
		Use:           "get",
		Short:         "List alerts from RHOBS for a given cluster",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			rhobsFetcher, err := CreateRhobsFetcher(commonOptions.clusterId, RhobsFetchForMetrics, commonOptions.hiveOcmUrl)
			if err != nil {
				log.Errorf("Error while computing metrics RHOBS cell: %v", err)
			}

			err = rhobsFetcher.PrintAlerts()
			if err != nil {
				return fmt.Errorf("failed to print alerts: %v", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFormatStr, "output", "o", string(MetricsFormatTable), `Format of the output - allowed values: "table", "csv" or "json"`)
	cmd.Flags().BoolVarP(&isPrintingClusterResultsOnly, "filter", "f", false, "Only keep the results matching the given cluster - "+
		"only effective if some of those results have a _id, _mc_id or mc_name label")

	return cmd
}
