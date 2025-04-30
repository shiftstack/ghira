package jiraclient

import (
	"log"
	"net/http"
	"strconv"
	"time"

	jira "github.com/andygrunwald/go-jira"
)

type throttlingHttpClient struct {
	*http.Client
}

func (c *throttlingHttpClient) Do(req *http.Request) (*http.Response, error) {
	res, err := c.Client.Do(req)

	if res.StatusCode == http.StatusTooManyRequests {
		wait := time.Second * 1
		if retryAfter := res.Header.Get("retry-after"); retryAfter != "" {
			if n, err := strconv.Atoi(retryAfter); err != nil {
				wait = time.Duration(n) * time.Second
			}
		}
		log.Printf("Throttled by Jira: waiting %s", wait)
		time.Sleep(wait)
		return c.Do(req)
	}

	return res, err
}

// NewWithToken returns a Jira client that retries when hitting 429
func NewWithToken(baseURL, jiraToken string) (jiraClient *jira.Client, err error) {
	return jira.NewClient(
		&throttlingHttpClient{(&jira.BearerAuthTransport{Token: jiraToken}).Client()},
		baseURL,
	)
}
