# ghira

Creates one Jira issue (in the ORC component of OSASINFRA) per each Github issue that is assigned to a team member (in the ORC repository).

Will also move cards to `Closed` when the issue is closed on Github.

Known issues:
* metadata, and notably the assignee, is not updated
* Github issues that have their assignee changed to a non-team-member will be ignored
* Issues are created in status "New". To completely sync a closed Github issue, run ghira twice.

Jira authentication uses `JIRA_EMAIL` and `JIRA_TOKEN`. When
`JIRA_TOKEN` is a scoped API token, also set `JIRA_CLOUD_ID` so that
requests go through the Atlassian API gateway.

Run locally:

```bash
export VAULT_TOKEN
./hack/run_with_env.sh go run .
```
