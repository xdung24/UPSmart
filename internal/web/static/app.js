"use strict";

const fields = [
  ["input_voltage", "Input voltage"],
  ["output_voltage", "Output voltage"],
  ["input_frequency", "Input frequency"],
  ["load", "Load"],
  ["battery_voltage", "Battery voltage"],
  ["ups_temperature", "UPS temperature (sensor location unverified)"],
  ["battery_capacity", "Battery capacity"]
];
const readings = document.getElementById("readings");
for (const [key, label] of fields) {
  const card = document.createElement("article");
  const title = document.createElement("h3");
  title.textContent = label;
  const value = document.createElement("p");
  value.id = key;
  value.textContent = "Unavailable";
  card.append(title, value);
  readings.append(card);
}

async function getJSON(path) {
  const response = await fetch(path, { cache: "no-store", signal: AbortSignal.timeout(5000) });
  if (!response.ok) throw new Error(`Local API returned HTTP ${response.status}`);
  return response.json();
}

async function refresh() {
  try {
    const snapshot = await getJSON("/api/v1/status");
    const telemetry = snapshot.telemetry;
    const unhealthy = snapshot.connection.state !== "connected" || !telemetry || telemetry.stale;
    const connection = document.getElementById("connection");
    connection.textContent = `${snapshot.connection.state}${telemetry && telemetry.stale ? " - STALE DATA" : ""}`;
    connection.className = unhealthy ? "warning" : "healthy";
    document.getElementById("device").textContent = snapshot.device
      ? `Device: ${snapshot.device.manufacturer} ${snapshot.device.product} (${snapshot.device.vendor_id}:${snapshot.device.product_id})`
      : "Device: not identified";
    const flag = value => value === null ? "unknown" : value ? "yes" : "no";
    document.getElementById("power").textContent = `Power: ${snapshot.power.mode} | Low battery: ${flag(snapshot.power.low_battery)} | Fault: ${flag(snapshot.power.fault)}${unhealthy ? " (not currently verified)" : ""}`;
    document.getElementById("updated").textContent = `Last successful update: ${snapshot.connection.last_success || "none"}`;
    const history = snapshot.health.event_history;
    const telegram = snapshot.health.telegram;
    const policy = snapshot.policy;
    const policyStatus = document.getElementById("policy-status");
    const policySettings = [];
    if (policy.automatic_enabled) {
      const triggers = [];
      if (policy.outage_after) triggers.push(`outage after ${policy.outage_after}`);
      if (policy.low_battery_enabled) triggers.push("verified low battery");
      policySettings.push(`Automatic: ${triggers.join(" or ")}; countdown ${policy.automatic_countdown}`);
    }
    if (policy.manual_enabled) policySettings.push(`Desktop manual countdown: ${policy.manual_countdown}`);
    const remaining = policy.deadline
      ? Math.max(0, Math.ceil((Date.parse(policy.deadline) - Date.parse(snapshot.generated_at)) / 1000))
      : null;
    policyStatus.textContent = !policy.enabled
      ? "Disabled. No PC shutdown requests."
      : `${policy.dry_run ? "DRY RUN - no Windows shutdown" : "LIVE - Windows shutdown enabled"} | ${policy.state}${policySettings.length ? ` | ${policySettings.join(" | ")}` : ""}${policy.trigger ? ` | Trigger: ${policy.trigger}` : ""}${remaining !== null ? ` | Countdown: ${remaining}s (as of last refresh); deadline: ${policy.deadline}` : ""}${policy.blocking_reason ? ` | ${policy.blocking_reason}` : ""}${policy.outcome ? ` | ${policy.outcome}` : ""}`;
    policyStatus.className = policy.enabled && !policy.dry_run ? "warning" : "";
    const commands = snapshot.health.telegram_commands;
    document.getElementById("command-status").textContent = !commands || commands.mode === "disabled"
      ? "Telegram checking commands disabled."
      : `Telegram commands: ${commands.mode}${commands.error ? ` - ${commands.error}` : ""}${commands.rejected_commands ? ` | Rejected inputs: ${commands.rejected_commands}` : ""}`;
    document.getElementById("telegram-status").textContent = !telegram || telegram.mode === "disabled"
      ? "Disabled. No messages are sent."
      : `${telegram.mode}${telegram.error ? ` - ${telegram.error}` : ""}${telegram.last_success ? ` | Last API acceptance: ${telegram.last_success}` : ""}${telegram.dropped_events ? ` | Undelivered events: ${telegram.dropped_events}` : ""}`;
    document.getElementById("history-status").textContent = !history || history.mode === "memory_only"
      ? "Memory-only history; resets when the process exits."
      : history.mode === "persistent"
        ? `Persistent local history${history.unpersisted_events ? `; ${history.unpersisted_events} earlier event(s) were not saved` : ""}.`
        : `Event persistence degraded: ${history.error || "storage unavailable"}`;
    document.getElementById("warnings").replaceChildren(...snapshot.health.warnings.map(message => {
      const item = document.createElement("li");
      item.textContent = message;
      return item;
    }));
    for (const [key] of fields) {
      const m = telemetry && telemetry.measurements[key];
      document.getElementById(key).textContent = m && m.value !== null
        ? `${m.value} ${m.unit}${m.quality === "estimated" ? " (estimated)" : ""}${unhealthy ? " (last known)" : ""}`
        : `Unavailable${m && m.reason ? ` - ${m.reason}` : ""}`;
    }
    try {
      const events = await getJSON("/api/v1/events?limit=20");
      document.getElementById("events").replaceChildren(...events.items.map(event => {
        const item = document.createElement("li");
        item.textContent = `${event.time} - ${event.message}`;
        return item;
      }));
    } catch (error) {
      document.getElementById("events").textContent = `Event history unavailable: ${error.message}`;
    }
  } catch (error) {
    const connection = document.getElementById("connection");
    connection.textContent = `Local server unavailable - ${error.message}`;
    connection.className = "warning";
    document.getElementById("power").textContent = "Power: unknown (API unavailable)";
    document.getElementById("telegram-status").textContent = "Notification status unavailable - API connection lost";
    document.getElementById("policy-status").textContent = "Shutdown policy status unknown - API connection lost; a countdown may still be active.";
    document.getElementById("policy-status").className = "warning";
    document.getElementById("command-status").textContent = "Telegram command status unavailable - API connection lost";
    for (const [key] of fields) document.getElementById(key).textContent = "Unavailable - API connection lost";
  } finally {
    window.setTimeout(refresh, document.hidden ? 10000 : 2000);
  }
}
refresh();

