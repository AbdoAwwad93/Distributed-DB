param(
    [string]$MasterUrl = "http://localhost:8080",
    [string]$SlaveUrl = "http://localhost:8081",
    [switch]$SkipMasterStopCheck
)

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Message)
    Write-Host ""
    Write-Host "== $Message ==" -ForegroundColor Cyan
}

function Invoke-Json {
    param(
        [Parameter(Mandatory = $true)][string]$Method,
        [Parameter(Mandatory = $true)][string]$Uri,
        [object]$Body
    )

    $params = @{
        Method = $Method
        Uri = $Uri
    }

    if ($null -ne $Body) {
        $params.ContentType = "application/json"
        $params.Body = $Body | ConvertTo-Json -Depth 8
    }

    Invoke-RestMethod @params
}

function Invoke-WriteQuery {
    param(
        [Parameter(Mandatory = $true)][string]$Method,
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Query
    )

    Invoke-Json -Method $Method -Uri "$MasterUrl$Path" -Body @{ query = $Query }
}

function Invoke-Select {
    param(
        [Parameter(Mandatory = $true)][string]$BaseUrl,
        [Parameter(Mandatory = $true)][string]$Query
    )

    $encodedQuery = [System.Uri]::EscapeDataString($Query)
    Invoke-RestMethod -Method Get -Uri "$BaseUrl/select?query=$encodedQuery"
}

function Show-Rows {
    param(
        [Parameter(Mandatory = $true)][string]$Title,
        [Parameter(Mandatory = $true)][string]$BaseUrl
    )

    Write-Host ""
    Write-Host $Title -ForegroundColor Yellow
    $response = Invoke-Select -BaseUrl $BaseUrl -Query "SELECT id, name, email, status FROM demo_users ORDER BY id"
    if ($response.rows.Count -eq 0) {
        Write-Host "No rows returned"
        return
    }

    $response.rows | Format-Table -AutoSize
}

Write-Step "Checking nodes"
Invoke-RestMethod "$MasterUrl/health" | Format-List
Invoke-RestMethod "$SlaveUrl/health" | Format-List

Write-Step "Waiting for slave registration"
Start-Sleep -Seconds 2
Invoke-RestMethod "$MasterUrl/cluster/status" | ConvertTo-Json -Depth 8

Write-Step "Creating seed table on master"
Invoke-Json -Method Post -Uri "$MasterUrl/create-table" -Body @{
    table = "demo_users"
    columns = @{
        id = "INT PRIMARY KEY"
        name = "VARCHAR(255)"
        email = "VARCHAR(255)"
        status = "VARCHAR(30)"
    }
} | ConvertTo-Json -Depth 8
Start-Sleep -Seconds 1

Write-Step "Clearing old demo rows"
Invoke-WriteQuery -Method Delete -Path "/delete" -Query "DELETE FROM demo_users" | ConvertTo-Json -Depth 8
Start-Sleep -Seconds 1

Write-Step "Inserting rows on master and replicating to slave"
Invoke-WriteQuery -Method Post -Path "/insert" -Query "INSERT INTO demo_users(id, name, email, status) VALUES (1, 'Ali', 'ali@test.com', 'active')" | ConvertTo-Json -Depth 8
Invoke-WriteQuery -Method Post -Path "/insert" -Query "INSERT INTO demo_users(id, name, email, status) VALUES (2, 'Omar', 'omar@test.com', 'active')" | ConvertTo-Json -Depth 8
Start-Sleep -Seconds 1
Show-Rows -Title "Rows from slave after insert replication" -BaseUrl $SlaveUrl

Write-Step "Updating one row on master and reading from slave"
Invoke-WriteQuery -Method Put -Path "/update" -Query "UPDATE demo_users SET status='updated' WHERE id=1" | ConvertTo-Json -Depth 8
Start-Sleep -Seconds 1
Show-Rows -Title "Rows from slave after update replication" -BaseUrl $SlaveUrl

Write-Step "Deleting one row on master and reading from slave"
Invoke-WriteQuery -Method Delete -Path "/delete" -Query "DELETE FROM demo_users WHERE id=2" | ConvertTo-Json -Depth 8
Start-Sleep -Seconds 1
Show-Rows -Title "Rows from slave after delete replication" -BaseUrl $SlaveUrl

Write-Step "Cluster status from master"
Invoke-RestMethod "$MasterUrl/cluster/status" | ConvertTo-Json -Depth 8

if (-not $SkipMasterStopCheck) {
    Write-Step "Master failure demo"
    Write-Host "Stop the master terminal now with Ctrl+C. Leave the slave running."
    Read-Host "Press Enter after the master is stopped"

    Show-Rows -Title "Slave still serves reads while master is stopped" -BaseUrl $SlaveUrl
}

Write-Step "Demo completed"
