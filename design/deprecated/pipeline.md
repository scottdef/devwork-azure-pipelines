// Dynatrace Query Language (DQL) conversion of Azure KQL security detection query
// Note: DQL uses Dynatrace Pattern Language (DPL) instead of regex

fetch logs
| filter log.source == "AppServiceConsoleLogs" // Adjust based on your log source configuration

// Exclude unwanted log entries
| filterOut matchesPhrase(content, "[{'page_content':")
| filterOut matchesPhrase(content, "Request failed with status code")
| filterOut matchesPhrase(content, "type=REFRESH_TOKEN_ERROR")
| filterOut matchesPhrase(content, "timeout of 5000ms exceeded")
| filterOut matchesPhrase(content, "Service Authorization System Error")
| filterOut matchesPhrase(content, "Answer")

// Parse for IDA passwords and Azure Databricks tokens
| parse content, "LD 'ida:password' LD:ida_password"
| parse content, "LD 'IssuerSecret' LD:issuer_secret"
| parse content, "LD '.azuredatabricks.net' LD 'dapi'? [a-z0-9/+]{22}:databricks_token"

// Parse for API keys and secrets (multiple patterns)
| parse content, "(api|client|app|application)[_\\- ]?(key|secret)[^,a-z][A-Za-z0-9+/]{20,}:api_secret_1"
| parse content, "x-api-(key|token)[^A-Za-z0-9]*[a-z0-9/+]{40}:x_api_key"
| parse content, "v1\\.[a-z0-9/+]{40}[^a-z0-9/+]:v1_api_key"

// Parse for Google API keys (AIza pattern)
| parse content, "\\WAIza[a-zA-Z0-9_\\-]{35}\\W:google_api_key"

// Parse for Azure service keys and SAS tokens
| parse content, "\\Wsig\\W[^\\s]*[a-z0-9/+]{43}=:azure_sig"
| parse content, "SecretValue[^A-Za-z0-9]*[a-z0-9/+]{43}=:secret_value"
| parse content, "\\Wsas[^A-Za-z0-9]*Key[^A-Za-z0-9]*[a-z0-9/+]{43}=:sas_key"
| parse content, "(primary|secondary|management).*Key[^A-Za-z0-9]*[a-z0-9/+]{43}=:azure_key"
| parse content, "SharedAccess(Policy)?.*Key[^A-Za-z0-9]*[a-z0-9/+]{43}=:shared_access_key"

// Parse for Azure service endpoints with keys
| parse content, "\\.azure-devices\\.net.*[a-z0-9/+]{43}=:iot_key"
| parse content, "\\.(core|servicebus|redis\\.cache|accesscontrol|mediaservices)\\.(windows\\.net|chinacloudapi\\.cn|cloudapi\\.de|usgovcloudapi\\.net).*[a-z0-9/+]{43}=:azure_service_key"

// Parse for Visual Studio tokens
| parse content, "visualstudio\\.com.*\\W[a-z2-7]{52}\\W:vs_token"

// Parse for Azure SAS URL parameters
| parse content, "se=2021.*sig=[a-z0-9%]{43,63}%3d:sas_url"

// Parse for Azure Functions keys
| parse content, "x-functions-key.*[a-z0-9/+]{54}={2}:functions_key"
| parse content, "ApiKey.*[a-z0-9/+]{54}={2}:api_key_long"
| parse content, "Code=.*[a-z0-9/+]{54}={2}:code_param"
| parse content, "\\.azurewebsites\\.net/api/.*[a-z0-9/+]{54}={2}:website_api_key"

// Parse for encoded keys in URLs
| parse content, "code=[a-z0-9%]{54,74}(%3d){2}:url_encoded_key"

// Parse for publishing passwords
| parse content, "(userpwd|publishingpassword).*[a-z0-9/+]{60}\\W:publish_pwd"

// Parse for long base64 encoded strings
| parse content, "[^a-z0-9/+][a-z0-9/+]{86}==:long_base64"

// Parse for private keys
| parse content, "-----BEGIN.*PRIVATE KEY.*-----:private_key_header"

// Parse for application secrets with various delimiters
| parse content, "(app|application|client)[_\\- ]?(key|keyurl|secret)[\\s=:>\"']*[^\\s\\-\"']{8,}:app_secret"

// Parse for refresh tokens
| parse content, "refresh[_\\-]?token[\\s=:>\"']*[a-z0-9/+=_.-]{20,200}:refresh_token"

// Parse for access tokens
| parse content, "AccessToken(Secret)?[\\s=:>\"']*[a-z0-9/+=_.-]{20,200}:access_token"

// Parse for URLs with embedded credentials
| parse content, "[a-z0-9]{3,5}://[^:]+:[^@]+@[^\\s]+:url_with_creds"
| parse content, "(amqp|ssh|https?|ftps?)://[^:]+:[^@]+@[^\\s]+:protocol_url_creds"

// Parse for SNMP configurations
| parse content, "snmp(-server)?\\.exe.*?(priv|community):snmp_config"

// Parse for PowerShell secure strings
| parse content, "ConvertTo-?SecureString.*[\"']:ps_secure_string"

// Parse for consumer/api secrets
| parse content, "(Consumer|api)[_\\- ]?(Secret|Key)[\\s=:>\"']*[^\\s]{5,}:consumer_secret"

