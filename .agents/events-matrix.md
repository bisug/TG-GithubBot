# Webhook event availability matrix

Source: GitHub webhook events and payloads docs (docs.github.com).
R = repository hook, O = organization hook, A = GitHub App, B = business,
M = marketplace, S = sponsors listing.

| Event | R | O | A | Notes |
|---|---|---|---|---|
| branch_protection_configuration | ✅ | ✅ | ✅ | |
| branch_protection_rule | ✅ | ✅ | ✅ | |
| check_run | ✅ | ✅ | ✅ | R/O only get created/completed |
| check_suite | ✅ | ✅ | ✅ | R/O only get completed |
| code_scanning_alert | ✅ | ✅ | ✅ | |
| commit_comment | ✅ | ✅ | ✅ | |
| content_reference | ✅ | ✅ | ✅ | |
| create | ✅ | ✅ | ✅ | |
| custom_property | ❌ | ✅ | ✅ | business too |
| custom_property_values | ✅ | ✅ | ✅ | |
| delete | ✅ | ✅ | ✅ | |
| dependabot_alert | ✅ | ✅ | ✅ | |
| deploy_key | ✅ | ✅ | ✅ | |
| deployment | ✅ | ✅ | ✅ | |
| deployment_protection_rule | ❌ | ❌ | ✅ | |
| deployment_review | ❌ | ❌ | ✅ | |
| deployment_status | ✅ | ✅ | ✅ | |
| discussion | ✅ | ✅ | ✅ | preview |
| discussion_comment | ✅ | ✅ | ✅ | preview |
| fork | ✅ | ✅ | ✅ | business too |
| github_app_authorization | ❌ | ❌ | ✅ | default, cannot unsubscribe |
| gollum | ✅ | ✅ | ✅ | |
| installation | ❌ | ❌ | ✅ | default |
| installation_repositories | ❌ | ❌ | ✅ | default |
| installation_target | ❌ | ❌ | ✅ | default |
| issue_comment | ✅ | ✅ | ✅ | |
| issue_dependencies | ✅ | ✅ | ✅ | |
| issues | ✅ | ✅ | ✅ | |
| label | ✅ | ✅ | ✅ | |
| marketplace_purchase | ❌ | ❌ | ❌ | marketplace only |
| member | ✅ | ✅ | ✅ | business too |
| membership | ❌ | ✅ | ✅ | business too |
| merge_group | ❌ | ❌ | ✅ | |
| meta | ✅ | ✅ | ✅ | business/marketplace too |
| milestone | ✅ | ✅ | ✅ | |
| org_block | ❌ | ✅ | ✅ | business too |
| organization | ❌ | ✅ | ✅ | business too |
| package | ✅ | ✅ | ❌ | |
| page_build | ✅ | ✅ | ✅ | |
| personal_access_token_request | ❌ | ✅ | ✅ | |
| ping | ✅ | ✅ | ✅ | all types |
| project (classic) | ✅ | ✅ | ✅ | DEPRECATED |
| project_card (classic) | ✅ | ✅ | ✅ | DEPRECATED |
| project_column (classic) | ✅ | ✅ | ✅ | DEPRECATED |
| projects_v2 | ❌ | ✅ | ❌ | preview |
| projects_v2_item | ❌ | ✅ | ❌ | preview |
| projects_v2_status_update | ❌ | ✅ | ❌ | preview |
| public | ✅ | ✅ | ✅ | |
| pull_request | ✅ | ✅ | ✅ | |
| pull_request_review | ✅ | ✅ | ✅ | |
| pull_request_review_comment | ✅ | ✅ | ✅ | |
| pull_request_review_thread | ✅ | ✅ | ✅ | |
| pull_request_target | ✅ | ✅ | ✅ | |
| push | ✅ | ✅ | ✅ | |
| registry_package | ✅ | ✅ | ✅ | use `package` instead |
| release | ✅ | ✅ | ✅ | |
| repository | ✅ | ✅ | ✅ | business too |
| repository_advisory | ✅ | ✅ | ✅ | |
| repository_dispatch | ❌ | ❌ | ✅ | |
| repository_import | ✅ | ✅ | ❌ | |
| repository_ruleset | ✅ | ✅ | ✅ | |
| repository_vulnerability_alert | ✅ | ✅ | ❌ | closing down → dependabot_alert |
| secret_scanning_alert | ✅ | ✅ | ✅ | |
| secret_scanning_alert_location | ✅ | ✅ | ✅ | |
| secret_scanning_scan | ✅ | ✅ | ✅ | no action field |
| security_advisory | ❌ | ❌ | ✅ | |
| security_and_analysis | ✅ | ✅ | ✅ | |
| sponsorship | ❌ | ❌ | ❌ | sponsors listing only |
| star | ✅ | ✅ | ✅ | |
| status | ✅ | ✅ | ✅ | |
| sub_issues | ✅ | ✅ | ✅ | |
| team | ❌ | ✅ | ✅ | business too |
| team_add | ✅ | ✅ | ✅ | |
| user | ✅ | ✅ | ✅ | |
| watch | ✅ | ✅ | ✅ | |
| workflow_dispatch | ❌ | ❌ | ✅ | |
| workflow_job | ✅ | ✅ | ✅ | business too |
| workflow_run | ✅ | ✅ | ✅ | business too |

## go-github v90 parse support

`gh.ParseWebhook` handles 66 event types. NOT covered (fall back to generic
or local structs): `issue_dependencies`, `repository_advisory`,
`secret_scanning_scan`, `sub_issues`, `projects_v2_status_update`, `project`,
`project_card`, `project_column`.
