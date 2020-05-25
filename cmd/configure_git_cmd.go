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
	"bufio"
	"encoding/base64"
	"fmt"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type configureGitCmd struct {
	BaseCommand

	gitUsername string
	gitEmail    string

	sshKeyEnv  string
	sshKeyFile string
}

func (cmd *configureGitCmd) Execute() {
	if val, found := os.LookupEnv(cmd.sshKeyEnv); found && val != "" {
		sshKey, err := base64.StdEncoding.DecodeString(val)
		if err != nil {
			cmd.Failf("unable to decode ssh key. err: %v\n", err)
		}
		if err = ioutil.WriteFile(cmd.sshKeyFile, sshKey, 0600); err != nil {
			cmd.Failf("unable to write ssh key file %v. err: %v\n", cmd.sshKeyFile, err)
		}
	} else {
		cmd.Failf("unable to read ssh key from env var %v. Found? %v\n", cmd.sshKeyEnv, found)
	}

	kfAbs, err := filepath.Abs(cmd.sshKeyFile)
	if err != nil {
		cmd.Failf("unable to read path for sshKeyFile? %v\n", cmd.sshKeyFile)
	}

	keyDir := path.Dir(kfAbs)

	ignoreExists := false
	file, err := os.Open(keyDir + string(os.PathSeparator) + ".gitignore")
	if err != nil {
		//probably means the file isn't there etc. just ignore this particular error
	} else {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), cmd.sshKeyFile) {
				ignoreExists = true
			}
		}

		if err := scanner.Err(); err != nil {
			//probably means the file isn't there etc. just ignore this particular error
		}
		file.Close()
	}

	if !ignoreExists {
		cmd.Infof("adding " + cmd.sshKeyFile + " to .gitignore\n")
		//add the deploy key to .gitignore... next to whereever the sshkey goes...
		f, err := os.OpenFile(keyDir + string(os.PathSeparator) + ".gitignore",
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			cmd.Failf("could not write to .gitignore", err)
		}
		defer f.Close()
		if _, err := f.WriteString("\n" + cmd.sshKeyFile + "\n"); err != nil {
			cmd.Failf("error writing to .gitignore", err)
		}
	} else {
		cmd.Infof(".gitignore file already contains entry for " + cmd.sshKeyFile)
	}

	cmd.RunGitCommand("set git username", "config", "user.name", cmd.gitUsername)
	cmd.RunGitCommand("set git password", "config", "user.email", cmd.gitEmail)
	cmd.RunGitCommand("set ssh config", "config", "core.sshCommand", fmt.Sprintf("ssh -i %v", cmd.sshKeyFile))

	// Ensure we're in ssh mode
	if repoSlug, ok := os.LookupEnv("TRAVIS_REPO_SLUG"); ok {
		url := fmt.Sprintf("git@github.com:%v.git", repoSlug)
		cmd.RunGitCommand("set remote to ssh", "remote", "set-url", "origin", url)
	}
}

func newConfigureGitCmd(root *RootCommand) *cobra.Command {
	cobraCmd := &cobra.Command{
		Use:   "configure-git",
		Short: "Configure git",
		Args:  cobra.ExactArgs(0),
	}

	result := &configureGitCmd{
		BaseCommand: BaseCommand{
			RootCommand: root,
			Cmd:         cobraCmd,
		},
	}

	cobraCmd.PersistentFlags().StringVar(&result.gitUsername, "git-username", DefaultGitUsername, "override the default git username")
	cobraCmd.PersistentFlags().StringVar(&result.gitEmail, "git-email", DefaultGitEmail, "override the default git email")
	cobraCmd.PersistentFlags().StringVar(&result.sshKeyEnv, "ssh-key-env-var", DefaultSshKeyEnvVar, "set ssh key environment variable name")
	cobraCmd.PersistentFlags().StringVar(&result.sshKeyFile, "ssh-key-file", DefaultSshKeyFile, "set ssh key file name")

	return Finalize(result)
}
