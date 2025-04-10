package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	jira "github.com/andygrunwald/go-jira"
	"github.com/shiftstack/bugwatcher/pkg/jiraclient"
	"github.com/shiftstack/bugwatcher/pkg/query"
)

const (
	jiraBaseURL      = "https://issues.redhat.com/"
	githubRepository = "k-orc/openstack-resource-controller"
	shiftStackQuery  = `project = "OSASINFRA" AND (component in ("ORC"))`
)

var (
	GITHUB_TOKEN = os.Getenv("GITHUB_TOKEN")
	JIRA_TOKEN   = os.Getenv("JIRA_TOKEN")
	TEAM_DICT    = os.Getenv("TEAM_DICT")

	ghIssueNumberRegex = regexp.MustCompile(`GH-orc-(\d+): `)
	linkHeaderRegex    = regexp.MustCompile(`<(\S+)>; rel="next"`)
)

type GithubIssue struct {
	Title  string `json:"title"`
	Body   string `json:"body_text"`
	URL    string `json:"html_url"`
	Number int    `json:"number"`
	Author struct {
		Handle       string `json:"login"`
		JiraUsername string `json:"-"`
	} `json:"user"`
	Assignee struct {
		Handle       string `json:"login"`
		JiraUsername string `json:"-"`
	} `json:"assignee"`
	Status string `json:"state"`
	IsPR   any    `json:"pull_request"`
}

// AssignedToTheTeam does three things (sorry). First of all, it filters out
// PRs. Then it resolves Github handles to Jira usernames, and it filters out
// issues that are not assigned to team members. This will have to be
// refactored to account for issues that are synced as being assigned to the
// team, and then are reassigned.
func AssignedToTheTeam(issues []GithubIssue, team Team) []GithubIssue {
	out := make([]GithubIssue, 0, len(issues))
	for _, i := range issues {

		// Ignore PRs
		if i.IsPR != nil {
			continue
		}

		// Resolve the author's Jira username
		if jiraUsername := team.JiraUsernameByGithubHandle(i.Author.Handle); jiraUsername != "" {
			i.Author.JiraUsername = jiraUsername
		}

		// Resolve the assignee's Jira username; append to the results if found.
		if jiraUsername := team.JiraUsernameByGithubHandle(i.Assignee.Handle); jiraUsername != "" {
			i.Assignee.JiraUsername = jiraUsername
			out = append(out, i)
		}
	}
	return out
}

func fetchGitHubIssues(token string) ([]GithubIssue, error) {
	// url := "https://api.github.com/search/issues"
	// req, err := http.NewRequest("GET", url, nil)
	// if err != nil {
	// 	return nil, err
	// }
	// if token != "" {
	// 	req.Header.Set("Authorization", "Bearer "+token)
	// 	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	// 	req.Header.Set("Accept", "application/vnd.github.text+json") // Don't need the Markdown version
	// }
	// {
	// 	q := req.URL.Query()
	// 	q.Add("q", fmt.Sprintf("repo:%s is:issue state:all", githubRepository))
	// 	req.URL.RawQuery = q.Encode()
	// }

	var issues []GithubIssue

	// https://docs.github.com/en/rest/issues/issues?apiVersion=2022-11-28#list-repository-issues
	client := &http.Client{}
	url := fmt.Sprintf("https://api.github.com/repos/%s/issues", githubRepository)
	for url != "" {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
			req.Header.Set("Accept", "application/vnd.github.text+json") // Don't need the Markdown version
		}
		{
			q := req.URL.Query()
			q.Add("state", "all")
			req.URL.RawQuery = q.Encode()
		}

		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() {
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
		}()

		if statusCode := res.StatusCode; statusCode != 200 {
			body, err := io.ReadAll(res.Body)
			if err != nil {
				return nil, fmt.Errorf("Status code %d from Github. Additionally, reading the body errored with: %v", statusCode, err)
			}
			return nil, fmt.Errorf("Status code %d from Github: %s", statusCode, body)
		}
		var issueBatch []GithubIssue
		err = json.NewDecoder(res.Body).Decode(&issueBatch)
		if err != nil {
			return nil, err
		}
		issues = append(issues, issueBatch...)

		url = ""
		if linkHeader := res.Header.Get("link"); linkHeader != "" {
			if s := linkHeaderRegex.FindStringSubmatch(linkHeader); len(s) > 1 {
				url = s[1]
			}
		}
	}

	log.Printf("Found: %d issues on Github", len(issues))
	return issues, nil
}

