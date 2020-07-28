/*
 * Copyright NetFoundry, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"html/template"
	"os"
	"time"
)

var goBuildInfoTemplate = `/*
 * Copyright NetFoundry, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

// Code generated by ziti-ci. DO NOT EDIT.

package {{.PackageName}}

const (
	Version   = "{{.Version}}"
	Revision  = "{{.Revision}}"
	Branch    = "{{.Branch}}"
	BuildUser = "{{.BuildUser}}"
	BuildDate = "{{.BuildDate}}"
)
`

type GoBuildInfo struct {
	PackageName string
	Version     string
	Revision    string
	Branch      string
	BuildUser   string
	BuildDate   string
}

type GoBuildInfoCmd struct {
	BaseCommand

	useV bool
	noAddNoCommit bool
}

func (cmd *GoBuildInfoCmd) Execute() {
	cmd.EvalCurrentAndNextVersion()

	var tagVersion string
	if cmd.useV {
		tagVersion = fmt.Sprintf("v%v", cmd.NextVersion)
	} else {
		tagVersion = fmt.Sprintf("%v", cmd.NextVersion)
	}

	buildInfo := &GoBuildInfo{
		PackageName: cmd.Args[1],
		Version:     tagVersion,
		Revision:    cmd.GetCmdOutputOneLine("get git SHA", "git", "rev-parse", "--short=12", "HEAD"),
		Branch:      cmd.GetCurrentBranch(),
		BuildUser:   cmd.GetUsername(),
		BuildDate:   time.Now().Format("2006-01-02 15:04:05"),
	}

	compiledTemplate, err := template.New("buildInfo").Parse(goBuildInfoTemplate)
	if err != nil {
		cmd.Failf("failure compiling build info template %+v\n", err)
	}

	file, err := os.Create(cmd.Args[0])
	if err != nil {
		cmd.Failf("failure opening build info output file %v. err: %+v\n", cmd.Args[0], err)
	}

	err = compiledTemplate.Execute(file, buildInfo)
	if err != nil {
		cmd.Failf("failure executing build template to output file %v. err: %+v\n", cmd.Args[0], err)
	}

	cmd.RunGitCommand("set git username", "config", "user.name", DefaultGitUsername)
	cmd.RunGitCommand("set git password", "config", "user.email", DefaultGitEmail)

	if cmd.noAddNoCommit {
		cmd.Infof("--noAddNoCommit specified - not committing %s", cmd.Args[0])
	} else {
		cmd.RunGitCommand("add build info file to git", "add", cmd.Args[0])
		cmd.RunGitCommand("commit build info file", "commit", "-m", fmt.Sprintf("Release %v", tagVersion))
	}
}

func newGoBuildInfoCmd(root *RootCommand) *cobra.Command {
	cobraCmd := &cobra.Command{
		Use:   "generate-build-info output-file go-package",
		Short: "Generate build info struct in a go file",
		Args:  cobra.MinimumNArgs(2),
	}

	result := &GoBuildInfoCmd{
		BaseCommand: BaseCommand{
			RootCommand: root,
			Cmd:         cobraCmd,
		},
	}

	cobraCmd.Flags().BoolVar(&result.useV, "useVersion", true, "include a 'v' in the version or not, default is true")
	cobraCmd.Flags().BoolVar(&result.noAddNoCommit, "noAddNoCommit", false, "do not add nor commit the version file in this action, default is false")

	return Finalize(result)
}
