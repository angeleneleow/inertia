// Copyright © 2017 UBC Launch Pad team@ubclaunchpad.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package daemon

import (
	"fmt"
	"os"

	jwt "github.com/dgrijalva/jwt-go"
	docker "github.com/docker/docker/client"
	"github.com/google/go-github/github"
	log "github.com/sirupsen/logrus"
	git "gopkg.in/src-d/go-git.v4"

	"github.com/ubclaunchpad/inertia/common"
)

var (
	// DefaultPort defines the standard daemon port
	// TODO: Reference daemon pkg for this information?
	// We only want the package dependencies to go in one
	// direction, so best to think about how to do this.
	// Clearly cannot ask for this information over HTTP.
	DefaultPort = "8081"

	daemonGithubKeyLocation = "/app/host/.ssh/id_rsa_inertia_deploy"
)

// processPushEvent prints information about the given PushEvent.
func processPushEvent(event *github.PushEvent) {
	repo := event.GetRepo()
	log.Println("Received PushEvent")
	log.Println(fmt.Sprintf("Repository Name: %s", *repo.Name))
	log.Println(fmt.Sprintf("Repository Git URL: %s", *repo.GitURL))
	log.Println(fmt.Sprintf("Ref: %s", event.GetRef()))

	// Clone repository if not available, otherwise skip this step and
	// let deploy() handle the pull.
	err := common.CheckForGit(projectDirectory)
	if err != nil {
		log.Println("No git repository present - cloning from push event...")
		pemFile, err := os.Open(daemonGithubKeyLocation)
		if err != nil {
			log.Println("No GitHub key found: " + err.Error())
			return
		}
		auth, err := common.GetGithubKey(pemFile)
		if err != nil {
			log.Println("Github key couldn't be read: " + err.Error())
			return
		}
		_, err = common.Clone(projectDirectory, common.GetSSHRemoteURL(*repo.GitURL), auth, os.Stdout)
		if err != nil {
			log.Println("Clone failed: " + err.Error())
			err = common.RemoveContents(projectDirectory)
			if err != nil {
				log.WithError(err)
			}
			return
		}
	}

	localRepo, err := git.PlainOpen(projectDirectory)
	if err != nil {
		log.WithError(err)
		return
	}

	// Check for matching remotes
	err = common.CompareRemotes(localRepo, common.GetSSHRemoteURL(*repo.GitURL))
	if err != nil {
		log.WithError(err)
		return
	}

	// Deploy project
	cli, err := docker.NewEnvClient()
	if err != nil {
		log.WithError(err)
		return
	}
	defer cli.Close()
	err = deploy(localRepo, cli, os.Stdout)
	if err != nil {
		log.WithError(err)
	}
}

// processPullRequestEvent prints information about the given PullRequestEvent.
// Handling PRs is unnecessary because merging one will trigger a PushEvent.
// For now, simply logs events - may in the future do something configured
// by the user.
func processPullRequestEvent(event *github.PullRequestEvent) {
	repo := event.GetRepo()
	pr := event.GetPullRequest()
	merged := "false"
	if *pr.Merged {
		merged = "true"
	}
	log.Println("Received PullRequestEvent")
	log.Println(fmt.Sprintf("Repository Name: %s", *repo.Name))
	log.Println(fmt.Sprintf("Repository Git URL: %s", *repo.GitURL))
	log.Println(fmt.Sprintf("Ref: %s", pr.GetBase().GetRef()))
	log.Println(fmt.Sprintf("Merge status: %v", merged))
}

// GetAPIPrivateKey returns the private RSA key to authenticate HTTP
// requests sent to the daemon. For now, we simply use the GitHub
// deploy key.
func GetAPIPrivateKey(*jwt.Token) (interface{}, error) {
	pemFile, err := os.Open(daemonGithubKeyLocation)
	if err != nil {
		return nil, err
	}
	key, err := common.GetGithubKey(pemFile)
	if err != nil {
		return nil, err
	}
	return []byte(key.String()), nil
}
