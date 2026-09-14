// Spectra Client Logic - Zero Dependencies, Minimalist & Efficient
(function () {
  'use strict';

  // Config & State
  let currentInterval = 2000; // default 2s
  let timerId = null;
  let isFetching = false;
  let showCores = false;

  const MAX_HISTORY = 30;
  const history = {
    cpu: new Array(MAX_HISTORY).fill(0),
    mem: new Array(MAX_HISTORY).fill(0),
    netRx: new Array(MAX_HISTORY).fill(0),
    netTx: new Array(MAX_HISTORY).fill(0)
  };

  // DOM Elements
  const hostPill = document.getElementById('host-pill');
  const statusIndicator = document.getElementById('status-indicator');
  const statusText = document.getElementById('status-text');
  const latencyLabel = document.getElementById('response-latency');
  const btnManualRefresh = document.getElementById('btn-manual-refresh');
  const tickButtons = document.querySelectorAll('.tick-btn');

  // CPU
  const cpuPercent = document.getElementById('cpu-percent');
  const cpuBar = document.getElementById('cpu-bar');
  const cpuModel = document.getElementById('cpu-model');
  const cpuCores = document.getElementById('cpu-cores');
  const cpuFreq = document.getElementById('cpu-freq');
  const cpuLoad = document.getElementById('cpu-load');
  const toggleCoresBtn = document.getElementById('toggle-cores-btn');
  const coresGrid = document.getElementById('cores-grid');
  const coresCountLabel = document.getElementById('cores-count-label');

  // Memory
  const memPercent = document.getElementById('mem-percent');
  const memBar = document.getElementById('mem-bar');
  const ramBreakdown = document.getElementById('ram-breakdown');
  const memUsed = document.getElementById('mem-used');
  const memAvail = document.getElementById('mem-avail');
  const memSwap = document.getElementById('mem-swap');

  // Network
  const netRxRate = document.getElementById('net-rx-rate');
  const netTxRate = document.getElementById('net-tx-rate');
  const netRxTotal = document.getElementById('net-rx-total');
  const netTxTotal = document.getElementById('net-tx-total');
  const netRxPackets = document.getElementById('net-rx-packets');
  const netTxPackets = document.getElementById('net-tx-packets');

  // Storage
  const diskList = document.getElementById('disk-list');

  // System Specifications Card
  const sysHostname = document.getElementById('sys-hostname');
  const sysPlatform = document.getElementById('sys-platform');
  const sysKernel = document.getElementById('sys-kernel');
  const sysArch = document.getElementById('sys-arch');
  const sysUptime = document.getElementById('sys-uptime');
  const sysProcs = document.getElementById('sys-procs');
  const sysBootTime = document.getElementById('sys-boottime');
  const sysLastPolled = document.getElementById('sys-last-polled');
  const sysHeaderUptime = document.getElementById('sys-header-uptime');

  // Canvas elements
  const cpuCanvas = document.getElementById('cpu-chart');
  const memCanvas = document.getElementById('mem-chart');
  const netCanvas = document.getElementById('net-chart');

  // Utility: Format bytes to human readable (GB, MB, etc)
  function formatBytes(bytes, decimals = 1) {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
  }

  // Utility: Format rate to KB/s, MB/s
  function formatRate(bps) {
    if (!bps || bps < 1) return { val: '0.0', unit: 'KB/s' };
    if (bps < 1024 * 1024) {
      return { val: (bps / 1024).toFixed(1), unit: 'KB/s' };
    }
    return { val: (bps / (1024 * 1024)).toFixed(2), unit: 'MB/s' };
  }

  // Draw clean sparkline chart on canvas
  function drawSparkline(canvas, dataSeries, options = {}) {
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    const rect = canvas.getBoundingClientRect();

    if (canvas.width !== rect.width * dpr || canvas.height !== rect.height * dpr) {
      canvas.width = rect.width * dpr;
      canvas.height = rect.height * dpr;
    }

    ctx.save();
    ctx.scale(dpr, dpr);
    const width = rect.width;
    const height = rect.height;

    ctx.clearRect(0, 0, width, height);

    // Grid baseline
    ctx.strokeStyle = '#f1f5f9';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(0, height - 1);
    ctx.lineTo(width, height - 1);
    ctx.stroke();

    const maxVal = options.max || Math.max(...dataSeries, 100);
    const minVal = 0;
    const range = maxVal - minVal || 1;

    // Draw area gradient & stroke
    const step = width / (dataSeries.length - 1);

    ctx.beginPath();
    for (let i = 0; i < dataSeries.length; i++) {
      const x = i * step;
      const normalized = (dataSeries[i] - minVal) / range;
      const y = height - (normalized * (height - 6)) - 3;
      if (i === 0) {
        ctx.moveTo(x, y);
      } else {
        ctx.lineTo(x, y);
      }
    }

    // Color choices
    const color = options.color || '#0284c7';
    ctx.strokeStyle = color;
    ctx.lineWidth = 2;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.stroke();

    // Subtle fill
    ctx.lineTo(width, height);
    ctx.lineTo(0, height);
    ctx.closePath();
    const grad = ctx.createLinearGradient(0, 0, 0, height);
    grad.addColorStop(0, options.fillColor || 'rgba(2, 132, 199, 0.12)');
    grad.addColorStop(1, 'rgba(2, 132, 199, 0.00)');
    ctx.fillStyle = grad;
    ctx.fill();

    ctx.restore();
  }

  // Draw dual network sparkline (Rx down, Tx up)
  function drawNetSparkline(canvas, rxSeries, txSeries) {
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    const rect = canvas.getBoundingClientRect();

    if (canvas.width !== rect.width * dpr || canvas.height !== rect.height * dpr) {
      canvas.width = rect.width * dpr;
      canvas.height = rect.height * dpr;
    }

    ctx.save();
    ctx.scale(dpr, dpr);
    const width = rect.width;
    const height = rect.height;

    ctx.clearRect(0, 0, width, height);

    const maxVal = Math.max(...rxSeries, ...txSeries, 1024);
    const step = width / (rxSeries.length - 1);

    function plotLine(series, strokeColor, fillColor) {
      ctx.beginPath();
      for (let i = 0; i < series.length; i++) {
        const x = i * step;
        const normalized = series[i] / maxVal;
        const y = height - (normalized * (height - 6)) - 3;
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      }
      ctx.strokeStyle = strokeColor;
      ctx.lineWidth = 1.8;
      ctx.stroke();

      ctx.lineTo(width, height);
      ctx.lineTo(0, height);
      ctx.closePath();
      ctx.fillStyle = fillColor;
      ctx.fill();
    }

    // Rx (Blue) and Tx (Green)
    plotLine(rxSeries, '#0284c7', 'rgba(2, 132, 199, 0.08)');
    plotLine(txSeries, '#10b981', 'rgba(16, 185, 129, 0.08)');

    ctx.restore();
  }

  // Helper for progress bar color classes
  function updateBarState(barEl, percent) {
    barEl.style.width = Math.min(100, Math.max(0, percent)) + '%';
    barEl.classList.remove('warn', 'danger');
    if (percent >= 90) {
      barEl.classList.add('danger');
    } else if (percent >= 75) {
      barEl.classList.add('warn');
    }
  }

  // Render Disk partitions
  function renderDisks(disks) {
    if (!disks || disks.length === 0) {
      diskList.innerHTML = '<div class="disk-placeholder">No disk storage mounts detected</div>';
      return;
    }

    let html = '';
    disks.forEach(d => {
      const usedPercent = d.used_percent || 0;
      let colorClass = '';
      if (usedPercent >= 90) colorClass = 'danger';
      else if (usedPercent >= 75) colorClass = 'warn';

      html += `
        <div class="disk-row">
          <div class="disk-info">
            <span class="disk-mount">${d.mountpoint} <span style="font-weight: normal; color: var(--text-muted); font-size: 11px;">(${d.fstype})</span></span>
            <span class="disk-usage-text"><strong>${formatBytes(d.used)}</strong> / ${formatBytes(d.total)} (${usedPercent.toFixed(1)}%)</span>
          </div>
          <div class="disk-track">
            <div class="disk-fill ${colorClass}" style="width: ${Math.min(100, usedPercent)}%;"></div>
          </div>
        </div>
      `;
    });
    diskList.innerHTML = html;
  }

  // Render Per-Core CPU usage
  function renderCores(coresUsage) {
    if (!coresUsage || coresUsage.length === 0) return;
    coresCountLabel.textContent = coresUsage.length;

    let html = '';
    coresUsage.forEach((p, idx) => {
      html += `
        <div class="core-item">
          <div class="core-item-header">
            <span>C${idx}</span>
            <span>${p.toFixed(0)}%</span>
          </div>
          <div class="core-mini-track">
            <div class="core-mini-fill" style="width: ${Math.min(100, p)}%;"></div>
          </div>
        </div>
      `;
    });
    coresGrid.innerHTML = html;
  }

  // Main Fetch logic
  async function fetchStats() {
    if (isFetching) return;
    isFetching = true;
    const startTime = performance.now();

    btnManualRefresh.classList.add('spinning');

    try {
      // Cloudflare cache busting with timestamp
      const res = await fetch(`/api/stats?t=${Date.now()}`, {
        headers: { 'Cache-Control': 'no-cache' }
      });

      if (res.status === 401) {
        window.location.href = '/login';
        return;
      }
      if (!res.ok) throw new Error(`HTTP ${res.status}`);

      const data = await res.json();
      const latency = Math.round(performance.now() - startTime);
      latencyLabel.textContent = `Latency: ${latency}ms`;

      // Update connection status
      statusIndicator.classList.remove('offline');
      statusText.textContent = 'Connected';

      // 1. Host Info
      if (data.host) {
        const h = data.host;
        hostPill.textContent = h.hostname || 'Server';
        sysHostname.textContent = h.hostname || '-';
        sysPlatform.textContent = h.platform || h.os || '-';
        sysKernel.textContent = h.kernel_version || '-';
        sysArch.textContent = h.arch || '-';
        sysUptime.textContent = h.uptime_str || `${h.uptime}s`;
        if (sysHeaderUptime) {
          sysHeaderUptime.textContent = `Uptime: ${h.uptime_str || `${h.uptime}s`}`;
        }
        sysProcs.textContent = h.procs ? `${h.procs} Tasks` : '-';
        if (sysBootTime && h.boot_time) {
          const d = new Date(h.boot_time);
          sysBootTime.textContent = isNaN(d.getTime()) ? '-' : d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        }
        sysLastPolled.textContent = new Date().toLocaleTimeString();

        if (h.load_1 !== undefined && h.load_1 !== null) {
          cpuLoad.textContent = `${h.load_1} / ${h.load_5} / ${h.load_15}`;
        } else {
          cpuLoad.textContent = 'N/A';
        }
      }

      // 2. CPU
      if (data.cpu) {
        const c = data.cpu;
        cpuPercent.innerHTML = `${c.usage_percent.toFixed(1)}<span class="stat-unit">%</span>`;
        updateBarState(cpuBar, c.usage_percent);

        if (c.model_name && c.model_name !== 'Unknown CPU') {
          cpuModel.textContent = c.model_name;
        } else {
          cpuModel.textContent = `${c.cores_logical} Logical Cores`;
        }

        cpuCores.textContent = `${c.cores_physical || '-'} Phys / ${c.cores_logical || '-'} Log`;
        cpuFreq.textContent = c.mhz > 0 ? `${(c.mhz / 1000).toFixed(2)} GHz` : 'N/A';

        // History sparkline
        history.cpu.push(c.usage_percent);
        if (history.cpu.length > MAX_HISTORY) history.cpu.shift();
        drawSparkline(cpuCanvas, history.cpu, { max: 100, color: '#0284c7' });

        if (c.cores_usage) {
          renderCores(c.cores_usage);
        }
      }

      // 3. Memory
      if (data.memory) {
        const m = data.memory;
        memPercent.innerHTML = `${m.used_percent.toFixed(1)}<span class="stat-unit">%</span>`;
        updateBarState(memBar, m.used_percent);

        ramBreakdown.textContent = `${formatBytes(m.used)} / ${formatBytes(m.total)}`;
        memUsed.textContent = formatBytes(m.used);
        memAvail.textContent = formatBytes(m.available || m.free);

        if (m.swap_total > 0) {
          memSwap.textContent = `${formatBytes(m.swap_used)} (${m.swap_percent.toFixed(0)}%)`;
        } else {
          memSwap.textContent = 'None';
        }

        history.mem.push(m.used_percent);
        if (history.mem.length > MAX_HISTORY) history.mem.shift();
        drawSparkline(memCanvas, history.mem, { max: 100, color: '#6366f1', fillColor: 'rgba(99, 102, 241, 0.12)' });
      }

      // 4. Network
      if (data.network) {
        const n = data.network;
        const rx = formatRate(n.rx_rate_bps);
        const tx = formatRate(n.tx_rate_bps);

        netRxRate.innerHTML = `${rx.val} <span class="net-unit">${rx.unit}</span>`;
        netTxRate.innerHTML = `${tx.val} <span class="net-unit">${tx.unit}</span>`;

        netRxTotal.textContent = formatBytes(n.bytes_recv);
        netTxTotal.textContent = formatBytes(n.bytes_sent);

        netRxPackets.textContent = (n.packets_recv || 0).toLocaleString();
        netTxPackets.textContent = (n.packets_sent || 0).toLocaleString();

        history.netRx.push(n.rx_rate_bps || 0);
        history.netTx.push(n.tx_rate_bps || 0);
        if (history.netRx.length > MAX_HISTORY) {
          history.netRx.shift();
          history.netTx.shift();
        }
        drawNetSparkline(netCanvas, history.netRx, history.netTx);
      }

      // 5. Disks
      if (data.disks) {
        renderDisks(data.disks);
      }

    } catch (err) {
      console.error('Failed to fetch stats:', err);
      statusIndicator.classList.add('offline');
      statusText.textContent = 'Offline';
    } finally {
      isFetching = false;
      btnManualRefresh.classList.remove('spinning');
    }
  }

  // Timer Management
  function resetTimer() {
    if (timerId) {
      clearInterval(timerId);
      timerId = null;
    }
    if (currentInterval > 0) {
      timerId = setInterval(fetchStats, currentInterval);
    }
  }

  // Event Listeners for Tick Interval Buttons
  tickButtons.forEach(btn => {
    btn.addEventListener('click', () => {
      tickButtons.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');

      const interval = parseInt(btn.getAttribute('data-interval'), 10);
      currentInterval = interval;
      resetTimer();

      if (interval > 0) {
        fetchStats();
      }
    });
  });

  // Manual Refresh
  btnManualRefresh.addEventListener('click', () => {
    fetchStats();
  });

  // Toggle Per-Core View
  toggleCoresBtn.addEventListener('click', () => {
    showCores = !showCores;
    coresGrid.style.display = showCores ? 'grid' : 'none';
    toggleCoresBtn.querySelector('.toggle-icon').textContent = showCores ? '▴' : '▾';
  });

  // Logout
  const btnLogout = document.getElementById('btn-logout');
  if (btnLogout) {
    btnLogout.addEventListener('click', async () => {
      try {
        await fetch('/api/logout', { method: 'POST' });
      } finally {
        window.location.href = '/login';
      }
    });
  }

  // Function to redraw all canvas charts with current container dimensions
  function redrawCharts() {
    drawSparkline(cpuCanvas, history.cpu, { max: 100, color: '#0284c7' });
    drawSparkline(memCanvas, history.mem, { max: 100, color: '#6366f1', fillColor: 'rgba(99, 102, 241, 0.12)' });
    drawNetSparkline(netCanvas, history.netRx, history.netTx);
  }

  // Handle Window Resize and Orientation Change for crisp responsive canvas rendering
  window.addEventListener('resize', redrawCharts);
  window.addEventListener('orientationchange', redrawCharts);

  // ResizeObserver for modern fluid layouts
  if (typeof ResizeObserver !== 'undefined') {
    const ro = new ResizeObserver(() => {
      requestAnimationFrame(redrawCharts);
    });
    document.querySelectorAll('.chart-container').forEach(el => ro.observe(el));
  }

  // Initial Fetch & Start Polling
  fetchStats();
  resetTimer();
})();
