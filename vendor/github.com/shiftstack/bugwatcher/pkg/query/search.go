package query

import (
	"context"
	"log"
	"net/http"

	jira "github.com/andygrunwald/go-jira"
)

func SearchIssues(ctx context.Context, client *jira.Client, searchString string) <-chan jira.Issue {
	issueCh := make(chan jira.Issue)

	go func() {
		opt := &jira.SearchOptions{MaxResults: 100}
		for last, total := 0, 1; last < total; {
			opt.StartAt = last
			issues, res, err := client.Issue.SearchWithContext(ctx, searchString, opt)
			if err != nil {
				log.Fatalf("error fetching issues: %v", err)
				return
			}
			switch res.StatusCode {
			case http.StatusOK, http.StatusNoContent, http.StatusAccepted:
			default:
				log.Printf("unexpected status code %q while fetching issues", res.Status)
				return
			}

			log.Printf("Incoming batch of %d issues", len(issues))

			for _, issue := range issues {
				issueCh <- issue
			}

			last, total = res.StartAt+len(issues), res.Total
		}
		close(issueCh)
	}()

	return issueCh
}
