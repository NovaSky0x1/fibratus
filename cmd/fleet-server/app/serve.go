/*
 * Copyright 2021-2022 by Nedim Sabic Sabic
 * https://www.fibratus.io
 * All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rabbitstack/fibratus/internal/fleetserver"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var configFile string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the fleet management server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := fleetserver.LoadConfig(configFile)
		if err != nil {
			return err
		}

		initLogging(cfg.Logging.Level)

		srv, err := fleetserver.New(cfg)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			sig := <-sigCh
			log.Infof("received %v signal, shutting down...", sig)
			cancel()
		}()

		return srv.Run(ctx)
	},
}

func init() {
	serveCmd.Flags().StringVarP(&configFile, "config", "c", "configs/fleet-server.yml", "Path to configuration file")
}

func initLogging(level string) {
	lvl, err := log.ParseLevel(level)
	if err != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
}
