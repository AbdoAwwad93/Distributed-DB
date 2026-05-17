package console

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"distributed-db/internal/config"
	"distributed-db/internal/models"
)

type Menu struct {
	cfg     config.NodeConfig
	baseURL string
	reader  *bufio.Reader
	client  *http.Client
}

func NewMenu(cfg config.NodeConfig) *Menu {
	return &Menu{
		cfg:     cfg,
		baseURL: "http://127.0.0.1:" + cfg.Port,
		reader:  bufio.NewReader(os.Stdin),
		client:  &http.Client{},
	}
}

func (m *Menu) Run() {
	for {
		m.printMenu()
		choice := m.prompt("Choose an operation")
		fmt.Println()

		var exit bool
		switch m.cfg.Role {
		case "master":
			exit = m.runMasterChoice(choice)
		default:
			exit = m.runSlaveChoice(choice)
		}

		if exit {
			fmt.Println("Shutting down node...")
			return
		}

		fmt.Println()
	}
}

func (m *Menu) runMasterChoice(choice string) bool {
	switch choice {
	case "1":
		m.createTable()
	case "2":
		m.dropTable()
	case "3":
		m.dropDatabase()
	case "4":
		m.runQueryEndpoint(http.MethodPost, "/insert", "INSERT query")
	case "5":
		m.runQueryEndpoint(http.MethodPut, "/update", "UPDATE query")
	case "6":
		m.runQueryEndpoint(http.MethodDelete, "/delete", "DELETE query")
	case "7":
		m.selectQuery()
	case "8":
		m.sendRequest(http.MethodGet, "/cluster/status", nil)
	case "9":
		m.sendRequest(http.MethodGet, "/approval-requests?status=pending", nil)
	case "10":
		m.decideApproval("approve")
	case "11":
		m.decideApproval("reject")
	case "12":
		m.sendRequest(http.MethodPost, "/replication/retry", nil)
	case "0":
		return true
	default:
		fmt.Println("Invalid choice. Please try again.")
	}

	return false
}

func (m *Menu) runSlaveChoice(choice string) bool {
	switch choice {
	case "1":
		m.createTable()
	case "2":
		m.dropTable()
	case "3":
		m.runQueryEndpoint(http.MethodPost, "/insert", "INSERT query")
	case "4":
		m.runQueryEndpoint(http.MethodPut, "/update", "UPDATE query")
	case "5":
		m.runQueryEndpoint(http.MethodDelete, "/delete", "DELETE query")
	case "6":
		m.selectQuery()
	case "7":
		m.sendRequest(http.MethodPut, "/promote", nil)
		m.cfg.Role = "master"
	case "0":
		return true
	default:
		fmt.Println("Invalid choice. Please try again.")
	}

	return false
}

func (m *Menu) createTable() {
	table := m.prompt("Table name")
	columnCount := m.promptInt("Number of columns")
	columns := make(map[string]string, columnCount)

	for i := 0; i < columnCount; i++ {
		name := m.prompt(fmt.Sprintf("Column %d name", i+1))
		definition := m.prompt(fmt.Sprintf("Column %d definition", i+1))
		columns[name] = definition
	}

	m.sendRequest(http.MethodPost, "/create-table", models.CreateTableRequest{
		Table:   table,
		Columns: columns,
	})
}

func (m *Menu) dropTable() {
	table := m.prompt("Table name")
	m.sendRequest(http.MethodDelete, "/drop-table", models.DropTableRequest{Table: table})
}

func (m *Menu) dropDatabase() {
	confirm := strings.ToLower(m.prompt("Type DROP to confirm dropping the whole database"))
	if confirm != "drop" {
		fmt.Println("Drop database canceled.")
		return
	}

	m.sendRequest(http.MethodDelete, "/drop-database", models.MessageResponse{})
}

func (m *Menu) runQueryEndpoint(method, path, label string) {
	query := m.promptMultiline(label)
	m.sendRequest(method, path, models.QueryRequest{Query: query})
}

func (m *Menu) selectQuery() {
	query := m.promptMultiline("SELECT query")
	m.sendRequest(http.MethodPost, "/select", models.QueryRequest{Query: query})
}

func (m *Menu) decideApproval(action string) {
	requestID := m.prompt("Approval request ID")
	reason := m.prompt("Reason (optional)")
	m.sendRequest(http.MethodPut, "/approval-requests/"+requestID+"/"+action, models.ApprovalDecisionRequest{
		Reason: reason,
	})
}

