User ID & Profile Standards Auditing

1. Evidence of the automated auditing scripts via GitHub API
2. Evidence the scripts have been running on an automated basis every week
3. Evidence of the automatic notifications sent to users when non-compliance is identified
4. Evidence that persistent violations (must be defined by business) resulted in suspension of the account



MultiFactor Authentication
1. Evidence demonstrating all accounts with access to CLA's github have 2FA configured for their account
2. Evidence demonstrating that users cannot turn this setting off
3. Evidence demonstrating that new users have this setting automatically set and/or the process documentation showing this is a requirement for setting up new users



Repository Visibility
1. Evidence that all repos are set to "internal' or "private"
2. Evidence that default visibility for CLA repos is set to Internal or Private via Terraform laC
3. Evidence showing individual users are not able to create repos via GitHub UI
4. Evidence of automated scans for visibility across all CLA repos and associated alerting / monitoring for non-compliance



Repository permissions
1. Evidence of quarterly audits of permissions, including any changes that were made as a result of the quarterly audit


Dynatrace Logs Integration
1. Evidence that logs are configured to be forwarded from GitHub to Dynatrace
2. Evidence that querying is available for the GitHub logs in Dynatrace for auditing, debugging and compliance.
3. Evidence that retention policies are set for >= 12 months for GitHub logs in Dynatrace



Audit Compensating Control Settings
1. Evidence of the script set to audit the compensating control settings to establish 'Compliance as code"
2. Evidence that out of expected values are reported as a result of the automated script and remediated as needed
3. Evidence showing the automated script runs weedy
