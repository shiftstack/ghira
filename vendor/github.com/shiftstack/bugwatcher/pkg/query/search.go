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
		opt := &jira.SearchOptionsV2{MaxResults: 100, Fields: []string{"*all"}}
		for {
			issues, res, err := client.Issue.SearchV2JQLWithContext(ctx, searchString, opt)
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

			if res.IsLast || res.NextPageToken == "" {
				break
			}
			opt.NextPageToken = res.NextPageToken
		}
		close(issueCh)
	}()

	return issueCh
}
