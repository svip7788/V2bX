package cmd

import (
	"github.com/InazumaV/V2bX/conf"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	config      string
	watch       bool
	childNodeID int
)

var serverCommand = cobra.Command{
	Use:   "server",
	Short: "Run V2bX server",
	Run:   serverHandle,
	Args:  cobra.NoArgs,
}

func init() {
	serverCommand.PersistentFlags().
		StringVarP(&config, "config", "c",
			"/etc/V2bX/config.json", "config file path")
	serverCommand.PersistentFlags().
		BoolVarP(&watch, "watch", "w",
			true, "watch file path change")
	serverCommand.Flags().
		IntVar(&childNodeID, "child-node-id", 0, "internal child node id")
	_ = serverCommand.Flags().MarkHidden("child-node-id")
	command.AddCommand(&serverCommand)
}

func serverHandle(_ *cobra.Command, _ []string) {
	if childNodeID == 0 {
		showVersion()
	}
	c := conf.New()
	err := c.LoadFromPath(config)
	if err != nil {
		log.WithField("err", err).Error("Load config file failed")
		return
	}

	cleanup, err := configureServerLogging(c)
	if err != nil {
		log.WithField("err", err).Error("Open log file failed, using stdout instead")
	}
	defer cleanup()

	if childNodeID > 0 {
		if err := runChildNodeServer(c, config, childNodeID); err != nil {
			log.WithFields(log.Fields{
				"node_id": childNodeID,
				"err":     err,
			}).Error("Run child node failed")
		}
		return
	}

	if err := runMasterServer(c, config, watch); err != nil {
		log.WithField("err", err).Error("Run master server failed")
	}
}
