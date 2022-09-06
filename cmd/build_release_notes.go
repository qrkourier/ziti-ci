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
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/mod/modfile"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type buildReleaseNotesCmd struct {
	BaseCommand
	AllCommits    bool
	ShowUnchanged bool
}

func (cmd *buildReleaseNotesCmd) Execute() {
	if !cmd.RootCobraCmd.Flags().Changed("quiet") {
		cmd.quiet = true
	}

	cmd.EvalCurrentAndNextVersion()
	data, err := os.ReadFile("go.mod")
	if err != nil {
		panic(err)
	}

	newGoMod, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		panic(err)
	}

	output := cmd.runCommandWithOutput("get go.mod contents", "git", "show", fmt.Sprintf("v%v:go.mod", cmd.CurrentVersion))
	data = []byte(strings.Join(output, "\n"))
	oldGoMod, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		panic(err)
	}

	oldVersions := map[string]*modfile.Require{}

	for _, m := range oldGoMod.Require {
		if strings.Contains(m.Mod.Path, "openziti") {
			oldVersions[m.Mod.Path] = m
		}
	}

	for _, m := range newGoMod.Require {
		if strings.Contains(m.Mod.Path, "openziti") {
			project := strings.Split(m.Mod.Path, "/")[2]
			prev, found := oldVersions[m.Mod.Path]
			if !found {
				fmt.Printf("* `%v`: `%v` (new)\n", m.Mod.Path, m.Mod.Version)
			} else if m.Mod.Version != prev.Mod.Version {
				fmt.Printf("* `%v`: `%v` -> `%v` [View commits](https://github.com/openziti/%v/compare/%v...%v)\n", m.Mod.Path, prev.Mod.Version, m.Mod.Version, project, prev.Mod.Version, m.Mod.Version)
				if err = cmd.GetChanges(project, prev.Mod.Version, m.Mod.Version); err != nil {
					panic(err)
				}
			} else if cmd.ShowUnchanged {
				fmt.Printf("* `%v`: `%v` (unchanged)\n", m.Mod.Path, m.Mod.Version)
			}
		}
	}
}

func (cmd *buildReleaseNotesCmd) GetChanges(project string, oldVersion string, newVersion string) error {
	dir, err := os.Getwd()
	if err != nil {
		return errors.Wrapf(err, "unable to get working directory")
	}

	defer func() {
		if err := os.Chdir(dir); err != nil {
			panic(errors.Wrapf(err, "unable to restore working directory to %v", dir))
		}
	}()

	if err := os.Chdir("../" + project); err != nil {
		return errors.Wrapf(err, "")
	}

	cmd.runCommand("fetch latest tags", "git", "fetch", "--tags")

	r, err := git.PlainOpen(".")
	if err != nil {
		return err
	}

	newTagHash, err := r.ResolveRevision(plumbing.Revision(newVersion))
	if err != nil {
		// check if we're pointing to git hash
		parts := strings.Split(newVersion, "-")
		if len(parts) == 3 {
			gitHash := parts[2]
			newTagHash, err = r.ResolveRevision(plumbing.Revision(gitHash))
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	oldTagHash, err := r.ResolveRevision(plumbing.Revision(oldVersion))
	if err != nil {
		return err
	}

	oldTagIter, err := r.Log(&git.LogOptions{Order: git.LogOrderCommitterTime, From: *oldTagHash})
	if err != nil {
		return err
	}
	defer oldTagIter.Close()

	tagCommit, err := oldTagIter.Next()
	if err != nil {
		return err
	}

	// The old tag may be a tag commit not in the main-line, so we'll have to find the parent
	if tagCommit.NumParents() == 1 && tagCommit.Author.Name == "ziti-ci" {
		c, err := tagCommit.Parent(0)
		if err != nil {
			return err
		}
		oldTagHash = &c.Hash
	}

	iter, err := r.Log(&git.LogOptions{Order: git.LogOrderCommitterTime, From: *newTagHash})
	if err != nil {
		return err
	}

	showedChange := false

	defer iter.Close()
	defer func() {
		if showedChange {
			fmt.Println()
		}
	}()

	for {
		c, err := iter.Next()
		if err == io.EOF {
			return nil
		}
		if c == nil {
			return err
		}
		if c.Hash == *oldTagHash {
			return nil
		}

		if c.Author.Name == "ziti-ci" || c.Author.Name == "dependabot[bot]" {
			continue
		}

		// skip merge commits
		if c.NumParents() > 1 {
			continue
		}

		if cmd.AllCommits {
			lines := strings.Split(c.Message, "\n")
			fmt.Printf("    * %v: %v (%v)\n", c.Hash.String()[:7], lines[0], c.Author.Email)
			showedChange = true
		} else {
			for _, issue := range cmd.extractIssues(c) {
				cmd.outputIssue(issue)
				showedChange = true
			}
		}
	}
}

func (cmd *buildReleaseNotesCmd) extractIssues(c *object.Commit) []string {
	r, err := regexp.Compile(`(fix(e[sd])?|close[sd]?|resolve[sd]?)\s*#(\d+)`)
	if err != nil {
		panic(err)
	}

	matches := r.FindAllStringSubmatch(strings.ToLower(c.Message), -1)
	var result []string
	for _, match := range matches {
		result = append(result, match[3])
	}
	return result
}

func (cmd *buildReleaseNotesCmd) outputIssue(issue string) {
	bin, err := exec.LookPath("gh")
	if err != nil {
		panic(errors.Wrap(err, "gh (github CLI) not found. Please make sure it's installed an you are authenticated"))
	}
	out := cmd.runCommandWithOutput("Get Issue", bin,
		"issue", "view", issue, "--json", "number,title,url", "--jq", `"[Issue #" + (.number|tostring) + "](" + .url + ") - " + .title`)
	fmt.Printf("    * %v\n", out[0])
}

func newBuildReleaseNotesCmd(root *RootCommand) *cobra.Command {
	cobraCmd := &cobra.Command{
		Use:   "build-release-notes",
		Short: "Prints out the release notes for the latest or a given version",
		Args:  cobra.MaximumNArgs(1),
	}

	result := &buildReleaseNotesCmd{
		BaseCommand: BaseCommand{
			RootCommand: root,
			Cmd:         cobraCmd,
		},
	}

	cobraCmd.Flags().BoolVarP(&result.AllCommits, "all-commits", "a", false, "Show all commits, not just closed issues")
	cobraCmd.Flags().BoolVarP(&result.ShowUnchanged, "show-unchanged", "u", false, "Show OpenZiti upstream libraries, even if unchanged")

	return Finalize(result)
}
