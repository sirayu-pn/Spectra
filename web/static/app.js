// Spectra Client Logic - Zero Dependencies, Minimalist & Efficient
// Memory-optimized: Float32Array ring buffers, zero-allocation DOM updates, SSE streaming, and Page Visibility API.
(function () {
  'use strict';

  // Config & State
  const MAX_HISTORY = 30;
  const history = {
    cpu: new Float32Array(MAX_HISTORY),
    mem: new Float32Array(MAX_HISTORY),
    netRx: new Float32Array(MAX_HISTORY),
    netTx: new Float32Array(MAX_HISTORY)
  };

  // Hardware-accelerated, zero-allocation ring buffer push
  function pushHistory(arr, val) {
    arr.copyWithin(0, 1);
    arr[MAX_HISTORY - 1] = val;
  }

  let eventSource = null;
  let isPageVisible = !document.hidden;
  let isManualFetching = false;
  let showCores = false;

  // DOM Elements
  const hostPill = document.getElementById('host-pill');
  const statusIndicator = document.getElementById('status-indicator');
  const statusText = document.getElementById('status-text');
  const latencyLabel = document.getElementById('response-latency');
  const btnManualRefresh = document.getElementById('btn-manual-refresh');

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

  // Canvas elements & Cached 2D Contexts (Zero context re-acquisition)
  const cpuCanvas = document.getElementById('cpu-chart');
  const memCanvas = document.getElementById('mem-chart');
  const netCanvas = document.getElementById('net-chart');

  const cpuCtx = cpuCanvas ? cpuCanvas.getContext('2d') : null;
  const memCtx = memCanvas ? memCanvas.getContext('2d') : null;
  const netCtx = netCanvas ? netCanvas.getContext('2d') : null;

  // Cached canvas dimensions to eliminate getBoundingClientRect() forced reflows in the hot path
  const canvasSizes = {
    cpu: { w: 0, h: 0, dpr: 1 },
    mem: { w: 0, h: 0, dpr: 1 },
    net: { w: 0, h: 0, dpr: 1 }
  };

  // Synchronize canvas buffer resolution with CSS display size (Called only on resize/init)
  function syncCanvasSize(canvas, sizeObj) {
    if (!canvas) return;
    const dpr = window.devicePixelRatio || 1;
    const rect = canvas.getBoundingClientRect();
    if (rect.width === 0 || rect.height === 0) return;

    sizeObj.w = rect.width;
    sizeObj.h = rect.height;
    sizeObj.dpr = dpr;

    const pixelW = Math.round(rect.width * dpr);
    const pixelH = Math.round(rect.height * dpr);

    if (canvas.width !== pixelW || canvas.height !== pixelH) {
      canvas.width = pixelW;
      canvas.height = pixelH;
    }
  }

  function syncAllCanvasSizes() {
    syncCanvasSize(cpuCanvas, canvasSizes.cpu);
    syncCanvasSize(memCanvas, canvasSizes.mem);
    syncCanvasSize(netCanvas, canvasSizes.net);
  }

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

  // Optimized Direct Text Node updates (Zero innerHTML parsing)
  function updatePercentNode(el, percent) {
    if (!el) return;
    if (el.firstChild && el.firstChild.nodeType === Node.TEXT_NODE) {
      el.firstChild.nodeValue = percent.toFixed(1);
    } else {
      el.innerHTML = `${percent.toFixed(1)}<span class="stat-unit">%</span>`;
    }
  }

  function updateRateNode(el, rateObj) {
    if (!el) return;
    if (el.firstChild && el.firstChild.nodeType === Node.TEXT_NODE) {
      el.firstChild.nodeValue = `${rateObj.val} `;
      const unitSpan = el.querySelector('.net-unit');
      if (unitSpan && unitSpan.textContent !== rateObj.unit) {
        unitSpan.textContent = rateObj.unit;
      }
    } else {
      el.innerHTML = `${rateObj.val} <span class="net-unit">${rateObj.unit}</span>`;
    }
  }

  // Draw clean sparkline chart on canvas using cached metrics
  function drawSparkline(ctx, canvas, sizeObj, dataSeries, options = {}) {
    if (!ctx || !canvas || sizeObj.w === 0) return;

    const dpr = sizeObj.dpr;
    const width = sizeObj.w;
    const height = sizeObj.h;

    ctx.save();
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, width, height);

    // Grid baseline
    ctx.strokeStyle = '#f1f5f9';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(0, height - 1);
    ctx.lineTo(width, height - 1);
    ctx.stroke();

    let maxVal = options.max || 100;
    for (let i = 0; i < dataSeries.length; i++) {
      if (dataSeries[i] > maxVal) maxVal = dataSeries[i];
    }
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

  // Draw dual network sparkline (Rx down, Tx up) using cached metrics
  function drawNetSparkline(ctx, canvas, sizeObj, rxSeries, txSeries) {
    if (!ctx || !canvas || sizeObj.w === 0) return;

    const dpr = sizeObj.dpr;
    const width = sizeObj.w;
    const height = sizeObj.h;

    ctx.save();
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, width, height);

    let maxVal = 1024;
    for (let i = 0; i < rxSeries.length; i++) {
      if (rxSeries[i] > maxVal) maxVal = rxSeries[i];
      if (txSeries[i] > maxVal) maxVal = txSeries[i];
    }

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

  // Render Disk partitions with in-place DOM reuse (avoids re-creating HTML elements)
  function renderDisks(disks) {
    if (!disks || disks.length === 0) {
      if (!diskList.querySelector('.disk-placeholder')) {
        diskList.innerHTML = '<div class="disk-placeholder">No disk storage mounts detected</div>';
      }
      return;
    }

    const existingRows = diskList.querySelectorAll('.disk-row');
    if (existingRows.length === disks.length && !diskList.querySelector('.disk-placeholder')) {
      // In-place update existing DOM elements
      disks.forEach((d, idx) => {
        const row = existingRows[idx];
        const usedPercent = d.used_percent || 0;
        const usageText = row.querySelector('.disk-usage-text');
        const fillBar = row.querySelector('.disk-fill');

        if (usageText) {
          usageText.innerHTML = `<strong>${formatBytes(d.used)}</strong> / ${formatBytes(d.total)} (${usedPercent.toFixed(1)}%)`;
        }
        if (fillBar) {
          fillBar.style.width = `${Math.min(100, usedPercent)}%`;
          fillBar.classList.remove('warn', 'danger');
          if (usedPercent >= 90) fillBar.classList.add('danger');
          else if (usedPercent >= 75) fillBar.classList.add('warn');
        }
      });
      return;
    }

    // Full render on mount count change or first load
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

  // Render Per-Core CPU usage with in-place DOM reuse
  function renderCores(coresUsage) {
    if (!coresUsage || coresUsage.length === 0) return;
    coresCountLabel.textContent = coresUsage.length;

    const existingItems = coresGrid.querySelectorAll('.core-item');
    if (existingItems.length === coresUsage.length) {
      // Fast path: Update existing core elements without destroying DOM
      for (let i = 0; i < coresUsage.length; i++) {
        const p = coresUsage[i];
        const item = existingItems[i];
        const pctSpan = item.querySelector('.core-item-header span:last-child');
        const fill = item.querySelector('.core-mini-fill');
        if (pctSpan) pctSpan.textContent = `${p.toFixed(0)}%`;
        if (fill) fill.style.width = `${Math.min(100, p)}%`;
      }
      return;
    }

    // Rebuild only when core count differs or initial render
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

  // Main Dashboard State Updater
  function updateDashboard(data) {
    if (!data) return;

    // 1. Host Info
    if (data.host) {
      const h = data.host;
      if (hostPill.textContent !== (h.hostname || 'Server')) {
        hostPill.textContent = h.hostname || 'Server';
      }
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
      updatePercentNode(cpuPercent, c.usage_percent);
      updateBarState(cpuBar, c.usage_percent);

      if (c.model_name && c.model_name !== 'Unknown CPU') {
        cpuModel.textContent = c.model_name;
      } else {
        cpuModel.textContent = `${c.cores_logical} Logical Cores`;
      }

      cpuCores.textContent = `${c.cores_physical || '-'} Phys / ${c.cores_logical || '-'} Log`;
      cpuFreq.textContent = c.mhz > 0 ? `${(c.mhz / 1000).toFixed(2)} GHz` : 'N/A';

      // Zero-allocation typed array push & sparkline draw
      pushHistory(history.cpu, c.usage_percent);
      drawSparkline(cpuCtx, cpuCanvas, canvasSizes.cpu, history.cpu, { max: 100, color: '#0284c7' });

      if (c.cores_usage) {
        renderCores(c.cores_usage);
      }
    }

    // 3. Memory
    if (data.memory) {
      const m = data.memory;
      updatePercentNode(memPercent, m.used_percent);
      updateBarState(memBar, m.used_percent);

      ramBreakdown.textContent = `${formatBytes(m.used)} / ${formatBytes(m.total)}`;
      memUsed.textContent = formatBytes(m.used);
      memAvail.textContent = formatBytes(m.available || m.free);

      if (m.swap_total > 0) {
        memSwap.textContent = `${formatBytes(m.swap_used)} (${m.swap_percent.toFixed(0)}%)`;
      } else {
        memSwap.textContent = 'None';
      }

      pushHistory(history.mem, m.used_percent);
      drawSparkline(memCtx, memCanvas, canvasSizes.mem, history.mem, { max: 100, color: '#6366f1', fillColor: 'rgba(99, 102, 241, 0.12)' });
    }

    // 4. Network
    if (data.network) {
      const n = data.network;
      const rx = formatRate(n.rx_rate_bps);
      const tx = formatRate(n.tx_rate_bps);

      updateRateNode(netRxRate, rx);
      updateRateNode(netTxRate, tx);

      netRxTotal.textContent = formatBytes(n.bytes_recv);
      netTxTotal.textContent = formatBytes(n.bytes_sent);

      netRxPackets.textContent = (n.packets_recv || 0).toLocaleString();
      netTxPackets.textContent = (n.packets_sent || 0).toLocaleString();

      pushHistory(history.netRx, n.rx_rate_bps || 0);
      pushHistory(history.netTx, n.tx_rate_bps || 0);
      drawNetSparkline(netCtx, netCanvas, canvasSizes.net, history.netRx, history.netTx);
    }

    // 5. Disks
    if (data.disks) {
      renderDisks(data.disks);
    }
  }

  // Fallback Polling / Manual Refresh
  async function fetchStatsManual() {
    if (isManualFetching) return;
    isManualFetching = true;
    btnManualRefresh.classList.add('spinning');
    const startTime = performance.now();

    try {
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
      statusIndicator.classList.remove('offline');
      statusText.textContent = 'Connected (HTTP)';

      updateDashboard(data);
    } catch (err) {
      console.error('Manual fetch failed:', err);
      statusIndicator.classList.add('offline');
      statusText.textContent = 'Offline';
    } finally {
      isManualFetching = false;
      setTimeout(() => {
        btnManualRefresh.classList.remove('spinning');
      }, 400);
    }
  }

  // Server-Sent Events (SSE) Stream Connection Management
  function startSSE() {
    if (eventSource) return;

    try {
      eventSource = new EventSource('/api/stream');

      eventSource.onopen = function () {
        statusIndicator.classList.remove('offline');
        statusText.textContent = 'Live (SSE)';
        latencyLabel.textContent = 'Stream: Live';
      };

      eventSource.onmessage = function (event) {
        if (!isPageVisible) return; // Skip CPU/DOM work while tab is hidden
        try {
          const data = JSON.parse(event.data);
          statusIndicator.classList.remove('offline');
          statusText.textContent = 'Live (SSE)';
          updateDashboard(data);
        } catch (e) {
          console.warn('Error parsing SSE JSON:', e);
        }
      };

      eventSource.onerror = function () {
        // EventSource will automatically retry in background
        statusIndicator.classList.add('offline');
        statusText.textContent = 'Reconnecting...';
      };
    } catch (e) {
      console.warn('SSE initialization failed, falling back to manual fetch:', e);
      fetchStatsManual();
    }
  }

  function stopSSE() {
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
  }

  // Page Visibility API: Stop SSE connection and chart rendering when tab is hidden, saving RAM & CPU
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) {
      isPageVisible = false;
      stopSSE();
      statusText.textContent = 'Paused (Background)';
    } else {
      isPageVisible = true;
      syncAllCanvasSizes();
      startSSE();
    }
  });

  // Manual Refresh Button
  btnManualRefresh.addEventListener('click', () => {
    fetchStatsManual();
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
    btnLogout.addEventListener('click', async (e) => {
      e.preventDefault();
      btnLogout.style.pointerEvents = 'none';
      btnLogout.style.opacity = '0.6';
      try {
        await fetch('/api/logout', { 
          method: 'POST', 
          credentials: 'same-origin',
          headers: { 'Cache-Control': 'no-cache' }
        });
      } catch (err) {
        console.warn('Logout request error:', err);
      }
      window.location.replace('/login');
    });
  }

  // Redraw all canvas charts with cached dimensions
  function redrawCharts() {
    syncAllCanvasSizes();
    drawSparkline(cpuCtx, cpuCanvas, canvasSizes.cpu, history.cpu, { max: 100, color: '#0284c7' });
    drawSparkline(memCtx, memCanvas, canvasSizes.mem, history.mem, { max: 100, color: '#6366f1', fillColor: 'rgba(99, 102, 241, 0.12)' });
    drawNetSparkline(netCtx, netCanvas, canvasSizes.net, history.netRx, history.netTx);
  }

  // Handle Window Resize and Orientation Change cleanly
  window.addEventListener('resize', redrawCharts);
  window.addEventListener('orientationchange', redrawCharts);

  if (typeof ResizeObserver !== 'undefined') {
    const ro = new ResizeObserver(() => {
      requestAnimationFrame(redrawCharts);
    });
    document.querySelectorAll('.chart-container').forEach(el => ro.observe(el));
  }

  // Initialize canvas resolutions and start Real-time SSE Stream
  syncAllCanvasSizes();
  startSSE();
})();
