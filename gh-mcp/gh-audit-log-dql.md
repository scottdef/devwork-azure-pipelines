# Dynatrace API endpoint and credentials
$DT_API_TOKEN = '<Insert your token here>'
$environmentId = '<Insert your environment id here>'

# Construct the URL
$URL = "https://$environmentId.live.dynatrace.com/api/v2/settings/objects"

# Headers
$headers = @{
    'Authorization' = "Api-Token $DT_API_TOKEN"
    'Content-Type' = 'application/json'
}

# Query parameters for the Management Zones schema
$params = @{
    'schemaIds' = 'builtin:management-zones'
    'pageSize' = 100
}

# Fetch management zones
try {
    $response = Invoke-RestMethod -Uri $URL -Method Get -Headers $headers -Body $params -ErrorAction Stop
    
    # Process the management zones
    if ($response.items) {
        foreach ($zone in $response.items) {
            Write-Host "Management Zone: $($zone.value.name) (ID: $($zone.objectId))"
        }
    }
    else {
        Write-Host "No management zones found."
    }
}
catch {
    $statusCode = $_.Exception.Response.StatusCode.value__
    $errorMessage = $_.Exception.Message
    Write-Host "Error fetching management zones: $statusCode - $errorMessage" -ForegroundColor Red
}