func createJiraIssue(jiraClient *jira.Client, issue GithubIssue) (*jira.Issue, error) {
	i := jira.Issue{
		Fields: &jira.IssueFields{
			Assignee: &jira.User{
				Name: issue.Assignee.JiraUsername,
			},
			Description: fmt.Sprintf("Originally posted on Github: %s\n\n%s", issue.URL, issue.Body),
			Type: jira.IssueType{
				Name: "Task",
			},
			Project: jira.Project{
				Key: "OSASINFRA",
			},
			Summary:    "GH-orc-" + strconv.Itoa(issue.Number) + ": " + issue.Title,
			Components: []*jira.Component{{Name: "ORC"}},
		},
	}

	jiraIssue, response, err := jiraClient.Issue.Create(&i)
	if err != nil {
		io.Copy(os.Stderr, response.Body)
		log.Println()
		return nil, err
	}

	return jiraIssue, nil
}

type knownIssue struct {
	Key    string
	Status *jira.Status
}

func main() {
	var team Team
	{
		var err error
		team, err = LoadTeam(strings.NewReader(TEAM_DICT))
		if err != nil {
			log.Printf("Failed to load the list of team members: %v", err)
			os.Exit(1)
		}
	}

	jiraClient, err := jiraclient.NewWithToken(query.JiraBaseURL, JIRA_TOKEN)
	if err != nil {
		log.Fatalf("error building a Jira client: %v", err)
	}

	issues, err := fetchGitHubIssues(GITHUB_TOKEN)
	if err != nil {
		fmt.Println("Error fetching GitHub issues:", err)
		return
	}

	alreadyKnown := make(map[int]knownIssue)
	for issue := range query.SearchIssues(context.Background(), jiraClient, shiftStackQuery) {
		if s := ghIssueNumberRegex.FindStringSubmatch(issue.Fields.Summary); len(s) > 1 {
			n, err := strconv.Atoi(s[1])
			if err != nil {
				panic("unexpected error: could not parse the issue number: " + err.Error())
			}
			alreadyKnown[n] = knownIssue{
				Key:    issue.Key,
				Status: issue.Fields.Status,
			}
		}
	}

	{
		alreadyKnownNumbers := make([]string, 0, len(alreadyKnown))
		for k := range alreadyKnown {
			alreadyKnownNumbers = append(alreadyKnownNumbers, strconv.Itoa(k))
		}
		log.Printf("Known issues: %v", alreadyKnownNumbers)
	}

	for _, issue := range AssignedToTheTeam(issues, team) {
		log.Printf("Now processing Github issue number %d, assigned to %s, status %q", issue.Number, issue.Author.Handle, issue.Status)

		if jiraIssue, ok := alreadyKnown[issue.Number]; ok {
			var transitionTodo, transitionClosed string
			{
				possibleTransitions, _, _ := jiraClient.Issue.GetTransitions(jiraIssue.Key)
				for _, v := range possibleTransitions {
					switch v.Name {
					case "Closed":
						transitionClosed = v.ID
					case "To Do":
						transitionTodo = v.ID
					}
				}
			}

			switch {
			case issue.Status == "closed" && jiraIssue.Status.Name != "Closed":
				if _, err := jiraClient.Issue.DoTransition(jiraIssue.Key, transitionClosed); err != nil {
					log.Printf("ERROR: Unable to transition issue %s to Closed: %v", jiraIssue.Key, err)
				} else {
					log.Printf("Transitioned issue %s to Closed", jiraIssue.Key)
				}
			case issue.Status == "open" && jiraIssue.Status.Name == "Closed":
				if _, err := jiraClient.Issue.DoTransition(jiraIssue.Key, transitionTodo); err != nil {
					log.Printf("ERROR: Unable to transition issue %s to To Do: %v", jiraIssue.Key, err)
				} else {
					log.Printf("Transitioned issue %s to To Do", jiraIssue.Key)
				}
			}
		} else {
			jiraIssue, err := createJiraIssue(jiraClient, issue)
			if err != nil {
				fmt.Println("Error creating Jira story:", err)
			}

			if jiraIssue != nil {
				log.Printf("Created Jira task with key: %s", jiraIssue.Key)
			}
		}
	}
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)

	ex_usage := false
	if GITHUB_TOKEN == "" {
		ex_usage = true
		log.Print("Required environment variable not found: GITHUB_TOKEN")
	}

	if JIRA_TOKEN == "" {
		ex_usage = true
		log.Print("Required environment variable not found: JIRA_TOKEN")
	}

	if TEAM_DICT == "" {
		ex_usage = true
		log.Print("Required environment variable not found: TEAM_MEMBERS_DICT")
	}

	if ex_usage {
		log.Print("Exiting.")
		os.Exit(64)
	}
}
