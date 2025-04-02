package main

import (
	"encoding/json"
	"fmt"
	"io"
)

type Team map[string]TeamMember

type TeamMember struct {
	GithubHandle string `json:"github_handle"`
	JiraName     string `json:"jira_name"`
}

func LoadTeam(teamJSON io.Reader) (team map[string]TeamMember, err error) {
	if err := json.NewDecoder(teamJSON).Decode(&team); err != nil {
		return nil, fmt.Errorf("failed to unmarshal team: %w", err)
	}

	return team, nil
}

func (t Team) JiraUsernameByGithubHandle(githubHandle string) string {
	for _, member := range t {
		if member.GithubHandle == githubHandle {
			return member.JiraName
		}
	}
	return ""
}
