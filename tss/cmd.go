package main

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenderly/mantle/l2geth/log"
	"github.com/tenderly/mantle/tss/common"
	"github.com/tenderly/mantle/tss/manager"
	"github.com/tenderly/mantle/tss/node/cmd/tssnode"
)

func main() {
	log.Root().SetHandler(
		log.LvlFilterHandler(
			log.LvlInfo,
			log.StreamHandler(os.Stdout, log.TerminalFormat(true)),
		),
	)

	rootCmd := &cobra.Command{
		Use:   "tss",
		Short: "Tss Start Daemon",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfgFile, _ := cmd.Flags().GetString("config")
			loadedCfg, err := common.LoadConfig(cfgFile)
			if err == nil {
				return common.SetCmdConfig(cmd, loadedCfg)
			} else {
				return nil
			}
		},
	}

	rootCmd.AddCommand(
		manager.Command(),
		tssnode.Command(),
		tssnode.PeerIDCommand(),
	)

	rootCmd.PersistentFlags().StringP("config", "c", "config", "configuration file with extension")

	ctx := context.Background()
	ctx = context.WithValue(ctx, "config", &common.Configuration{})
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
