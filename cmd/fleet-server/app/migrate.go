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
	"github.com/rabbitstack/fibratus/internal/fleetserver"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store/postgres"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := fleetserver.LoadConfig(configFile)
		if err != nil {
			return err
		}
		initLogging(cfg.Logging.Level)

		log.Info("running database migrations...")
		if err := postgres.Migrate(cfg.Database.DSN()); err != nil {
			return err
		}
		log.Info("migrations completed successfully")
		return nil
	},
}
