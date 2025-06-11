// Dynatrace Query Language (DQL) Security Detection Query
// Corrected with proper DPL syntax and simplified patterns
// Focus on reliable detection patterns using DPL built-in matchers

fetch logs
| filter log.source == "AppServiceConsoleLogs" // Adjust based on your log source

// Pre-filter logs that might contain secrets using matchesPhrase for performance
| filter matchesPhrase(content, "*secret*") 
    or matchesPhrase(content, "*password*") 
    or matchesPhrase(content, "*key*")
    or matchesPhrase(content, "*token*")
    or matchesPhrase(content, "*api*")
    or matchesPhrase(content, "*Bearer*")
    or matchesPhrase(content, "eyJ")  // JWT tokens
    or matchesPhrase(content, "sk-")  // OpenAI keys
    or matchesPhrase(content, "ghp_") // GitHub tokens

// Exclude unwanted log entries
| filterOut matchesPhrase(content, "[{'page_content':")
| filterOut matchesPhrase(content, "Request failed with status code")
| filterOut matchesPhrase(content, "type=REFRESH_TOKEN_ERROR")
| filterOut matchesPhrase(content, "timeout of 5000ms exceeded")
| filterOut matchesPhrase(content, "Service Authorization System Error")
| filterOut matchesPhrase(content, "Answer")

// Simple patterns using DPL built-in matchers and character groups

// Parse for basic secret patterns with simple delimiters
| parse content, "password" SPACE+ [A-Za-z0-9+/=]{8,}:basic_password
| parse content, "secret" SPACE+ [A-Za-z0-9+/=]{8,}:basic_secret  
| parse content, "token" SPACE+ [A-Za-z0-9+/=]{8,}:basic_token
| parse content, "api_key" SPACE+ [A-Za-z0-9+/=]{8,}:basic_api_key

