(function () {
  const page = document.body.dataset.page;
  const output = document.getElementById("output");
  const roleBadge = document.getElementById("roleBadge");
  const healthBadge = document.getElementById("healthBadge");
  const dbBadge = document.getElementById("dbBadge");

  function setLoading(button, loading) {
    if (!button) return;
    if (loading) {
      button.dataset.label = button.textContent;
      button.textContent = "Working...";
      button.disabled = true;
    } else {
      button.textContent = button.dataset.label || button.textContent;
      button.disabled = false;
    }
  }

  async function request(path, options) {
    const response = await fetch(path, options);
    const text = await response.text();
    let data = null;
    try {
      data = text ? JSON.parse(text) : null;
    } catch (_err) {
      data = text;
    }

    if (!response.ok) {
      const message = typeof data === "string"
        ? data
        : (data && (data.message || data.error)) || response.statusText;
      throw {
        status: response.status,
        message,
        data,
        headers: response.headers
      };
    }

    return { data, headers: response.headers };
  }

  function clearOutput() {
    output.innerHTML = "";
  }

  function addMessage(title, text, tone) {
    const box = document.createElement("div");
    box.className = `message ${tone || "info"}`;
    box.innerHTML = `<strong>${escapeHTML(title)}</strong><div>${escapeHTML(text)}</div>`;
    output.prepend(box);
  }

  function addHTML(fragment) {
    const wrapper = document.createElement("div");
    wrapper.innerHTML = fragment;
    output.prepend(wrapper);
  }

  function escapeHTML(value) {
    return String(value ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function prettyDate(value) {
    if (!value) return "Unknown";
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
  }

  function parseColumns(text) {
    const lines = text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
    const columns = {};
    for (const line of lines) {
      const firstSpace = line.indexOf(" ");
      if (firstSpace <= 0) {
        throw new Error(`Invalid column line: "${line}"`);
      }
      const name = line.slice(0, firstSpace).trim();
      const definition = line.slice(firstSpace + 1).trim();
      if (!name || !definition) {
        throw new Error(`Invalid column line: "${line}"`);
      }
      columns[name] = definition;
    }
    return columns;
  }

  function renderRows(rows) {
    if (!Array.isArray(rows) || rows.length === 0) {
      addMessage("Query completed", "No rows matched the select query.", "info");
      return;
    }

    const columns = [...new Set(rows.flatMap((row) => Object.keys(row)))];
    const head = columns.map((column) => `<th>${escapeHTML(column)}</th>`).join("");
    const body = rows.map((row) => {
      const cells = columns
        .map((column) => `<td>${escapeHTML(formatValue(row[column]))}</td>`)
        .join("");
      return `<tr>${cells}</tr>`;
    }).join("");

    addHTML(`
      <div class="table-wrap">
        <table>
          <thead><tr>${head}</tr></thead>
          <tbody>${body}</tbody>
        </table>
      </div>
    `);
  }

  function formatValue(value) {
    if (value === null || value === undefined) return "—";
    if (typeof value === "object") return JSON.stringify(value);
    return String(value);
  }

  function renderReplication(replication) {
    if (!Array.isArray(replication) || replication.length === 0) {
      return;
    }

    const cards = replication.map((item) => `
      <article class="mini-card">
        <div class="mini-head">
          <strong>${escapeHTML(item.slave || "slave")}</strong>
          <span class="pill ${item.status === "success" ? "approved" : "rejected"}">${escapeHTML(item.status || "unknown")}</span>
        </div>
        <div class="muted">${escapeHTML(item.error || "Replication finished without reported errors.")}</div>
      </article>
    `).join("");

    addHTML(`<div class="card-list">${cards}</div>`);
  }

  function renderApprovalConfirmation(data, headers) {
    const masterDecision = headers.get("X-Master-Decision");
    if (masterDecision !== "pending") {
      return false;
    }

    addHTML(`
      <div class="message info">
        <strong>Approval request sent to master</strong>
        <div>Your write request was queued for approval on ${escapeHTML(headers.get("X-Master-Node") || "the master node")}.</div>
      </div>
    `);

    if (data && data.id) {
      addHTML(`
        <div class="mini-card">
          <div class="mini-head">
            <strong>Request ${escapeHTML(data.id)}</strong>
            <span class="pill pending">${escapeHTML(data.status || "pending")}</span>
          </div>
          <div class="muted">Requested by ${escapeHTML(data.requestedBy || "slave node")} at ${escapeHTML(prettyDate(data.createdAt))}</div>
        </div>
      `);
    }
    return true;
  }

  async function refreshHealth() {
    try {
      const [{ data: health }, { data: dbHealth }] = await Promise.all([
        request("/health"),
        request("/db/health")
      ]);
      roleBadge.textContent = capitalize(health.node || page);
      healthBadge.textContent = health.status === "ok" ? "Online" : "Issue";
      dbBadge.textContent = dbHealth.dbName || "Connected";
    } catch (error) {
      healthBadge.textContent = "Unavailable";
      dbBadge.textContent = "Unavailable";
      addMessage("Health check failed", error.message || "Could not load node status.", "error");
    }
  }

  function capitalize(value) {
    const text = String(value || "");
    return text ? text.charAt(0).toUpperCase() + text.slice(1) : text;
  }

  async function runQueryAction(action, button) {
    const query = document.getElementById("queryBox").value.trim();
    if (!query) {
      addMessage("Query required", "Please enter a SQL query first.", "error");
      return;
    }

    const config = {
      select: { path: "/select", method: "POST" },
      insert: { path: "/insert", method: "POST" },
      update: { path: "/update", method: "PUT" },
      delete: { path: "/delete", method: "DELETE" }
    }[action];

    if (!config) return;

    setLoading(button, true);
    try {
      clearOutput();
      const { data, headers } = await request(config.path, {
        method: config.method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ query })
      });

      if (renderApprovalConfirmation(data, headers)) {
        return;
      }

      if (action === "select") {
        addMessage("Select completed", `Returned ${Array.isArray(data.rows) ? data.rows.length : 0} row(s).`, "success");
        renderRows(data.rows);
        return;
      }

      addMessage("Write completed", data.message || "The database operation finished successfully.", "success");
      renderReplication(data.replication);
    } catch (error) {
      clearOutput();
      addMessage("Operation failed", error.message || "The request could not be completed.", "error");
    } finally {
      setLoading(button, false);
    }
  }

  async function handleCreateTable(event) {
    event.preventDefault();
    const button = event.submitter || event.target.querySelector("button[type='submit']");
    const table = event.target.table.value.trim();
    const rawColumns = event.target.columns.value;
    if (!table) {
      addMessage("Table name required", "Please enter a table name.", "error");
      return;
    }

    let columns;
    try {
      columns = parseColumns(rawColumns);
    } catch (error) {
      addMessage("Column format problem", error.message, "error");
      return;
    }

    setLoading(button, true);
    try {
      clearOutput();
      const { data, headers } = await request("/create-table", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ table, columns })
      });
      if (!renderApprovalConfirmation(data, headers)) {
        addMessage("Table created", data.message || `Table "${table}" was created.`, "success");
        renderReplication(data.replication);
      }
    } catch (error) {
      clearOutput();
      addMessage("Create table failed", error.message || "The table could not be created.", "error");
    } finally {
      setLoading(button, false);
    }
  }

  async function handleDropTable(event) {
    event.preventDefault();
    const button = event.submitter || event.target.querySelector("button[type='submit']");
    const table = event.target.table.value.trim();
    if (!table) {
      addMessage("Table name required", "Please enter the table to drop.", "error");
      return;
    }

    setLoading(button, true);
    try {
      clearOutput();
      const { data, headers } = await request("/drop-table", {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ table })
      });
      if (!renderApprovalConfirmation(data, headers)) {
        addMessage("Table dropped", data.message || `Table "${table}" was dropped.`, "success");
        renderReplication(data.replication);
      }
    } catch (error) {
      clearOutput();
      addMessage("Drop table failed", error.message || "The table could not be dropped.", "error");
    } finally {
      setLoading(button, false);
    }
  }

  async function handleDropDatabase(button) {
    const confirmed = window.confirm("Drop the entire database? This cannot be undone.");
    if (!confirmed) return;

    setLoading(button, true);
    try {
      clearOutput();
      const { data, headers } = await request("/drop-database", { method: "DELETE" });
      if (!renderApprovalConfirmation(data, headers)) {
        addMessage("Database dropped", data.message || "The database was removed.", "success");
        renderReplication(data.replication);
      }
    } catch (error) {
      clearOutput();
      addMessage("Drop database failed", error.message || "The database could not be dropped.", "error");
    } finally {
      setLoading(button, false);
    }
  }

  async function handleRegisterSlave(event) {
    event.preventDefault();
    const button = event.submitter || event.target.querySelector("button[type='submit']");
    const formData = new FormData(event.target);
    const payload = Object.fromEntries(formData.entries());

    setLoading(button, true);
    try {
      clearOutput();
      const { data } = await request("/register-slave", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
      });
      addMessage("Slave registered", `${data.id} is now connected at ${data.url}.`, "success");
      event.target.reset();
      refreshCluster();
    } catch (error) {
      clearOutput();
      addMessage("Registration failed", error.message || "Could not register the slave.", "error");
    } finally {
      setLoading(button, false);
    }
  }

  async function refreshCluster(button) {
    setLoading(button, !!button);
    const container = document.getElementById("clusterCards");
    if (!container) return;

    try {
      const { data } = await request("/cluster/status");
      if (!Array.isArray(data.slaves) || data.slaves.length === 0) {
        container.innerHTML = `<div class="mini-card"><div class="muted">No slaves are registered yet.</div></div>`;
        return;
      }

      container.innerHTML = data.slaves.map((slave) => `
        <article class="mini-card">
          <div class="mini-head">
            <strong>${escapeHTML(slave.id || slave.url)}</strong>
            <span class="pill ${slave.healthy ? "healthy" : "unhealthy"}">${slave.healthy ? "Healthy" : "Unhealthy"}</span>
          </div>
          <div class="muted">URL: ${escapeHTML(slave.url)}</div>
          <div class="muted">Pending replications: ${escapeHTML(slave.pendingCount ?? 0)}</div>
          <div class="muted">Last checked: ${escapeHTML(prettyDate(slave.lastChecked))}</div>
          <div class="muted">${escapeHTML(slave.lastError || "No recent replication errors.")}</div>
        </article>
      `).join("");
    } catch (error) {
      container.innerHTML = `<div class="mini-card"><div class="muted">${escapeHTML(error.message || "Could not load cluster status.")}</div></div>`;
    } finally {
      setLoading(button, false);
    }
  }

  async function retryReplication(button) {
    setLoading(button, true);
    try {
      clearOutput();
      const { data } = await request("/replication/retry", { method: "POST" });
      addMessage("Retry completed", data.message || "Pending replication jobs were retried.", "success");
      renderReplication(data.replication);
      refreshCluster();
    } catch (error) {
      clearOutput();
      addMessage("Retry failed", error.message || "Pending replications could not be retried.", "error");
    } finally {
      setLoading(button, false);
    }
  }

  async function resyncReplica(button) {
    const confirmed = window.confirm("Pull a fresh replica from the master and replace the local replica data?");
    if (!confirmed) return;

    setLoading(button, true);
    try {
      clearOutput();
      const { data } = await request("/replication/resync", { method: "POST" });
      addMessage("Replica resynced", data.message || "The slave pulled a fresh snapshot from the master.", "success");
      await refreshHealth();
    } catch (error) {
      clearOutput();
      addMessage("Resync failed", error.message || "The replica could not be refreshed from the master.", "error");
    } finally {
      setLoading(button, false);
    }
  }

  async function refreshApprovals(button) {
    setLoading(button, !!button);
    const list = document.getElementById("approvalList");
    if (!list) return;

    try {
      const filter = document.getElementById("approvalFilter").value;
      const { data } = await request(`/approval-requests?status=${encodeURIComponent(filter)}`);
      if (!Array.isArray(data.requests) || data.requests.length === 0) {
        list.innerHTML = `<div class="mini-card"><div class="muted">No approval requests found for this filter.</div></div>`;
        return;
      }

      list.innerHTML = data.requests.map((item) => `
        <article class="request-card" data-approval-id="${escapeHTML(item.id)}">
          <div class="request-head">
            <strong>${escapeHTML(item.method)} ${escapeHTML(item.path)}</strong>
            <span class="pill ${escapeHTML(item.status)}">${escapeHTML(item.status)}</span>
          </div>
          <div class="muted">ID: ${escapeHTML(item.id)}</div>
          <div class="muted">Requested by: ${escapeHTML(item.requestedBy || "unknown node")}</div>
          <div class="muted">Created: ${escapeHTML(prettyDate(item.createdAt))}</div>
          <div class="muted">Reason: ${escapeHTML(item.reason || "No reason provided.")}</div>
          <pre>${escapeHTML(item.body || "No body")}</pre>
          ${item.status === "pending" ? `
            <div class="field-full" style="margin-top:12px;">
              <label>Decision note</label>
              <input class="decision-reason" placeholder="Optional reason for approval or rejection">
            </div>
            <div class="actions">
              <button class="success approval-action" data-decision="approve">Approve</button>
              <button class="danger approval-action" data-decision="reject">Reject</button>
            </div>
          ` : ""}
        </article>
      `).join("");
    } catch (error) {
      list.innerHTML = `<div class="mini-card"><div class="muted">${escapeHTML(error.message || "Could not load approval requests.")}</div></div>`;
    } finally {
      setLoading(button, false);
    }
  }

  async function submitApprovalDecision(card, decision, button) {
    const approvalID = card.dataset.approvalId;
    const reason = card.querySelector(".decision-reason")?.value.trim() || "";
    setLoading(button, true);
    try {
      clearOutput();
      const { data } = await request(`/approval-requests/${approvalID}/${decision}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason })
      });
      addMessage(
        decision === "approve" ? "Request approved" : "Request rejected",
        data.message || "The approval request was processed successfully.",
        "success"
      );
      if (data.response?.rows) {
        renderRows(data.response.rows);
      }
      if (data.response?.replication) {
        renderReplication(data.response.replication);
      }
      refreshApprovals();
      refreshCluster();
    } catch (error) {
      clearOutput();
      addMessage("Decision failed", error.message || "The approval request could not be processed.", "error");
    } finally {
      setLoading(button, false);
    }
  }

  async function promoteNode(button) {
    const confirmed = window.confirm("Promote this node to master?");
    if (!confirmed) return;

    setLoading(button, true);
    try {
      clearOutput();
      const { data } = await request("/promote", { method: "PUT" });
      addMessage("Node promoted", data.message || "This node is now acting as master.", "success");
      await refreshHealth();
    } catch (error) {
      clearOutput();
      addMessage("Promotion failed", error.message || "The node could not be promoted.", "error");
    } finally {
      setLoading(button, false);
    }
  }

  function initShared() {
    document.querySelectorAll("[data-query-action]").forEach((button) => {
      button.addEventListener("click", () => runQueryAction(button.dataset.queryAction, button));
    });

    document.getElementById("createTableForm")?.addEventListener("submit", handleCreateTable);
    document.getElementById("dropTableForm")?.addEventListener("submit", handleDropTable);
    document.getElementById("refreshHealth")?.addEventListener("click", refreshHealth);
    document.getElementById("resyncReplica")?.addEventListener("click", () => resyncReplica(document.getElementById("resyncReplica")));
    document.getElementById("promoteButton")?.addEventListener("click", () => promoteNode(document.getElementById("promoteButton")));
  }

  function initMaster() {
    document.getElementById("registerSlaveForm")?.addEventListener("submit", handleRegisterSlave);
    document.getElementById("refreshCluster")?.addEventListener("click", (event) => refreshCluster(event.currentTarget));
    document.getElementById("retryReplication")?.addEventListener("click", (event) => retryReplication(event.currentTarget));
    document.getElementById("refreshApprovals")?.addEventListener("click", (event) => refreshApprovals(event.currentTarget));
    document.getElementById("approvalFilter")?.addEventListener("change", () => refreshApprovals());
    document.getElementById("dropDatabaseButton")?.addEventListener("click", (event) => handleDropDatabase(event.currentTarget));

    document.getElementById("approvalList")?.addEventListener("click", (event) => {
      const button = event.target.closest(".approval-action");
      if (!button) return;
      event.preventDefault();
      const card = button.closest("[data-approval-id]");
      submitApprovalDecision(card, button.dataset.decision, button);
    });

    refreshCluster();
    refreshApprovals();
  }

  initShared();
  refreshHealth();

  if (page === "master") {
    initMaster();
  }
})();
