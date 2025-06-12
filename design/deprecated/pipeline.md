// Dynatrace Query Language (DQL) Security Detection Query
// Simplified for DQL notebook execution - using only basic DPL patterns
// Focus on patterns without complex space handling

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

// Simple patterns using only basic DPL matchers that work in DQL notebooks

// Parse for quoted secrets
| parse content, "password=\"" LD:quoted_password "\""
| parse content, "secret=\"" LD:quoted_secret "\""
| parse content, "api_key=\"" LD:quoted_api_key "\""
| parse content, "token=\"" LD:quoted_token "\""

// Parse for common secret formats with colons
| parse content, "password:" LD:colon_password
| parse content, "secret:" LD:colon_secret
| parse content, "api_key:" LD:colon_api_key
| parse content, "token:" LD:colon_token

// Parse for Bearer tokens
| parse content, "Bearer " LD:bearer_token

// Parse for JWT tokens (eyJ start)
| parse content, "eyJ" LD:jwt_token

// Parse for GitHub tokens
| parse content, "ghp_" LD:github_token
| parse content, "gho_" LD:github_oauth_token

// Parse for OpenAI API keys
| parse content, "sk-" LD:openai_key

// Parse for Google API keys
| parse content, "AIza" LD:google_api_key

// Parse for Azure signatures and SAS tokens
| parse content, "sig=" LD:azure_sig
| parse content, "SharedAccessKey=" LD:shared_access_key

// Parse for AWS keys
| parse content, "aws_access_key_id=" LD:aws_access_key
| parse content, "aws_secret_access_key=" LD:aws_secret_key

// Parse for database passwords
| parse content, "DB_PASS=" LD:db_password
| parse content, "database_password=" LD:database_password

// Parse for Azure Functions keys
| parse content, "x-functions-key:" LD:azure_function_key
| parse content, "code=" LD:azure_code_param

// Parse for Slack tokens
| parse content, "xoxb-" LD:slack_bot_token
| parse content, "xoxp-" LD:slack_user_token

// Parse for private key headers
| parse content, "-----BEGIN" LD "PRIVATE KEY-----":private_key_marker

// Parse for connection strings with credentials
| parse content, "://" LD:connection_string_with_creds

// Parse for JSON-style secrets
| parse content, "\"secret\":\"" LD:json_secret "\""
| parse content, "\"password\":\"" LD:json_password "\""
| parse content, "\"api_key\":\"" LD:json_api_key "\""

// Additional common patterns
| parse content, "apikey=" LD:apikey_param
| parse content, "client_secret=" LD:client_secret
| parse content, "access_token=" LD:access_token
| parse content, "refresh_token=" LD:refresh_token

// Create summary field for detected secrets
| fieldsAdd has_secrets = isNotNull(quoted_password) or isNotNull(quoted_secret) or isNotNull(quoted_api_key)
    or isNotNull(quoted_token) or isNotNull(colon_password) or isNotNull(colon_secret)
    or isNotNull(colon_api_key) or isNotNull(colon_token) or isNotNull(bearer_token)
    or isNotNull(jwt_token) or isNotNull(github_token) or isNotNull(github_oauth_token)
    or isNotNull(openai_key) or isNotNull(google_api_key) or isNotNull(azure_sig)
    or isNotNull(shared_access_key) or isNotNull(aws_access_key) or isNotNull(aws_secret_key)
    or isNotNull(db_password) or isNotNull(database_password) or isNotNull(azure_function_key)
    or isNotNull(azure_code_param) or isNotNull(slack_bot_token) or isNotNull(slack_user_token)
    or isNotNull(private_key_marker) or isNotNull(connection_string_with_creds)
    or isNotNull(json_secret) or isNotNull(json_password) or isNotNull(json_api_key)
    or isNotNull(apikey_param) or isNotNull(client_secret) or isNotNull(access_token)
    or isNotNull(refresh_token)

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
| filterOut matchesPhrase(quoted_password, "password")
| filterOut matchesPhrase(quoted_secret, "secret")
| filterOut matchesPhrase(colon_password, "password")
| filterOut matchesPhrase(colon_secret, "secret")

// Trim whitespace from extracted values
| fieldsAdd quoted_password = trim(quoted_password)
| fieldsAdd quoted_secret = trim(quoted_secret)
| fieldsAdd bearer_token = trim(bearer_token)
| fieldsAdd jwt_token = trim(jwt_token)

// Select relevant fields for output
| fields timestamp, log.source, secret_type, content,
    quoted_password, quoted_secret, quoted_api_key, quoted_token,
    colon_password, colon_secret, colon_api_key, colon_token,
    bearer_token, jwt_token, github_token, github_oauth_token,
    openai_key, google_api_key, azure_sig, shared_access_key,
    aws_access_key, aws_secret_key, db_password, database_password,
    azure_function_key, azure_code_param, slack_bot_token, slack_user_token,
    private_key_marker, connection_string_with_creds,
    json_secret, json_password, json_api_key,
    apikey_param, client_secret, access_token, refresh_token

// Optional: Add summarization by secret type
| summarize count(), by: {secret_type}

// Alternative: Sort and limit (uncomment if needed)
// | sort timestamp desc
// | limit 1000
