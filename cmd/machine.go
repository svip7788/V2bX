package cmd

import (
	"fmt"
	"os"

	"github.com/InazumaV/V2bX/conf"
	"github.com/spf13/cobra"
)

var (
	machineConfigPath string
	machinePanel      string
	machineID         int
	machineToken      string
	machineCore       string
)

var machineCommand = cobra.Command{
	Use:   "machine",
	Short: "Manage machine-mode panels in config (multi-site supported)",
}

var machineSetCommand = cobra.Command{
	Use:   "set",
	Short: "Add or update a panel machine; other panels are kept",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		switch machineCore {
		case "", "auto", "sing", "xray":
		default:
			return fmt.Errorf("--core 只能是 auto/sing/xray")
		}
		token := machineToken
		if token == "" {
			token = os.Getenv("V2BX_MACHINE_TOKEN")
		}
		action, err := conf.UpsertMachine(machineConfigPath, conf.MachineConfig{
			APIHost:   machinePanel,
			MachineID: machineID,
			Token:     token,
			Core:      machineCore,
			Timeout:   30,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "MACHINE_%s %s #%d\n", action, machinePanel, machineID)
		return nil
	},
}

var machineRemoveCommand = cobra.Command{
	Use:   "remove",
	Short: "Remove a panel machine from config",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		found, err := conf.RemoveMachine(machineConfigPath, machinePanel)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("配置中没有面板 %s", machinePanel)
		}
		fmt.Fprintf(os.Stdout, "MACHINE_removed %s\n", machinePanel)
		return nil
	},
}

func init() {
	machineCommand.PersistentFlags().StringVarP(&machineConfigPath, "config", "c", "/etc/V2bX/config.json", "config file path")
	machineCommand.PersistentFlags().StringVar(&machinePanel, "panel", "", "panel url")
	_ = machineCommand.MarkPersistentFlagRequired("panel")
	machineSetCommand.Flags().IntVar(&machineID, "machine-id", 0, "machine id")
	machineSetCommand.Flags().StringVar(&machineToken, "token", "", "machine token (or env V2BX_MACHINE_TOKEN)")
	machineSetCommand.Flags().StringVar(&machineCore, "core", "auto", "auto|sing|xray")
	machineCommand.AddCommand(&machineSetCommand, &machineRemoveCommand)
	command.AddCommand(&machineCommand)
}