func (m *Menu) sendRequest(method, path string, body any) {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			fmt.Printf("Failed to encode request: %v\n", err)
			return
		}
		payload = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, m.baseURL+path, payload)
	if err != nil {
		fmt.Printf("Failed to create request: %v\n", err)
		return
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	response, err := m.client.Do(req)
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
		return
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		fmt.Printf("Failed to read response: %v\n", err)
		return
	}

	fmt.Printf("Status: %s\n", response.Status)
	if masterDecision := response.Header.Get("X-Master-Decision"); masterDecision != "" {
		fmt.Printf("Master decision: %s\n", masterDecision)
	}

	if len(bytes.TrimSpace(responseBody)) == 0 {
		return
	}

	if m.printFriendlyResponse(path, responseBody) {
		return
	}

	fmt.Println(strings.TrimSpace(string(responseBody)))
}

func (m *Menu) printFriendlyResponse(path string, responseBody []byte) bool {
	switch {
	case strings.HasPrefix(path, "/select"):
		var response models.SelectResponse
		if json.Unmarshal(responseBody, &response) == nil {
			printRows(response.Rows)
			return true
		}
	case strings.HasPrefix(path, "/cluster/status"):
		var response models.ClusterStatusResponse
		if json.Unmarshal(responseBody, &response) == nil {
			fmt.Printf("Current node role: %s\n", response.Node)
			if len(response.Slaves) == 0 {
				fmt.Println("No registered slaves.")
				return true
			}
			printSlaves(response.Slaves)
			return true
		}
	case strings.HasPrefix(path, "/approval-requests?"), path == "/approval-requests":
		var response models.ApprovalListResponse
		if json.Unmarshal(responseBody, &response) == nil {
			fmt.Printf("Current node role: %s\n", response.Node)
			if len(response.Requests) == 0 {
				fmt.Println("No approval requests found.")
				return true
			}
			printApprovals(response.Requests)
			return true
		}
	case strings.Contains(path, "/approve"), strings.Contains(path, "/reject"):
		var response struct {
			Message  string                 `json:"message"`
			Request  models.ApprovalRequest `json:"request"`
			Response any                    `json:"response"`
		}
		if json.Unmarshal(responseBody, &response) == nil {
			fmt.Println(response.Message)
			printApprovalSummary(response.Request)
			if response.Response != nil {
				fmt.Println("Execution result:")
				printAnyValue(response.Response)
			}
			return true
		}
	default:
		var writeResp struct {
			Message     string                       `json:"message"`
			Replication []models.ReplicationResponse `json:"replication"`
			ID          string                       `json:"id"`
			URL         string                       `json:"url"`
			Status      string                       `json:"status"`
			Node        string                       `json:"node"`
			DBName      string                       `json:"dbName"`
		}
		if json.Unmarshal(responseBody, &writeResp) == nil {
			if writeResp.Message != "" {
				fmt.Println(writeResp.Message)
			}
			if writeResp.ID != "" || writeResp.URL != "" {
				fmt.Printf("Slave ID: %s\n", writeResp.ID)
				fmt.Printf("Slave URL: %s\n", writeResp.URL)
			}
			if writeResp.Status != "" && writeResp.DBName != "" {
				fmt.Printf("Database %s is %s.\n", writeResp.DBName, writeResp.Status)
			}
			if len(writeResp.Replication) > 0 {
				printReplication(writeResp.Replication)
			}
			if writeResp.Message != "" || writeResp.ID != "" || writeResp.DBName != "" || len(writeResp.Replication) > 0 {
				return true
			}
		}
	}

	return false
}

func (m *Menu) printMenu() {
	fmt.Printf("=== %s menu (%s) ===\n", strings.Title(m.cfg.Role), m.cfg.NodeID)
	if m.cfg.Role == "master" {
		fmt.Println("1. Create table")
		fmt.Println("2. Drop table")
		fmt.Println("3. Drop database")
		fmt.Println("4. Insert query")
		fmt.Println("5. Update query")
		fmt.Println("6. Delete query")
		fmt.Println("7. Select query")
		fmt.Println("8. Show cluster status")
		fmt.Println("9. Show pending approvals")
		fmt.Println("10. Approve request")
		fmt.Println("11. Reject request")
		fmt.Println("12. Retry pending replication")
		fmt.Println("0. Exit")
		return
	}

	fmt.Println("1. Create table")
	fmt.Println("2. Drop table")
	fmt.Println("3. Insert query")
	fmt.Println("4. Update query")
	fmt.Println("5. Delete query")
	fmt.Println("6. Select query")
	fmt.Println("7. Promote to master")
	fmt.Println("0. Exit")
}

