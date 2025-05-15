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

	TeamMember bool
	BugTriage  bool
	leave      []Leave
}

func (p Person) IsAvailable(t time.Time) bool {
	for _, leave := range p.leave {
		if leave.End.After(t) && leave.Start.Before(t) {
			return false
		}
	}
	return true
}

func Load(peopleYAML, teamYAML io.Reader) ([]Person, error) {
	var people []Person
	if err := yaml.NewDecoder(peopleYAML).Decode(&people); err != nil {
		return nil, fmt.Errorf("error decoding people: %w", err)
	}

	var team map[string]struct {
		BugTriage bool `yaml:"bug_triage"`
		Leave     []struct {
			Start time.Time `yaml:"start"`
			End   time.Time `yaml:"end"`
		} `yaml:"leave"`
	}

	if err := yaml.NewDecoder(teamYAML).Decode(&team); err != nil {
		return nil, fmt.Errorf("error decoding team: %w", err)
	}

	for i := range people {
		// user handles need a prepended `@` when mentioned in the chat
		people[i].Slack = "@" + people[i].Slack

		if teamMember, ok := team[people[i].Kerberos]; ok {
			people[i].TeamMember = true
			people[i].BugTriage = teamMember.BugTriage
			if len(teamMember.Leave) > 0 {
				people[i].leave = make([]Leave, len(teamMember.Leave))
				for j := range teamMember.Leave {
					people[i].leave[j].Start = teamMember.Leave[j].Start
					people[i].leave[j].End = teamMember.Leave[j].End
				}
			}
		}
	}
	return people, nil
}

// PersonByJiraName returns the first person in the slice with the given Jira
// name. The returned boolean is false if not found.
func PersonByJiraName(people []Person, jiraName string) (Person, bool) {
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
	for i := range people {
		if people[i].Github == githubHandle {
			return people[i], true
		}
	}
	return Person{}, false
}
