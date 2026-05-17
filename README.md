# Distributed Database System

A Go + MySQL distributed database demo with one master program and one reusable slave program.

## Main Idea

Run the master on one device. Run the same slave code on any other device. The slave only needs the master's IP address in `MASTER_URL`, then it automatically registers itself with the master.

Project commands:

```text
nodes/master    master node
nodes/slave     generic slave node for every slave device
```

There is no separate `slave1` or `slave2` code anymore.

## Master Device Setup

On the master device, create `.env` like this. Replace `MASTER_DEVICE_IP` with the real LAN IP of the master device.

```env
MYSQL_DSN=root:your_mysql_password@tcp(localhost:3306)/distributed_master?parseTime=true
MASTER_ID=master
MASTER_HOST=0.0.0.0
MASTER_PORT=8080
MASTER_PUBLIC_URL=http://MASTER_DEVICE_IP:8080
MASTER_URL=http://MASTER_DEVICE_IP:8080
SLAVE_URLS=
```

Start the master:

```powershell
go run .\nodes\master
```

`MASTER_HOST=0.0.0.0` makes the master listen on the network, not only on localhost.
After the server starts, the master terminal also shows an operations menu so you can choose actions interactively.

## Slave Device Setup

On each slave device, use the same project code and create `.env` like this. Replace `MASTER_DEVICE_IP` with the master IP. Replace `THIS_SLAVE_DEVICE_IP` with the slave device IP.

```env
MASTER_URL=http://MASTER_DEVICE_IP:8080
NODE_ID=slave-1
NODE_HOST=0.0.0.0
NODE_PORT=8081
NODE_PUBLIC_URL=http://THIS_SLAVE_DEVICE_IP:8081
NODE_MYSQL_DSN=root:your_mysql_password@tcp(localhost:3306)/distributed_slave?parseTime=true
```

Start the slave:

```powershell
go run .\nodes\slave
```

The slave keeps retrying registration until the master is reachable. To add more slaves, run the same command on another device and change only:

```env
NODE_ID=slave-2
NODE_PUBLIC_URL=http://ANOTHER_SLAVE_IP:8081
NODE_MYSQL_DSN=root:your_mysql_password@tcp(localhost:3306)/distributed_slave2?parseTime=true
```

Each slave terminal also shows an operations menu. When you run write operations from a slave, the request is forwarded to the master approval queue.

## Confirm Connection

From any device that can reach the master:

```powershell
Invoke-RestMethod http://MASTER_DEVICE_IP:8080/cluster/status
```

You should see registered slaves with their IDs, URLs, health state, and pending replication count.

## Local Demo On One Device

Use one master terminal and one slave terminal.

Terminal 1:

```powershell
go run .\nodes\master
```

Terminal 2:

```powershell
go run .\nodes\slave
```

Terminal 3:

```powershell
.\scripts\demo.ps1
```

The demo creates `demo_users`, inserts rows, updates a row, deletes a row, reads from the slave, and then lets you stop the master to prove the slave still serves reads.

## Interactive Menus

Both node programs now open a terminal menu after startup:

- Master menu: create/drop tables, drop the database, run insert/update/delete/select queries, show cluster status, review approval requests, approve/reject requests, and retry pending replication.
- Slave menu: create/drop tables, run insert/update/delete/select queries, and promote the slave to master.

For each operation, the program asks for the required input such as table name, columns, approval ID, or SQL query.

## APIs

| Endpoint | Node | Purpose |
| --- | --- | --- |
| `POST /register-slave` | Master | Slave registers itself automatically |
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
- Open Windows Firewall for `8080` on the master and `8081` on each slave.
- Test master reachability from a slave with `Invoke-RestMethod http://MASTER_DEVICE_IP:8080/health`.
- Test slave reachability from master with `Invoke-RestMethod http://SLAVE_DEVICE_IP:8081/health`.
- Each device can use its own local MySQL server and database.
