# Distributed Database System

A small distributed database demo using Go, MySQL, HTTP APIs, master/slave replication, and basic fault tolerance.

All nodes can serve reads and can execute local table and row operations such as `SELECT`, `CREATE TABLE`, `INSERT`, `UPDATE`, `DELETE`, and `DROP TABLE`. Replication is still initiated by the master node, and `DROP DATABASE` remains master-only.

## Run The Nodes

Create or edit `.env` from `.env.example`, then set your MySQL password in each DSN.

Start the three services in three separate PowerShell terminals:

```powershell
go run .\cmd\slave1
```

```powershell
go run .\cmd\slave2
```

```powershell
go run .\cmd\master
```

Default ports:

| Node | URL |
| --- | --- |
| Master | `http://localhost:8080` |
| Slave 1 | `http://localhost:8081` |
| Slave 2 | `http://localhost:8082` |

## Demo Readiness

Run the scripted demo from another PowerShell terminal:

```powershell
.\scripts\demo.ps1
```

The script demonstrates:

- Seed example table creation: `demo_users`
- Insert replication from master to slaves
- Update replication from master to slaves
- Delete replication from master to slaves
- Reads from `slave1` and `slave2`
- Master failure behavior by stopping the master and reading from slaves again

To run the demo without the manual master-stop step:

```powershell
.\scripts\demo.ps1 -SkipMasterStopCheck
```

## Useful Manual Checks

Health checks:

```powershell
Invoke-RestMethod http://localhost:8080/health
Invoke-RestMethod http://localhost:8081/health
Invoke-RestMethod http://localhost:8082/health
```

Cluster status from master:

```powershell
Invoke-RestMethod http://localhost:8080/cluster/status
```

Read from a slave:

```powershell
$query = [System.Uri]::EscapeDataString("SELECT * FROM demo_users")
Invoke-RestMethod "http://localhost:8081/select?query=$query"
```

Manual retry for queued replication:

```powershell
Invoke-RestMethod -Method Post http://localhost:8080/replication/retry
```

Drop the master's configured database and replicate the drop to slaves:

```powershell
Invoke-RestMethod -Method Delete http://localhost:8080/drop-database
```