// Parse for authorization headers
| parse content, "authorization[,:= \"']+[dbaohmnsv]:auth_header"

// Parse for command line credentials
| parse content, "-u\\s+.{2,100}-p\\s+[^\\-/]:cli_creds"

// Parse for AWS keys
| parse content, "(\\Waws|amazon).{0,5}(secret|access.?key).*\\W[a-z0-9/+]{40}:aws_key"

// Parse for JWT tokens
| parse content, "eyJ0eXAiOiJKV1QiOiJKV1Qi[A-Za-z0-9+/=]+\\.[A-Za-z0-9+/=]+\\.[A-Za-z0-9+/=]*:jwt_token"
| parse content, "eyJhbGci[A-Za-z0-9+/=]+\\.[A-Za-z0-9+/=]+\\.[A-Za-z0-9+/=]*:jwt_token_2"

// Parse for Microsoft account passwords
| parse content, "@.*microsoft\\.com.*?(password|pass|pwd):ms_password"

// Parse for Windows net commands
| parse content, "net(\\.exe)?.*?(user\\s+|share\\s+/user:|user-?secrets? set)\\s+[a-z0-9]:net_command"

// Parse for Slack tokens
| parse content, "xox[pbar]-[a-z0-9-]+:slack_token"

// Parse for Microsoft corporate domains with passwords
| parse content, "(corp|redmond|europe|middleeast|northamerica|southpacific|southamerica|fareast|africa|exchange|extranet|partners|parttest|ntdev|ntwksta)(\\.microsoft\\.com)?.*?(password|pwd|pass|pw|userpass):ms_corp_pwd"

// Parse for SharePoint/Exchange credentials
| parse content, "(sign_in|SharePointOnlineAuthenticatedContext|UserCredentials|ExchangeCredentials|password).*?@.*?microsoft\\.com:ms_service_creds"

// Parse for Azure database passwords
| parse content, "(\\.database\\.azure\\.com|\\.database\\.windows\\.net|\\.cloudapp\\.net|\\.database\\.usgovcloudapi\\.net|\\.database\\.chinacloudapi\\.cn|\\.database\\.cloudapi\\.de).*?(DB_PASS|password|pwd):azure_db_pwd"

// Parse for generic secret/password patterns
| parse content, "(secret\\.?key|password)[\"']?\\s*[:=]\\s*[\"'][^\\s]+[\"']:generic_secret"

// Parse for database credentials
| parse content, "(DB_USER|user id|uid|username|sqluser|service\\s?account).*?(DB_PASS|password|pwd)[^\\s\"';,<]{2,}:db_creds"

// Parse for simple password patterns
| parse content, "(password|secretkey)[ \\t]*[=:]+[ \\t]*[^:\\s\"';,<]{2,200}:simple_password"

// Create summary fields for detected secrets
| fieldsAdd has_secrets = isNotNull(ida_password) or isNotNull(issuer_secret) or isNotNull(databricks_token) 
    or isNotNull(api_secret_1) or isNotNull(x_api_key) or isNotNull(v1_api_key) 
    or isNotNull(google_api_key) or isNotNull(azure_sig) or isNotNull(secret_value)
    or isNotNull(sas_key) or isNotNull(azure_key) or isNotNull(shared_access_key)
    or isNotNull(iot_key) or isNotNull(azure_service_key) or isNotNull(vs_token)
    or isNotNull(sas_url) or isNotNull(functions_key) or isNotNull(api_key_long)
    or isNotNull(code_param) or isNotNull(website_api_key) or isNotNull(url_encoded_key)
    or isNotNull(publish_pwd) or isNotNull(long_base64) or isNotNull(private_key_header)
    or isNotNull(app_secret) or isNotNull(refresh_token) or isNotNull(access_token)
    or isNotNull(url_with_creds) or isNotNull(protocol_url_creds) or isNotNull(snmp_config)
    or isNotNull(ps_secure_string) or isNotNull(consumer_secret) or isNotNull(auth_header)
    or isNotNull(cli_creds) or isNotNull(aws_key) or isNotNull(jwt_token) or isNotNull(jwt_token_2)
    or isNotNull(ms_password) or isNotNull(net_command) or isNotNull(slack_token)
    or isNotNull(ms_corp_pwd) or isNotNull(ms_service_creds) or isNotNull(azure_db_pwd)
    or isNotNull(generic_secret) or isNotNull(db_creds) or isNotNull(simple_password)

// Filter to only show logs with detected secrets
| filter has_secrets == true

// Select relevant fields for output
| fields timestamp, log.source, content, 
    ida_password, issuer_secret, databricks_token, api_secret_1, x_api_key, v1_api_key,
    google_api_key, azure_sig, secret_value, sas_key, azure_key, shared_access_key,
    iot_key, azure_service_key, vs_token, sas_url, functions_key, api_key_long,
    code_param, website_api_key, url_encoded_key, publish_pwd, long_base64,
    private_key_header, app_secret, refresh_token, access_token, url_with_creds,
    protocol_url_creds, snmp_config, ps_secure_string, consumer_secret, auth_header,
    cli_creds, aws_key, jwt_token, jwt_token_2, ms_password, net_command, slack_token,
    ms_corp_pwd, ms_service_creds, azure_db_pwd, generic_secret, db_creds, simple_password

// Optional: Add sorting and limiting
| sort timestamp desc
| limit 1000
