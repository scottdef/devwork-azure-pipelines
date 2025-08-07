#!/bin/bash

# SQLite JSON Import Script for env-dep-rec-res.json
# This script creates a table and imports Azure DevOps deployment records

# Set variables
DB_NAME="deployments.db"
JSON_FILE="env-dep-rec-res.json"
TABLE_NAME="deployment_records"

# Create SQLite database and table
sqlite3 "$DB_NAME" << 'EOF'
CREATE TABLE IF NOT EXISTS deployment_records (
    id INTEGER PRIMARY KEY,
    requestIdentifier TEXT,
    environmentId INTEGER,
    serviceOwner TEXT,
    scopeId TEXT,
    planType TEXT,
    planId TEXT,
    stageName TEXT,
    jobName TEXT,
    stageAttempt INTEGER,
    jobAttempt INTEGER,
    definition_id INTEGER,
    definition_name TEXT,
    definition_web_url TEXT,
    definition_api_url TEXT,
    owner_id INTEGER,
    owner_name TEXT,
    owner_web_url TEXT,
    owner_api_url TEXT,
    result TEXT,
    queueTime TEXT,
    startTime TEXT,
    finishTime TEXT
);
EOF

echo "Table '$TABLE_NAME' created successfully in $DB_NAME"

# Method 1: Using jq to parse JSON and generate SQL INSERT statements
echo "Importing data using jq..."

jq -r '.value[] | 
    "INSERT INTO deployment_records (
        id, requestIdentifier, environmentId, serviceOwner, scopeId, 
        planType, planId, stageName, jobName, stageAttempt, jobAttempt,
        definition_id, definition_name, definition_web_url, definition_api_url,
        owner_id, owner_name, owner_web_url, owner_api_url,
        result, queueTime, startTime, finishTime
    ) VALUES (" +
    (.id | tostring) + ", " +
    ("\"" + .requestIdentifier + "\"") + ", " +
    (.environmentId | tostring) + ", " +
    ("\"" + .serviceOwner + "\"") + ", " +
    ("\"" + .scopeId + "\"") + ", " +
    ("\"" + .planType + "\"") + ", " +
    ("\"" + .planId + "\"") + ", " +
    ("\"" + .stageName + "\"") + ", " +
    ("\"" + .jobName + "\"") + ", " +
    (.stageAttempt | tostring) + ", " +
    (.jobAttempt | tostring) + ", " +
    (.definition.id | tostring) + ", " +
    ("\"" + (.definition.name | gsub("\""; "\"\"")) + "\"") + ", " +
    ("\"" + .definition._links.web.href + "\"") + ", " +
    ("\"" + .definition._links.self.href + "\"") + ", " +
    (.owner.id | tostring) + ", " +
    ("\"" + (.owner.name | gsub("\""; "\"\"")) + "\"") + ", " +
    ("\"" + .owner._links.web.href + "\"") + ", " +
    ("\"" + .owner._links.self.href + "\"") + ", " +
    ("\"" + .result + "\"") + ", " +
    ("\"" + .queueTime + "\"") + ", " +
    ("\"" + .startTime + "\"") + ", " +
    ("\"" + .finishTime + "\"") +
    ");"' "$JSON_FILE" | sqlite3 "$DB_NAME"

echo "Data imported successfully!"

# Method 2: Alternative approach using SQLite's JSON1 extension
echo "Alternative method using SQLite JSON1 extension..."

# Create temporary table to hold raw JSON
sqlite3 "$DB_NAME" << 'EOF'
CREATE TEMP TABLE json_import (data TEXT);
EOF

# Import the entire JSON file
sqlite3 "$DB_NAME" << EOF
.mode ascii
.import $JSON_FILE json_import
EOF

# Extract and insert data using JSON functions
sqlite3 "$DB_NAME" << 'EOF'
INSERT INTO deployment_records (
    id, requestIdentifier, environmentId, serviceOwner, scopeId,
    planType, planId, stageName, jobName, stageAttempt, jobAttempt,
    definition_id, definition_name, definition_web_url, definition_api_url,
    owner_id, owner_name, owner_web_url, owner_api_url,
    result, queueTime, startTime, finishTime
)
SELECT 
    json_extract(value, '$.id'),
    json_extract(value, '$.requestIdentifier'),
    json_extract(value, '$.environmentId'),
    json_extract(value, '$.serviceOwner'),
    json_extract(value, '$.scopeId'),
    json_extract(value, '$.planType'),
    json_extract(value, '$.planId'),
    json_extract(value, '$.stageName'),
    json_extract(value, '$.jobName'),
    json_extract(value, '$.stageAttempt'),
    json_extract(value, '$.jobAttempt'),
    json_extract(value, '$.definition.id'),
    json_extract(value, '$.definition.name'),
    json_extract(value, '$.definition._links.web.href'),
    json_extract(value, '$.definition._links.self.href'),
    json_extract(value, '$.owner.id'),
    json_extract(value, '$.owner.name'),
    json_extract(value, '$.owner._links.web.href'),
    json_extract(value, '$.owner._links.self.href'),
    json_extract(value, '$.result'),
    json_extract(value, '$.queueTime'),
    json_extract(value, '$.startTime'),
    json_extract(value, '$.finishTime')
FROM json_import, json_each(json_extract(data, '$.value'));
EOF

# Verify the import
echo "Verifying import - Record count:"
sqlite3 "$DB_NAME" "SELECT COUNT(*) as total_records FROM deployment_records;"

echo "Sample records:"
sqlite3 "$DB_NAME" << 'EOF'
.mode column
.headers on
SELECT id, definition_name, result, stageName, queueTime 
FROM deployment_records 
LIMIT 5;
EOF

# Method 3: One-liner for quick import (requires jq)
echo "One-liner command for future reference:"
echo "jq -r '.value[] | [.id, .requestIdentifier, .environmentId, .serviceOwner, .scopeId, .planType, .planId, .stageName, .jobName, .stageAttempt, .jobAttempt, .definition.id, .definition.name, .definition._links.web.href, .definition._links.self.href, .owner.id, .owner.name, .owner._links.web.href, .owner._links.self.href, .result, .queueTime, .startTime, .finishTime] | @csv' $JSON_FILE | sqlite3 -cmd '.mode csv' -cmd '.import /dev/stdin deployment_records' $DB_NAME"

# Query examples
echo "Example queries:"
echo "1. Count by result status:"
sqlite3 "$DB_NAME" << 'EOF'
.mode column
.headers on
SELECT result, COUNT(*) as count 
FROM deployment_records 
GROUP BY result 
ORDER BY count DESC;
EOF

echo "2. Recent deployments:"
sqlite3 "$DB_NAME" << 'EOF'
.mode column
.headers on
SELECT definition_name, result, queueTime 
FROM deployment_records 
ORDER BY queueTime DESC 
LIMIT 10;
EOF

echo "Import completed successfully!"