function hardwareLabel(key) {
  return key.replaceAll("_", " ").replace(/\b\w/g, letter => letter.toUpperCase());
}

function hardwareDetails(value, prefix = "") {
  const rows = [];
  for (const [key, item] of Object.entries(value || {})) {
    const label = `${prefix}${hardwareLabel(key)}`;
    if (Array.isArray(item) && item.some(entry => entry !== null && typeof entry === "object")) {
      for (const [index, entry] of item.entries()) {
        rows.push(...hardwareDetails(entry, `${label} ${index + 1} / `));
      }
    } else if (item !== null && typeof item === "object" && !Array.isArray(item)) {
      if (Object.hasOwn(item, "value")) {
        rows.push([label, item.value === null
          ? `Unavailable${item.reason ? ` - ${item.reason}` : ""}`
          : `${item.value}${item.unit ? ` ${item.unit}` : ""}`]);
      } else {
        rows.push(...hardwareDetails(item, `${label} / `));
      }
    } else {
      rows.push([label, item === null ? "Unavailable" : Array.isArray(item) ? item.join(", ") : String(item)]);
    }
  }
  return rows;
}

async function refreshHardware() {
  const status = document.getElementById("hardware-status");
  try {
    const snapshot = await getJSON("/api/v1/hardware");
    const data = snapshot.data;
    const warnings = data ? [
      ...(data.warnings || []), data.gpus_reason,
      data.disks && data.disks.volumes_reason, data.disks && data.disks.physical_reason
    ].filter(Boolean) : [];
    status.textContent = `${snapshot.mode}${snapshot.stale ? " - no fresh sample" : ""}${snapshot.sampled_at ? ` | Last sample: ${snapshot.sampled_at}` : ""}${snapshot.error ? ` | ${snapshot.error}` : ""}${warnings.length ? ` | ${warnings.join("; ")}` : ""}`;
    status.className = snapshot.stale || snapshot.mode === "degraded" ? "warning" : "";
    const cards = [];
    if (snapshot.data) {
      for (const key of ["cpu", "gpus", "disks"]) {
        const group = snapshot.data[key];
        const devices = Array.isArray(group) ? group : group ? [group] : [];
        for (const [index, device] of devices.entries()) {
          const card = document.createElement("article");
          const title = document.createElement("h3");
          title.textContent = `${hardwareLabel(key)}${devices.length > 1 ? ` ${index + 1}` : ""}`;
          const details = document.createElement("dl");
          for (const [label, value] of hardwareDetails(device)) {
            const term = document.createElement("dt");
            term.textContent = label;
            const description = document.createElement("dd");
            description.textContent = value;
            details.append(term, description);
          }
          card.append(title, details);
          cards.push(card);
        }
      }
    }
    document.getElementById("hardware-readings").replaceChildren(...cards);
  } catch (error) {
    status.textContent = `Hardware API unavailable - displayed readings are not current: ${error.message}`;
    status.className = "warning";
  } finally {
    window.setTimeout(refreshHardware, document.hidden ? 10000 : 2000);
  }
}
refreshHardware();
