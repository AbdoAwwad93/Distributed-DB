# Distributed Database System

A small distributed database demo built with Go, MySQL, and HTTP APIs. The project runs one master node and two slave nodes, supports replication from the master to slaves, and follows a master-write / slave-read model.

## Features

- Master and slave nodes expose HTTP APIs
- Master can replicate writes to slave nodes
- Only the master can run write operations such as `CREATE TABLE`, `INSERT`, `UPDATE`, `DELETE`, `DROP TABLE`, and `DROP DATABASE`
- Slave nodes are read-only for client operations and should be used for `SELECT` requests
- `DROP DATABASE` is restricted to the master node
- Basic health checks and cluster status endpoints
- Demo script for a quick end-to-end walkthrough

## Project Structure

```text
nodes/master   - master node entrypoint
nodes/slave1   - slave 1 entrypoint
nodes/slave2   - slave 2 entrypoint
internal/api - HTTP handlers
internal/storage - MySQL access
internal/replication - replication logic
scripts/demo.ps1 - demo script
```

## Requirements

- Go installed
- MySQL server running
- A MySQL user that can create and modify the configured databases
- PowerShell for running the demo script

## Setup

1. Clone the repository.
2. Copy `.env.example` to `.env`.
3. Update the MySQL password in the DSNs inside `.env`.
4. Make sure MySQL is running on the host and port used in the DSNs.

Example `.env` values:

```env
MYSQL_DSN=root:your_mysql_password@tcp(localhost:3306)/distributed_master?parseTime=true
SLAVE1_MYSQL_DSN=root:your_mysql_password@tcp(localhost:3306)/distributed_slave1?parseTime=true
SLAVE2_MYSQL_DSN=root:your_mysql_password@tcp(localhost:3306)/distributed_slave2?parseTime=true

MASTER_HOST=localhost
MASTER_PORT=8080
SLAVE1_HOST=localhost
SLAVE1_PORT=8081
SLAVE2_HOST=localhost
SLAVE2_PORT=8082

SLAVE_URLS=http://localhost:8081,http://localhost:8082
```

## Run The Nodes

Start each node in a separate PowerShell terminal:

```powershell
go run .\nodes\master
```

```powershell
go run .\nodes\slave1
```

```powershell
go run .\nodes\slave2
```

Default URLs:

| Node | URL |
| --- | --- |
| Master | `http://localhost:8080` |
| Slave 1 | `http://localhost:8081` |
| Slave 2 | `http://localhost:8082` |

The application creates the configured databases automatically on startup if they do not already exist.

## Run The Demo

Run the demo script from another PowerShell terminal:

```powershell
.\scripts\demo.ps1
```

Run it without the manual master-stop step:

```powershell
.\scripts\demo.ps1 -SkipMasterStopCheck
```

The demo covers:

- Health checks
- Creating a table on the master
- Insert, update, and delete replication
- Reading data from slaves
- Basic master failure behavior

## API Usage Examples

### Health Check

```powershell
Invoke-RestMethod http://localhost:8080/health
Invoke-RestMethod http://localhost:8081/health
Invoke-RestMethod http://localhost:8082/health
```

### Database Health

```powershell
Invoke-RestMethod http://localhost:8080/db/health
```

### Cluster Status

```powershell
Invoke-RestMethod http://localhost:8080/cluster/status
```

### Create Table

Create tables through the master:

```powershell
Invoke-RestMethod -Method Post http://localhost:8080/create-table -ContentType "application/json" -Body (@{
    table = "demo_users"
    columns = @{
        id = "INT PRIMARY KEY"
        name = "VARCHAR(255)"
        email = "VARCHAR(255)"
        status = "VARCHAR(30)"
    }
} | ConvertTo-Json -Depth 8)
```

### Insert Row

```powershell
Invoke-RestMethod -Method Post http://localhost:8080/insert -ContentType "application/json" -Body (@{
    query = "INSERT INTO demo_users(id, name, email, status) VALUES (1, 'Ali', 'ali@test.com', 'active')"
} | ConvertTo-Json)
```

### Update Row

```powershell
Invoke-RestMethod -Method Put http://localhost:8080/update -ContentType "application/json" -Body (@{
    query = "UPDATE demo_users SET status='inactive' WHERE id=1"
} | ConvertTo-Json)
```

### Delete Row

```powershell
Invoke-RestMethod -Method Delete http://localhost:8080/delete -ContentType "application/json" -Body (@{
    query = "DELETE FROM demo_users WHERE id=1"
} | ConvertTo-Json)
```

### Select Rows

```powershell
$query = [System.Uri]::EscapeDataString("SELECT * FROM demo_users")
Invoke-RestMethod "http://localhost:8081/select?query=$query"
```

### Drop Table

```powershell
Invoke-RestMethod -Method Delete http://localhost:8080/drop-table -ContentType "application/json" -Body (@{
    table = "demo_users"
} | ConvertTo-Json)
```

### Drop Database

Only the master node can drop its configured database:

```powershell
Invoke-RestMethod -Method Delete http://localhost:8080/drop-database
```

### Retry Pending Replication

```powershell
Invoke-RestMethod -Method Post http://localhost:8080/replication/retry
```

## Behavior Notes

- All client write operations must be sent to the master node.
- The master replicates supported writes to the slave nodes.
- Slave nodes are intended for read operations such as `SELECT`.
- `DROP DATABASE` is master-only.
- Slave nodes can be promoted with the `/promote` endpoint if needed.

## Example Promote Request

```powershell
Invoke-RestMethod -Method Put http://localhost:8081/promote
```
