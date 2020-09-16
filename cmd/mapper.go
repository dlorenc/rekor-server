/*
Copyright © 2020 Dan Lorenc <dlorenc@redhat.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"github.com/projectrekor/rekor-server/pkg"
	"github.com/spf13/cobra"
)

// serveCmd represents the serve command
var mapperCmd = &cobra.Command{
	Use:   "mapper",
	Short: "start mapper personality",
	Long:  `start mapper personality`,
	Run: func(cmd *cobra.Command, args []string) {
		pkg.StartMapper()
	},
}

func init() {
	rootCmd.AddCommand(mapperCmd)
	//viper.SetDefault("port", "localhost:3000")
}
