package team

import (
	"fmt"
	"io"
	"time"

	"gopkg.in/yaml.v3"
)

// ID of @ocp-openstack-team
// https://api.slack.com/reference/surfaces/formatting#mentioning-groups
const TeamSlackId = "!subteam^SKW6QC31Q"

type Leave struct {
	Start time.Time
	End   time.Time
}

type Person struct {
	Kerberos string `yaml:"kerberos"`
	Github   string `yaml:"github_handle"`
	Jira     string `yaml:"jira_name"`
	Slack    string `yaml:"slack_id"`

	BugTriage bool    `yaml:"bug_triage,omitempty"`
	leave     []Leave `yaml:"leave,omitempty"`
}

func (p Person) IsAvailable(t time.Time) bool {
	for _, leave := range p.leave {
		if leave.End.After(t) && leave.Start.Before(t) {
			return false
		}
	}
	return true
}

func Load(peopleYAML io.Reader) ([]Person, error) {
	var people []Person
	if err := yaml.NewDecoder(peopleYAML).Decode(&people); err != nil {
		return nil, fmt.Errorf("error decoding people: %w", err)
	}

	for i := range people {
		// user handles need a prepended `@` when mentioned in the chat
		people[i].Slack = "@" + people[i].Slack
	}

	return people, nil
}

// PersonByJiraName returns the first person in the slice with the given Jira
// name. The returned boolean is false if not found.
func PersonByJiraName(people []Person, jiraName string) (Person, bool) {
	if jiraName == "" {
		return Person{}, false
	}
	for i := range people {
		if people[i].Jira == jiraName {
			return people[i], true
		}
	}
	return Person{}, false
}

// PersonByGithubHandle returns the first person in the slice with the given
// Github handle. The returned boolean is false if not found.
func PersonByGithubHandle(people []Person, githubHandle string) (Person, bool) {
	if githubHandle == "" {
		return Person{}, false
	}
	for i := range people {
		if people[i].Github == githubHandle {
			return people[i], true
		}
	}
	return Person{}, false
}
