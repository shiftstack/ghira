# ghira

Creates one Jira issue (in the ORC component of OSASINFRA) per each Github issue that is assigned to a team member (in the ORC repository).

Will also move cards to `Closed` when the issue is closed on Github.

Known issues:
* metadata, and notably the assignee, is not updated
* Github issues that have their assignee changed to a non-team-member will be ignored

Run locally:

```bash
export VAULT_TOKEN
./hack/run_with_env.sh go run .
```