// Parse for quoted secrets
| parse content, "password" SPACE* "=" SPACE* DQUOTE [^\"]+:quoted_password DQUOTE
| parse content, "secret" SPACE* "=" SPACE* DQUOTE [^\"]+:quoted_secret DQUOTE
| parse content, "api_key" SPACE* "=" SPACE* DQUOTE [^\"]+:quoted_api_key DQUOTE

// Parse for common secret formats with colons
| parse content, "password:" [A-Za-z0-9+/=]{8,}:colon_password
| parse content, "secret:" [A-Za-z0-9+/=]{8,}:colon_secret
| parse content, "api_key:" [A-Za-z0-9+/=]{8,}:colon_api_key

// Parse for Bearer tokens
| parse content, "Bearer" SPACE+ [A-Za-z0-9._~+/=-]{20,}:bearer_token

// Parse for JWT tokens (eyJ start)
| parse content, "eyJ" [A-Za-z0-9+/=]+ "." [A-Za-z0-9+/=]+ "." [A-Za-z0-9+/=]*:jwt_token

// Parse for GitHub tokens
| parse content, "ghp_" [A-Za-z0-9]{36}:github_token
| parse content, "gho_" [A-Za-z0-9]{36}:github_oauth_token

// Parse for OpenAI API keys
| parse content, "sk-" [A-Za-z0-9]{48}:openai_key

// Parse for Google API keys (AIza pattern)
| parse content, "AIza" [A-Za-z0-9_-]{35}:google_api_key

// Parse for Azure signatures and SAS tokens
| parse content, "sig=" [a-z0-9%]+:azure_sig
| parse content, "SharedAccessKey=" [A-Za-z0-9+/=]{40,}:shared_access_key

// Parse for AWS keys (basic pattern)
| parse content, "aws_access_key_id" SPACE* "=" SPACE* [A-Z0-9]{20}:aws_access_key
| parse content, "aws_secret_access_key" SPACE* "=" SPACE* [A-Za-z0-9+/=]{40}:aws_secret_key

// Parse for database passwords
| parse content, "DB_PASS" SPACE* "=" SPACE* [^\\s\"';,<]{6,}:db_password
| parse content, "database_password" SPACE* "=" SPACE* [^\\s\"';,<]{6,}:database_password

// Parse for Azure Functions keys
| parse content, "x-functions-key" SPACE* ":" SPACE* [A-Za-z0-9+/=]{50,}:azure_function_key
| parse content, "code=" [A-Za-z0-9%+/=]{50,}:azure_code_param

// Parse for Slack tokens
| parse content, "xoxb-" [a-z0-9-]+:slack_bot_token
| parse content, "xoxp-" [a-z0-9-]+:slack_user_token

// Parse for private key headers
| parse content, "-----BEGIN" SPACE+ LD SPACE+ "PRIVATE KEY-----":private_key_marker

// Parse for connection strings with credentials
| parse content, "://" LD ":" LD "@" LD:connection_string_with_creds

// Parse for simple key-value patterns with equals
| parse content, LD "key" LD "=" LD [A-Za-z0-9+/=]{12,}:generic_key_equals
| parse content, LD "secret" LD "=" LD [A-Za-z0-9+/=]{12,}:generic_secret_equals

// Parse for JSON-style secrets
| parse content, DQUOTE LD "secret" LD DQUOTE SPACE* ":" SPACE* DQUOTE [^\"]+:json_secret DQUOTE
| parse content, DQUOTE LD "password" LD DQUOTE SPACE* ":" SPACE* DQUOTE [^\"]+:json_password DQUOTE

// Create summary field for detected secrets
| fieldsAdd has_secrets = isNotNull(basic_password) or isNotNull(basic_secret) or isNotNull(basic_token)
    or isNotNull(basic_api_key) or isNotNull(quoted_password) or isNotNull(quoted_secret)
    or isNotNull(quoted_api_key) or isNotNull(colon_password) or isNotNull(colon_secret)
    or isNotNull(colon_api_key) or isNotNull(bearer_token) or isNotNull(jwt_token)
    or isNotNull(github_token) or isNotNull(github_oauth_token) or isNotNull(openai_key)
    or isNotNull(google_api_key) or isNotNull(azure_sig) or isNotNull(shared_access_key)
    or isNotNull(aws_access_key) or isNotNull(aws_secret_key) or isNotNull(db_password)
    or isNotNull(database_password) or isNotNull(azure_function_key) or isNotNull(azure_code_param)
    or isNotNull(slack_bot_token) or isNotNull(slack_user_token) or isNotNull(private_key_marker)
    or isNotNull(connection_string_with_creds) or isNotNull(generic_key_equals)
    or isNotNull(generic_secret_equals) or isNotNull(json_secret) or isNotNull(json_password)

// Filter to only show logs with detected secrets
| filter has_secrets == true

// Create secret type classification
| fieldsAdd secret_type = 
    if(isNotNull(jwt_token), "JWT Token",
    if(isNotNull(github_token) or isNotNull(github_oauth_token), "GitHub Token",
    if(isNotNull(openai_key), "OpenAI Key",
    if(isNotNull(google_api_key), "Google API Key",
    if(isNotNull(bearer_token), "Bearer Token",
    if(isNotNull(azure_sig) or isNotNull(shared_access_key) or isNotNull(azure_function_key) or isNotNull(azure_code_param), "Azure",
    if(isNotNull(aws_access_key) or isNotNull(aws_secret_key), "AWS",
    if(isNotNull(slack_bot_token) or isNotNull(slack_user_token), "Slack",
    if(isNotNull(db_password) or isNotNull(database_password), "Database",
    if(isNotNull(private_key_marker), "Private Key",
    if(isNotNull(connection_string_with_creds), "Connection String",
    "Generic Secret")))))))))))

// Filter out obvious false positives
| filterOut matchesPhrase(basic_password, "password") 
| filterOut matchesPhrase(basic_secret, "secret")
| filterOut matchesPhrase(basic_token, "token")
| filterOut matchesPhrase(quoted_password, "password")
| filterOut matchesPhrase(quoted_secret, "secret")
| filterOut matchesPhrase(colon_password, "password")

// Select relevant fields for output
| fields timestamp, log.source, secret_type, content,
    basic_password, basic_secret, basic_token, basic_api_key,
    quoted_password, quoted_secret, quoted_api_key,
    colon_password, colon_secret, colon_api_key,
    bearer_token, jwt_token, github_token, github_oauth_token,
    openai_key, google_api_key, azure_sig, shared_access_key,
    aws_access_key, aws_secret_key, db_password, database_password,
    azure_function_key, azure_code_param, slack_bot_token, slack_user_token,
    private_key_marker, connection_string_with_creds,
    generic_key_equals, generic_secret_equals, json_secret, json_password

// Optional: Add summarization by secret type
| summarize count(), by: {secret_type}

// Alternative: Sort and limit (uncomment if needed)
// | sort timestamp desc
// | limit 1000
