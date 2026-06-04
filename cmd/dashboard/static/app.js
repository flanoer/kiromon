/* Kiromon Dashboard frontend.
   Vanilla JS — no framework. Talks to the local Go server's /api/* endpoints. */

(function () {
    "use strict";

    const state = {
        activityChart: null,
        projectsChart: null,
        activityDays: 30,
        projectsDays: 30,
    };

    function $(sel) { return document.querySelector(sel); }
    function $$(sel) { return document.querySelectorAll(sel); }

    async function fetchJSON(url) {
        const resp = await fetch(url);
        if (!resp.ok) throw new Error(url + " → HTTP " + resp.status);
        return resp.json();
    }

    function formatActiveTime(minutes) {
        if (minutes == null || isNaN(minutes)) return "—";
        const h = Math.floor(minutes / 60);
        const m = Math.round(minutes % 60);
        return h > 0 ? `${h}h ${m}m` : `${m}m`;
    }

    function formatPct(v) {
        return v == null ? "N/A" : v.toFixed(1) + "%";
    }

    function formatDays(d) {
        return d == null ? "N/A" : "~" + d.toFixed(1) + "d";
    }

    function formatNum(v, digits) {
        if (v == null || isNaN(v)) return "—";
        return Number(v).toFixed(digits ?? 1);
    }

    function applyProgress(fillEl, textEl, pct) {
        if (pct == null) {
            fillEl.style.width = "0%";
            fillEl.classList.remove("warn", "danger");
            textEl.textContent = "N/A";
            return;
        }
        const clamped = Math.min(100, Math.max(0, pct));
        fillEl.style.width = clamped + "%";
        fillEl.classList.toggle("warn", pct >= 70 && pct < 90);
        fillEl.classList.toggle("danger", pct >= 90);
        textEl.textContent = pct.toFixed(1) + "%";
    }

    function showError(msg) {
        let banner = $(".error-banner");
        if (!banner) {
            banner = document.createElement("div");
            banner.className = "error-banner";
            document.querySelector("main").prepend(banner);
        }
        banner.textContent = "⚠️ " + msg;
    }

    function clearError() {
        const banner = $(".error-banner");
        if (banner) banner.remove();
    }

    /* ---------- Data loaders ---------- */

    async function loadToday() {
        const d = await fetchJSON("/api/today");
        $("#today-sessions").textContent = d.sessions;
        $("#today-messages").textContent = d.messages;
        $("#today-active").textContent = formatActiveTime(d.active_minutes);
        $("#week-messages").textContent = d.this_week_messages;

        applyProgress($("#cli-fill"), $("#cli-text"), d.cli_usage_pct);
        applyProgress($("#ide-fill"), $("#ide-text"), d.ide_usage_pct);
    }

    async function loadForecast() {
        const d = await fetchJSON("/api/forecast");
        $("#cli-days").textContent = formatDays(d.cli_days_left);
        $("#ide-days").textContent = formatDays(d.ide_days_left);
        $("#calendar-days").textContent = d.calendar_days_left + "d";

        const cli = d.cli_days_left;
        const cal = d.calendar_days_left;
        let hint = "";
        if (cli != null && cal != null) {
            if (cli < cal) {
                hint = `현재 추세로는 청구 사이클 종료(${cal}d) 전에 한도 도달할 수 있습니다.`;
            } else {
                hint = `현재 추세는 청구 사이클(${cal}d) 안전 범위입니다.`;
            }
        }
        $("#forecast-hint").textContent = hint;
    }

    async function loadHistory(days) {
        const d = await fetchJSON("/api/history?days=" + days);
        renderActivityChart(d.records || []);
    }

    async function loadAggregates() {
        const periods = [
            { id: "agg-week",  period: "week",  title: "Week" },
            { id: "agg-month", period: "month", title: "Month" },
            { id: "agg-year",  period: "year",  title: "Year" },
        ];
        for (const p of periods) {
            const d = await fetchJSON("/api/aggregate?period=" + p.period);
            renderAggTable(p.id, d);
        }
    }

    async function loadProjects(days) {
        const d = await fetchJSON("/api/projects?days=" + days + "&limit=10");
        renderProjects(d.projects || []);
    }

    /* ---------- Renderers ---------- */

    function renderActivityChart(records) {
        const labels = records.map(r => r.date);
        const messages = records.map(r => r.messages);
        const sessions = records.map(r => r.sessions);
        const active = records.map(r => r.active_minutes);

        const ctx = $("#activity-chart").getContext("2d");
        const fg = getComputedStyle(document.documentElement).getPropertyValue("--fg").trim();
        const grid = getComputedStyle(document.documentElement).getPropertyValue("--border").trim();

        if (state.activityChart) state.activityChart.destroy();
        state.activityChart = new Chart(ctx, {
            type: "bar",
            data: {
                labels,
                datasets: [
                    { label: "Messages", data: messages, backgroundColor: "#2563eb", yAxisID: "y" },
                    { label: "Sessions", data: sessions, backgroundColor: "#10b981", yAxisID: "y" },
                    { label: "Active (min)", data: active, type: "line", borderColor: "#f59e0b", backgroundColor: "#f59e0b", yAxisID: "y1", tension: 0.25 },
                ],
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                interaction: { mode: "index", intersect: false },
                scales: {
                    x: { ticks: { color: fg, maxRotation: 0, autoSkip: true, maxTicksLimit: 12 }, grid: { color: grid } },
                    y: { position: "left", ticks: { color: fg }, grid: { color: grid }, beginAtZero: true },
                    y1: { position: "right", ticks: { color: fg }, grid: { drawOnChartArea: false }, beginAtZero: true },
                },
                plugins: {
                    legend: { labels: { color: fg } },
                },
            },
        });
    }

    function renderAggTable(tableId, d) {
        const table = document.getElementById(tableId);
        const rows = [
            ["Avg sessions/day",  formatNum(d.avg_sessions, 1)],
            ["Avg messages/day",  formatNum(d.avg_messages, 1)],
            ["Avg active min/day", formatNum(d.avg_active_min, 0)],
            ["Peak sessions",     `${d.peak_sessions} (${d.peak_sessions_date || "—"})`],
            ["Peak messages",     `${d.peak_messages} (${d.peak_messages_date || "—"})`],
            ["Peak active min",   `${d.peak_active_min} (${d.peak_active_date || "—"})`],
            ["Sample size (days)", d.sample_size],
        ];
        table.innerHTML = rows.map(r => `<tr><td>${r[0]}</td><td>${r[1]}</td></tr>`).join("");
    }

    function renderProjects(projects) {
        const labels = projects.map(p => p.project);
        const messages = projects.map(p => p.messages);
        const ctx = $("#projects-chart").getContext("2d");
        const fg = getComputedStyle(document.documentElement).getPropertyValue("--fg").trim();
        const grid = getComputedStyle(document.documentElement).getPropertyValue("--border").trim();

        if (state.projectsChart) state.projectsChart.destroy();
        state.projectsChart = new Chart(ctx, {
            type: "bar",
            data: {
                labels,
                datasets: [{ label: "Messages", data: messages, backgroundColor: "#8b5cf6" }],
            },
            options: {
                indexAxis: "y",
                responsive: true,
                maintainAspectRatio: false,
                scales: {
                    x: { ticks: { color: fg }, grid: { color: grid }, beginAtZero: true },
                    y: { ticks: { color: fg }, grid: { color: grid } },
                },
                plugins: { legend: { display: false } },
            },
        });

        const table = $("#projects-table");
        if (!projects.length) {
            table.innerHTML = `<tr><td class="muted">No project data yet for this range.</td></tr>`;
            return;
        }
        const rows = projects.map(p => {
            const last = p.last_active ? new Date(p.last_active).toLocaleString() : "—";
            return `<tr>
                <td>${escapeHTML(p.project)}</td>
                <td class="num">${p.sessions}</td>
                <td class="num">${p.messages}</td>
                <td class="last">${escapeHTML(last)}</td>
            </tr>`;
        });
        table.innerHTML =
            `<tr><th>Project</th><th class="num">Sessions</th><th class="num">Messages</th><th>Last active</th></tr>` +
            rows.join("");
    }

    function escapeHTML(s) {
        return String(s).replace(/[&<>"']/g, c => ({
            "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
        }[c]));
    }

    /* ---------- Wiring ---------- */

    async function refreshAll() {
        clearError();
        try {
            await Promise.all([
                loadToday(),
                loadForecast(),
                loadHistory(state.activityDays),
                loadAggregates(),
                loadProjects(state.projectsDays),
            ]);
            $("#last-updated").textContent = "Updated: " + new Date().toLocaleTimeString();
        } catch (err) {
            console.error(err);
            showError(String(err));
        }
    }

    function setupRangeToggles() {
        $$(".range-toggle button[data-days]").forEach(btn => {
            btn.addEventListener("click", () => {
                $$(".range-toggle button[data-days]").forEach(b => b.classList.remove("active"));
                btn.classList.add("active");
                state.activityDays = Number(btn.dataset.days);
                loadHistory(state.activityDays).catch(e => showError(String(e)));
            });
        });
        $$(".range-toggle button[data-pdays]").forEach(btn => {
            btn.addEventListener("click", () => {
                $$(".range-toggle button[data-pdays]").forEach(b => b.classList.remove("active"));
                btn.classList.add("active");
                state.projectsDays = Number(btn.dataset.pdays);
                loadProjects(state.projectsDays).catch(e => showError(String(e)));
            });
        });
    }

    document.addEventListener("DOMContentLoaded", () => {
        setupRangeToggles();
        $("#refresh-btn").addEventListener("click", refreshAll);
        refreshAll();
        // Auto-refresh today/forecast every 60s; full refresh every 5m.
        setInterval(() => loadToday().catch(() => {}), 60_000);
        setInterval(() => loadForecast().catch(() => {}), 60_000);
        setInterval(refreshAll, 300_000);
    });
})();
