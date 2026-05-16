# Distributed Database System

A Go + MySQL distributed database demo with HTTP replication, dynamic slave registration, and basic fault tolerance.

## Multi-Device Setup

The master can run on one device, and any number of other devices can run the same generic slave code. Slaves register themselves with the master over the network.

### 1. Master Device

On the master machine, set `.env` like this. Replace `192.168.1.10` with the master's LAN IP address.

```env
MYSQL_DSN=root:your_mysql_password@tcp(localhost:3306)/distributed_master?parseTime=true
MASTER_ID=master
MASTER_HOST=0.0.0.0
MASTER_PORT=8080
MASTER_PUBLIC_URL=http://192.168.1.10:8080
MASTER_URL=http://192.168.1.10:8080
SLAVE_URLS=
```

Start the master:

```powershell
go run .\nodes\master
```

The master listens on all network interfaces because `MASTER_HOST=0.0.0.0`.

### 2. Any Slave Device

On each slave machine, use the same project code and set `.env` like this. Replace the IPs with your real master/slave LAN IPs.

```env
MASTER_URL=http://192.168.1.10:8080
NODE_ID=slave-laptop-1
NODE_HOST=0.0.0.0
NODE_PORT=8081
NODE_PUBLIC_URL=http://192.168.1.11:8081
NODE_MYSQL_DSN=root:your_mysql_password@tcp(localhost:3306)/distributed_slave?parseTime=true
```

Start the generic slave:

```powershell
go run .\nodes\slave
```

The slave will keep retrying `POST /register-slave` until the master is reachable. You can run more slaves by changing `NODE_ID`, `NODE_PORT`, `NODE_PUBLIC_URL`, and the database name in `NODE_MYSQL_DSN`.

### 3. Confirm Registration

From any machine that can reach the master:

```powershell
Invoke-RestMethod http://192.168.1.10:8080/cluster/status
```

You should see the registered slaves with their IDs, URLs, health state, and pending replication count.

## Demo On One Device

You can still run the fixed demo nodes locally in three terminals:

```powershell
go run .\nodes\master
```

```powershell
go run .\nodes\slave1
```

```powershell
go run .\nodes\slave2
```

Then run:

```powershell
.\scripts\demo.ps1
```

The script demonstrates table creation, insert/update/delete replication, reads from slaves, and slave read availability after stopping the master.

## Main APIs

| Endpoint | Node | Purpose |
| --- | --- | --- |
| `POST /register-slave` | Master | Dynamically add a slave to replication |
| `GET /cluster/status` | Master | Show registered slaves and health |
| `POST /create-table` | Master approval flow | Create table |
| `DELETE /drop-table` | Master approval flow | Drop table |
| `DELETE /drop-database` | Master only | Drop database |
| `POST /insert` | Master approval flow | Insert records |
| `PUT /update` | Master approval flow | Update records |
| `DELETE /delete` | Master approval flow | Delete records |
| `GET /select` | All nodes | Read/search records |
| `POST /replicate` | Slaves | Apply replicated write |

## Network Checklist

- Use real LAN IP addresses, not `localhost`, when machines are different devices.
- Open the node ports in Windows Firewall, for example `8080` on master and `8081` on each slave.
- Make sure each machine can reach the other with `Invoke-RestMethod http://IP:PORT/health`.
- Each device can use its own local MySQL server with its own database.