func (m *Menu) prompt(label string) string {
	for {
		fmt.Printf("%s: ", label)
		text, err := m.reader.ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Printf("Input error: %v\n", err)
			continue
		}

		value := strings.TrimSpace(text)
		if value != "" || err == io.EOF {
			return value
		}
	}
}

func (m *Menu) promptInt(label string) int {
	for {
		value := m.prompt(label)
		var count int
		if _, err := fmt.Sscanf(value, "%d", &count); err == nil && count > 0 {
			return count
		}
		fmt.Println("Please enter a valid number greater than zero.")
	}
}

func (m *Menu) promptMultiline(label string) string {
	fmt.Printf("%s (finish with an empty line):\n", label)
	lines := make([]string, 0, 4)
	for {
		text, err := m.reader.ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Printf("Input error: %v\n", err)
			continue
		}

		line := strings.TrimSpace(text)
		if line == "" {
			if len(lines) > 0 || err == io.EOF {
				return strings.Join(lines, " ")
			}
			fmt.Println("Query cannot be empty. Please enter at least one line.")
			continue
		}

		lines = append(lines, line)
		if err == io.EOF {
			return strings.Join(lines, " ")
		}
	}
}

func printRows(rows []map[string]any) {
	if len(rows) == 0 {
		fmt.Println("No rows found.")
		return
	}

	columns := make([]string, 0, len(rows[0]))
	for column := range rows[0] {
		columns = append(columns, column)
	}
	sort.Strings(columns)

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, strings.Join(columns, "\t"))
	for _, row := range rows {
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			values = append(values, formatCellValue(row[column]))
		}
		fmt.Fprintln(writer, strings.Join(values, "\t"))
	}
	_ = writer.Flush()
}

func printSlaves(slaves []models.SlaveStatus) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tURL\tHealthy\tPending\tLast checked\tLast error")
	for _, slave := range slaves {
		fmt.Fprintf(
			writer,
			"%s\t%s\t%t\t%d\t%s\t%s\n",
			emptyFallback(slave.ID, "-"),
			slave.URL,
			slave.Healthy,
			slave.PendingCount,
			formatTime(slave.LastChecked),
			emptyFallback(slave.LastError, "-"),
		)
	}
	_ = writer.Flush()
}

func printApprovals(requests []models.ApprovalRequest) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tMethod\tPath\tRequested by\tStatus\tCreated at")
	for _, request := range requests {
		fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			request.ID,
			request.Method,
			request.Path,
			request.RequestedBy,
			request.Status,
			formatTime(request.CreatedAt),
		)
	}
	_ = writer.Flush()
}

func printApprovalSummary(request models.ApprovalRequest) {
	fmt.Printf("Request ID: %s\n", request.ID)
	fmt.Printf("Method: %s\n", request.Method)
	fmt.Printf("Path: %s\n", request.Path)
	fmt.Printf("Requested by: %s\n", request.RequestedBy)
	fmt.Printf("Status: %s\n", request.Status)
	if request.Reason != "" {
		fmt.Printf("Reason: %s\n", request.Reason)
	}
}

func printReplication(results []models.ReplicationResponse) {
	fmt.Println("Replication results:")
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "Slave\tStatus\tError")
	for _, result := range results {
		fmt.Fprintf(
			writer,
			"%s\t%s\t%s\n",
			result.Slave,
			result.Status,
			emptyFallback(result.Error, "-"),
		)
	}
	_ = writer.Flush()
}

func printAnyValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Printf("- %s: %v\n", key, typed[key])
		}
	case []any:
		for _, item := range typed {
			fmt.Printf("- %v\n", item)
		}
	default:
		fmt.Println(typed)
	}
}

func formatCellValue(value any) string {
	if value == nil {
		return "NULL"
	}

	return fmt.Sprintf("%v", value)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}

	return value.Local().Format("2006-01-02 15:04:05")
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	return value
}
