#!/usr/bin/env bash

set -Eeuo pipefail

log() {
	printf >&2 "%s\n" "$*"
}

fetch_secret() {
	declare secret httpstatus
	declare -r \
		key="$1" \
		entry="$2"
	secret="$(mktemp)"
	readonly secret

	httpstatus="$(curl \
		-sS \
		-o "$secret" \
		-w "%{http_code}" \
		-X GET \
		-H  'accept: */*' \
		-H  "X-Vault-Token: ${VAULT_TOKEN}" \
		"https://vault.ci.openshift.org/v1/kv/data/selfservice/shiftstack-secrets/${key}")"

	if [[ "$httpstatus" != "200" ]]; then
		log "Unexpected status code from Vault: ${httpstatus}"
		exit 1
	fi

	jq -r -c ".data.data.\"$entry\"" < "$secret"
	rm "$secret"
}

# check_or_fetch takes the name of an environment variable, and a vault
# location to populate it from. If the environment variable is not already set,
# it fetches the key from the vault, sets the variable and exports it.
check_or_fetch() {
	declare -n var="$1"
	declare -r \
		var_name="$1"
		vault_key="$2" \
		vault_entry="$3"

	if [[ -n "${var:-}" ]]; then
		log "Variable ${var_name} found in the environment."
	else
		: "${VAULT_TOKEN?Set VAULT_TOKEN to a valid Vault token to allow fetching secrets.}"
		log "Fetching ${var_name} from the Vault..."
		var="$(fetch_secret "$vault_key" "$vault_entry")"
	fi

	export "${var_name?}"
}

check_or_fetch JIRA_TOKEN   bugwatcher jira-token
check_or_fetch GITHUB_TOKEN ghira      github-token
check_or_fetch PEOPLE       team       people.yaml
check_or_fetch TEAM         team       team.yaml

exec "$@"
