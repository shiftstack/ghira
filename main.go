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
	"github.com/shiftstack/bugwatcher/pkg/team"
)

const (
	jiraBaseURL      = "https://issues.redhat.com/"
	githubRepository = "k-orc/openstack-resource-controller"
	shiftStackQuery  = `project = "OSASINFRA" AND (component in ("ORC"))`
)

var (
	GITHUB_TOKEN = os.Getenv("GITHUB_TOKEN")
	JIRA_TOKEN   = os.Getenv("JIRA_TOKEN")
	PEOPLE       = os.Getenv("PEOPLE")
	TEAM         = os.Getenv("TEAM")

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

// AssignedToTheTeam does two things (sorry). It resolves Github handles to
// Jira usernames, and it filters out issues that are not assigned to team
// members. This will have to be refactored to account for issues that are
// synced as being assigned to the team, and then are reassigned.
func AssignedToTheTeam(issues <-chan GithubIssue, teamMembers []team.Person) <-chan GithubIssue {
	out := make(chan GithubIssue)

	go func() {
		defer close(out)

		for i := range issues {
			// Resolve the author's Jira username
			if author, ok := team.PersonByGithubHandle(teamMembers, i.Author.Handle); ok {
				i.Author.JiraUsername = author.Jira
			}

			// Resolve the assignee's Jira username; append to the results if found.
			if assignee, ok := team.PersonByGithubHandle(teamMembers, i.Assignee.Handle); ok {
				i.Assignee.JiraUsername = assignee.Jira
				out <- i
			}
		}
	}()
	return out
}

func fetchGitHubIssues(ctx context.Context, token string) <-chan GithubIssue {
	issueCh := make(chan GithubIssue)

	go func() {
		defer close(issueCh)

		// https://docs.github.com/en/rest/issues/issues?apiVersion=2022-11-28#list-repository-issues
		client := &http.Client{}
		url := fmt.Sprintf("https://api.github.com/repos/%s/issues", githubRepository)
		for url != "" {
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				log.Fatalf("error fetching issues: %v", err)
				return
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
				log.Fatalf("error fetching issues: %v", err)
				return
			}
			defer func() {
				io.Copy(io.Discard, res.Body)
				res.Body.Close()
			}()

			if statusCode := res.StatusCode; statusCode != 200 {
				body, err := io.ReadAll(res.Body)
				if err != nil {
					log.Fatalf("Status code %d from Github. Additionally, reading the body errored with: %v", statusCode, err)
					return
				}
				log.Fatalf("Status code %d from Github: %s", statusCode, body)
				return
			}
			var issueBatch []GithubIssue
			err = json.NewDecoder(res.Body).Decode(&issueBatch)
			if err != nil {
				log.Fatalf("error decoding Github issues: %v", err)
				return
			}
			for _, issue := range issueBatch {
				if issue.IsPR == nil {
					issueCh <- issue
				}
			}

			url = ""
			if linkHeader := res.Header.Get("link"); linkHeader != "" {
				if s := linkHeaderRegex.FindStringSubmatch(linkHeader); len(s) > 1 {
					url = s[1]
				}
			}
		}
	}()
	return issueCh
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
	ctx := context.Background()
	var teamMembers []team.Person
	{
		people, err := team.Load(strings.NewReader(PEOPLE), strings.NewReader(TEAM))
		if err != nil {
			log.Fatalf("error fetching team information: %v", err)
		}
		teamMembers = make([]team.Person, 0, len(people))
		for i := range people {
			if people[i].TeamMember {
				teamMembers = append(teamMembers, people[i])
			}
		}
	}

	jiraClient, err := jiraclient.NewWithToken(query.JiraBaseURL, JIRA_TOKEN)
	if err != nil {
		log.Fatalf("error building a Jira client: %v", err)
	}

	issues := fetchGitHubIssues(ctx, GITHUB_TOKEN)

	alreadyKnown := make(map[int]knownIssue)
	for issue := range query.SearchIssues(ctx, jiraClient, shiftStackQuery) {
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

	for issue := range AssignedToTheTeam(issues, teamMembers) {
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

	if PEOPLE == "" {
		ex_usage = true
		log.Print("Required environment variable not found: PEOPLE")
	}

	if TEAM == "" {
		ex_usage = true
		log.Print("Required environment variable not found: TEAM")
	}

	if ex_usage {
		log.Print("Exiting.")
		os.Exit(64)
	}
}
