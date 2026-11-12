const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const script = fs.readFileSync(path.join(__dirname, "static", "app.js"), "utf8");
const html = fs.readFileSync(path.join(__dirname, "static", "index.html"), "utf8");

function element() {
  return {
    textContent: "", children: [], className: "",
    append(...children) { this.children.push(...children); },
    replaceChildren(...children) { this.children = children; },
    set innerHTML(_) { throw new Error("Untrusted HTML must not be interpreted"); }
  };
}

async function render(policy, unavailable = false) {
  const elements = new Map([...html.matchAll(/id="([^"]+)"/g)].map(match => [match[1], element()]));
  const snapshot = {
    generated_at: "2026-11-12T12:00:00Z",
    connection: { state: "connected", last_success: "2026-11-12T12:00:00Z" },
    telemetry: { stale: false, measurements: { ups_temperature: { value: 29, unit: "C" } } },
    power: { mode: "online", low_battery: false, fault: false },
    health: { warnings: [], telegram: { mode: "disabled" }, telegram_commands: { mode: "disabled" } },
    policy
  };
  vm.runInNewContext(script, {
    document: {
      hidden: false,
      getElementById(id) { assert.ok(elements.has(id), `Missing element ${id}`); return elements.get(id); },
      createElement() {
        const node = element();
        Object.defineProperty(node, "id", { set(id) { elements.set(id, node); } });
        return node;
      }
    },
    window: { setTimeout() {} },
    AbortSignal,
    fetch: async url => {
      if (url === "/api/v1/status" && unavailable) throw new Error("offline");
      const body = url === "/api/v1/status" ? snapshot
        : url.startsWith("/api/v1/events") ? { items: [] }
          : { mode: "disabled", data: null, stale: true };
      return { ok: true, json: async () => body };
    }
  });
  await new Promise(resolve => setImmediate(resolve));
  return elements;
}

test("policy rendering distinguishes disabled, dry-run, live countdown, cancellation and OS failure", async () => {
  for (const [policy, expected] of [
    [{ enabled: false, state: "disabled" }, ["Disabled", "No PC shutdown"]],
    [{ enabled: true, dry_run: true, state: "pending", automatic_enabled: true, outage_after: "5m", low_battery_enabled: true, automatic_countdown: "30s", manual_enabled: true, manual_countdown: "20s", deadline: "2026-11-12T12:00:30Z" }, ["DRY RUN", "Countdown: 30s", "outage after 5m or verified low battery; countdown 30s", "Desktop manual countdown: 20s"]],
    [{ enabled: true, dry_run: false, state: "pending", deadline: "2026-11-12T12:00:30Z", trigger: "<script>unsafe</script>" }, ["LIVE", "Countdown: 30s", "<script>unsafe</script>"]],
    [{ enabled: true, dry_run: false, state: "cancelled", deadline: null }, ["LIVE", "cancelled"]],
    [{ enabled: true, dry_run: false, state: "failed", outcome: "Windows request denied" }, ["failed", "Windows request denied"]]
  ]) {
    const elements = await render(policy);
    const text = elements.get("policy-status").textContent;
    for (const word of expected) assert.ok(text.includes(word), `${word} missing from ${text}`);
    if (policy.state === "cancelled") assert.ok(!text.includes("Countdown:"));
    assert.equal(elements.get("policy-status").className, policy.enabled && !policy.dry_run ? "warning" : "");
    assert.equal(elements.get("ups_temperature").textContent, "29 C");
    assert.ok(elements.get("command-status").textContent.includes("disabled"));
  }
});

test("API loss invalidates policy display without suggesting countdown has stopped", async () => {
  const elements = await render({ enabled: true, state: "pending" }, true);
  assert.ok(elements.get("policy-status").textContent.includes("unknown"));
  assert.ok(elements.get("policy-status").textContent.includes("may still be active"));
  assert.equal(elements.get("policy-status").className, "warning");
});
