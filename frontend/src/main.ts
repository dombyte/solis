import './style.css'
import { store } from './store'
import { api } from './api/client' // Kept for history data fetching
import { websocketClient } from './api/websocket'
import { cards, getRegisterConfig } from './config/registers'
import { Chart, registerables } from 'chart.js'

// Register all Chart.js components
Chart.register(...registerables)

// Extend Window interface for global functions
declare global {
  interface Window {
    setHistoryPeriod: (period: 'daily' | 'monthly' | 'yearly') => void
    setHistoryDates: (start: string, end: string) => void
    refreshHistory: () => Promise<void>
    setPage: (page: Page) => Promise<void>
    toggleRegister: (key: string) => void
    showTooltip: (key: string) => void
    hideTooltip: () => void
    toggleTooltip: (key: string) => void
  }
}

// History chart and state
let historyChart: Chart | null = null
let historyPeriod: 'daily' | 'monthly' | 'yearly' = 'daily'
let historyStartDate: string = ''
let historyEndDate: string = ''

// Registers for each period
const periodRegisters: Record<'daily' | 'monthly' | 'yearly', string[]> = {
  daily: [
    'household_load_today_energy',
    'today_energy_consumption',
    'today_energy_fed_into_grid',
    'today_energy_imported_from_grid',
    'today_battery_discharge_energy',
    'today_battery_charge_energy',
    'pv_today_energy',
    'backup_load_today_energy'
  ],
  monthly: [
    'pv_month_energy',
    'household_load_month_energy',
    'backup_load_month_energy'
  ],
  yearly: [
    'pv_year_energy',
    'household_load_year_energy',
    'backup_load_year_energy'
  ]
}

// Selected registers for each period (default to PV Energy only)
let selectedRegisters: Map<'daily' | 'monthly' | 'yearly', Set<string>> = new Map()
selectedRegisters.set('daily', new Set(['pv_today_energy']))
selectedRegisters.set('monthly', new Set(['pv_month_energy']))
selectedRegisters.set('yearly', new Set(['pv_year_energy']))

const app = document.querySelector<HTMLDivElement>('#app')!

// Page state
type Page = 'dashboard' | 'history' | 'reports'
let currentPage: Page = 'dashboard'

async function setPage(page: Page) {
  currentPage = page
  startAutoRefresh()
  render()
  
  // Load history data when switching to history page
  if (page === 'history') {
    await loadHistoryData()
  }
}

// Expose to global scope for onclick handlers
window.setPage = setPage

// WebSocket handles real-time updates, no polling needed
let refreshInterval: number | null = null

function startAutoRefresh() {
  // No polling needed with WebSocket - updates are pushed in real-time
  if (refreshInterval) clearInterval(refreshInterval)
  refreshInterval = null
}

// WebSocket status indicator update function
function updateWebSocketStatus() {
  const statusBar = document.querySelector('.status-bar')
  if (!statusBar) return
  
  const wsStatus = websocketClient.isConnected() ? 'WS' : 'DC'
  let wsStatusEl = statusBar.querySelector('.ws-status')
  if (!wsStatusEl) {
    wsStatusEl = document.createElement('span')
    wsStatusEl.className = 'ws-status'
    statusBar.appendChild(wsStatusEl)
  }
  
  wsStatusEl.textContent = ` [${wsStatus}]`
  wsStatusEl.className = `ws-status ${websocketClient.isConnected() ? 'ws-connected' : 'ws-disconnected'}`
}

// Update only the data values without re-rendering the entire page
function updateDataValues() {
  const state = store.getState()
  
  // Update status bar
  const statusBar = document.querySelector('.status-bar')
  if (statusBar) {
    const statusClass = state.isOnline ? 'status-online' : 'status-offline'
    const statusText = state.isOnline ? 'Online' : state.error || 'Offline'
    
    // Update WebSocket status indicator
    updateWebSocketStatus()
    
    let statusIndicator = statusBar.querySelector('.status-indicator')
    if (!statusIndicator) {
      statusIndicator = document.createElement('span')
      statusIndicator.className = 'status-indicator'
      statusBar.prepend(statusIndicator)
    }
    statusIndicator.className = `status-indicator ${statusClass}`
    
    let statusTextEl = statusBar.querySelector('.status-text')
    if (!statusTextEl) {
      statusTextEl = document.createElement('span')
      statusTextEl.className = 'status-text'
      statusBar.appendChild(statusTextEl)
    }
    statusTextEl.textContent = statusText
    
    // Update timestamp
    let timestampEl = statusBar.querySelector('.timestamp')
    if (state.lastUpdated) {
      if (!timestampEl) {
        timestampEl = document.createElement('span')
        timestampEl.className = 'timestamp'
        statusBar.appendChild(timestampEl)
      }
      timestampEl.textContent = `Updated: ${state.lastUpdated.toLocaleTimeString()}`
    }
  }
  
  // Update all register values in the dashboard
  const valueElements = document.querySelectorAll('.value-data[data-key]')
  valueElements.forEach(el => {
    const key = el.getAttribute('data-key')
    if (key) {
      const value = store.getRegisterValue(key)
      if (value) {
        const config = store.getRegisterConfig(key)
        const displayValue = formatValue(value.value, config?.unit || '')
        el.textContent = displayValue
      }
    }
  })
}

// History page functions
async function loadHistoryData() {
  if (currentPage !== 'history') return
  
  if (!historyStartDate || !historyEndDate) {
    // Set default dates if not set
    const end = new Date()
    const start = new Date()
    
    if (historyPeriod === 'daily') {
      start.setDate(end.getDate() - 7) // Last 7 days
      historyStartDate = start.toISOString().split('T')[0]
      historyEndDate = end.toISOString().split('T')[0]
    } else if (historyPeriod === 'monthly') {
      start.setMonth(end.getMonth() - 6) // Last 6 months
      historyStartDate = start.toISOString().split('T')[0].slice(0, 7) // YYYY-MM
      historyEndDate = end.toISOString().split('T')[0].slice(0, 7)
    } else {
      start.setFullYear(end.getFullYear() - 2) // Last 2 years
      historyStartDate = start.getFullYear().toString()
      historyEndDate = end.getFullYear().toString()
    }
  }
  
  try {
    // Get selected registers for current period
    const periodSelected = selectedRegisters.get(historyPeriod) || new Set()
    const selectedKeys = Array.from(periodSelected).filter(r => periodRegisters[historyPeriod].includes(r))
    
    if (selectedKeys.length === 0) {
      return
    }
    
    // Fetch data for all selected registers
    const datasets: any[] = []
    const allLabels: Set<string> = new Set()
    
    for (const key of selectedKeys) {
      let data: any[] = []
      
      switch (historyPeriod) {
        case 'daily':
          data = await api.getDaily(key, historyStartDate, historyEndDate)
          break
        case 'monthly':
          data = await api.getMonthly(key, historyStartDate, historyEndDate)
          break
        case 'yearly':
          data = await api.getYearly(key, historyStartDate, historyEndDate)
          break
      }
      
      if (data && data.length > 0) {
        const registerConfig = store.getRegisterConfig(key)
        const name = registerConfig?.name || key
        const unit = registerConfig?.unit || ''
        
        // Extract labels and values
        const labels = data.map((d: any) => {
          if (historyPeriod === 'daily') {
            return new Date(d.date).toLocaleDateString()
          } else if (historyPeriod === 'monthly') {
            return new Date(d.month).toLocaleString('default', { month: 'short', year: 'numeric' })
          } else {
            return d.year
          }
        })
        
        const values = data.map((d: any) => d.value)
        
        datasets.push({
          label: name,
          data: values,
          key: key,
          borderColor: getColorForKey(key),
          backgroundColor: getColorForKey(key, 0.1),
          borderWidth: 2,
          fill: false,
          tension: 0.4,
          unit: unit
        })
        
        // Collect all labels
        labels.forEach(l => allLabels.add(l))
      }
    }
    
    if (datasets.length > 0) {
      // Sort labels chronologically
      const sortedLabels = Array.from(allLabels).sort()
      createHistoryChart(sortedLabels, datasets)
    } else {
      // Destroy any existing chart if no data
      if (historyChart) {
        historyChart.destroy()
        historyChart = null
      }
    }
  } catch (error) {
    console.error('Failed to load history data:', error)
  }
}

// Helper to get consistent color for each register key
function getColorForKey(key: string, alpha: number = 1): string {
  const colors = [
    '#667eea', '#764ba2', '#f093fb', '#4facfe',
    '#43e97b', '#fa709a', '#fee140', '#30cfd0'
  ]
  let hash = 0
  for (let i = 0; i < key.length; i++) {
    hash = key.charCodeAt(i) + ((hash << 5) - hash)
  }
  const index = Math.abs(hash) % colors.length
  const color = colors[index]
  
  if (alpha < 1) {
    // Convert hex to rgba
    const r = parseInt(color.slice(1, 3), 16)
    const g = parseInt(color.slice(3, 5), 16)
    const b = parseInt(color.slice(5, 7), 16)
    return `rgba(${r}, ${g}, ${b}, ${alpha})`
  }
  return color
}

function createHistoryChart(labels: string[], datasets: any[]) {
  const ctx = document.getElementById('historyChart') as HTMLCanvasElement | null
  
  if (!ctx) {
    return
  }
  
  // Destroy existing chart if it exists
  if (historyChart) {
    historyChart.destroy()
  }
  
  // Get unit from first dataset (all should be same unit type for energy)
  const firstDataset = datasets[0]
  const unit = firstDataset?.unit || 'kWh'
  
  // Map datasets to Chart.js format
  const chartDatasets = datasets.map(ds => ({
    label: ds.label,
    data: ds.data,
    borderColor: ds.borderColor,
    backgroundColor: ds.backgroundColor,
    borderWidth: ds.borderWidth,
    fill: ds.fill,
    tension: ds.tension
  }))
  
  historyChart = new Chart(ctx, {
    type: 'bar',
    data: {
      labels: labels,
      datasets: chartDatasets
    },
    options: {
      responsive: true,
      maintainAspectRatio: true,
      interaction: {
        mode: 'point',
        intersect: true
      },
      plugins: {
        legend: {
          position: 'top',
          labels: {
            usePointStyle: true,
            padding: 20
          }
        },
        tooltip: {
          callbacks: {
            label: (context) => {
              return `${context.dataset.label}: ${context.parsed.y} ${unit}`
            }
          }
        }
      },
      scales: {
        y: {
          beginAtZero: true,
          title: {
            display: true,
            text: unit ? `Value (${unit})` : 'Value'
          }
        },
        x: {
          title: {
            display: true,
            text: historyPeriod === 'daily' ? 'Date' : historyPeriod === 'monthly' ? 'Month' : 'Year'
          }
        }
      }
    }
  })
}

function setHistoryPeriod(period: 'daily' | 'monthly' | 'yearly') {
  historyPeriod = period
  // Reset dates when period changes to ensure correct format
  historyStartDate = ''
  historyEndDate = ''
  render()
  loadHistoryData()
}

function setHistoryDates(start: string, end: string) {
  // Convert dates to appropriate format based on period
  if (historyPeriod === 'daily') {
    historyStartDate = start
    historyEndDate = end
  } else if (historyPeriod === 'monthly') {
    historyStartDate = start.slice(0, 7) // YYYY-MM
    historyEndDate = end.slice(0, 7)
  } else {
    historyStartDate = start.slice(0, 4) // YYYY
    historyEndDate = end.slice(0, 4)
  }
  loadHistoryData()
}

function toggleRegister(key: string) {
  const periodSelected = selectedRegisters.get(historyPeriod) || new Set()
  if (periodSelected.has(key)) {
    periodSelected.delete(key)
  } else {
    periodSelected.add(key)
  }
  selectedRegisters.set(historyPeriod, periodSelected)
  loadHistoryData()
}

function isRegisterSelected(key: string): boolean {
  const periodSelected = selectedRegisters.get(historyPeriod) || new Set()
  return periodSelected.has(key)
}

// Expose history functions to global scope
window.setHistoryPeriod = setHistoryPeriod
window.setHistoryDates = setHistoryDates
window.refreshHistory = loadHistoryData
window.toggleRegister = toggleRegister

// Expose tooltip functions to global scope
window.showTooltip = showTooltip
window.hideTooltip = hideTooltip
window.toggleTooltip = toggleTooltip

// Global state for tooltip visibility
let activeTooltipKey: string | null = null

function showTooltip(key: string) {
  // Hide any currently active tooltip
  const currentActive = document.querySelector('.tooltip.show')
  if (currentActive) {
    currentActive.classList.remove('show')
  }
  
  // Show the requested tooltip
  const tooltip = document.querySelector(`[data-tooltip-key="${key}"]`)
  if (tooltip) {
    tooltip.classList.add('show')
    activeTooltipKey = key
  }
}

function hideTooltip() {
  const currentActive = document.querySelector('.tooltip.show')
  if (currentActive) {
    currentActive.classList.remove('show')
  }
  activeTooltipKey = null
}

function toggleTooltip(key: string) {
  if (activeTooltipKey === key) {
    hideTooltip()
  } else {
    showTooltip(key)
  }
}

function formatValue(value: number | string, unit: string): string {
  if (value === null || value === undefined) return '-'
  if (typeof value === 'string') return value
  
  const numValue = value as number
  
  // Format percentage
  if (unit === '%') {
    return `${numValue.toFixed(1)}%`
  }
  
  // Format power values
  if (unit === 'W') {
    if (Math.abs(numValue) >= 1000) {
      return `${(numValue / 1000).toFixed(2)} kW`
    }
    return `${Math.round(numValue)} W`
  }
  
  // Format energy values
  if (unit === 'kWh' || unit === 'Wh') {
    if (Math.abs(numValue) >= 1000 && unit === 'Wh') {
      return `${(numValue / 1000).toFixed(2)} kWh`
    }
    return `${numValue.toFixed(2)} ${unit}`
  }
  
  // Format voltage
  if (unit === 'V') {
    return `${numValue.toFixed(1)} V`
  }
  
  // Format current
  if (unit === 'A') {
    return `${numValue.toFixed(2)} A`
  }
  
  // Default formatting
  return `${numValue} ${unit}`
}

function render() {
  const state = store.getState()
  
  const statusClass = state.isOnline ? 'status-online' : 'status-offline'
  const statusText = state.isOnline ? 'Online' : state.error || 'Offline'
  
  // Update WebSocket status
  updateWebSocketStatus()
  
  // Navigation menu
  const navHtml = `
    <nav class="main-nav">
      <button class="nav-item ${currentPage === 'dashboard' ? 'active' : ''}" onclick="setPage('dashboard')">Dashboard</button>
      <button class="nav-item ${currentPage === 'history' ? 'active' : ''}" onclick="setPage('history')">History</button>
      <button class="nav-item ${currentPage === 'reports' ? 'active' : ''}" onclick="setPage('reports')">Reports</button>
    </nav>
  `
  
  // Build card HTML from configuration
  const cardsHtml = cards.map(card => {
    const valueRows = card.registers.map(regConfig => {
      const value = store.getRegisterValue(regConfig.key)
      const displayValue = value ? formatValue(value.value, regConfig.unit || '') : '-'
      const fullConfig = getRegisterConfig(regConfig.key)
      const description = fullConfig?.description || ''
      
      return `
        <div class="value-row" data-key="${regConfig.key}">
          <div class="value-label-container">
            <span class="value-label">${regConfig.name}</span>
            ${description ? `
              <button class="info-icon" 
                      onclick="event.stopPropagation(); toggleTooltip('${regConfig.key}')"
                      onmouseenter="showTooltip('${regConfig.key}')"
                      onmouseleave="hideTooltip()"
                      aria-label="Info about ${regConfig.name}">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <circle cx="12" cy="12" r="10"/>
                  <path d="M12 16v-4M12 8h.01"/>
                </svg>
              </button>
              <div class="tooltip" data-tooltip-key="${regConfig.key}">${description}</div>
            ` : ''}
          </div>
          <span class="value-data" data-key="${regConfig.key}">${displayValue}</span>
        </div>
      `
    }).join('')
    
    return `
      <div class="card">
        <h2 class="card-title">${card.title}</h2>
        ${valueRows}
      </div>
    `
  }).join('')
  
  // Get today's date for date inputs (always use full date format for input fields)
  const today = new Date().toISOString().split('T')[0]
  const sevenDaysAgo = new Date()
  sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7)
  const sevenDaysAgoStr = sevenDaysAgo.toISOString().split('T')[0]
  
  // Ensure we have default dates set (for initial load)
  if (!historyStartDate || !historyEndDate) {
    if (historyPeriod === 'daily') {
      historyStartDate = sevenDaysAgoStr
      historyEndDate = today
    } else if (historyPeriod === 'monthly') {
      historyStartDate = sevenDaysAgoStr.slice(0, 7)
      historyEndDate = today.slice(0, 7)
    } else {
      historyStartDate = sevenDaysAgo.getFullYear().toString()
      historyEndDate = new Date().getFullYear().toString()
    }
  }
  
  // Generate register checkboxes for current period
  const periodRegs = periodRegisters[historyPeriod]
  const registerCheckboxes = periodRegs.map(key => {
    const config = store.getRegisterConfig(key)
    const displayName = config?.name || key
    const description = config?.description || ''
    const isChecked = isRegisterSelected(key)
    
    return `
      <label class="register-checkbox" for="reg-${key}">
        <input type="checkbox" id="reg-${key}" ${isChecked ? 'checked' : ''} 
               onchange="window.toggleRegister('${key}')">
        <div class="register-checkbox-content">
          <span>${displayName}</span>
          ${description ? `
            <button class="info-icon info-icon-small" 
                    onclick="event.stopPropagation(); toggleTooltip('history-${key}')"
                    onmouseenter="showTooltip('history-${key}')"
                    onmouseleave="hideTooltip()"
                    aria-label="Info about ${displayName}">
              <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/>
                <path d="M12 16v-4M12 8h.01"/>
              </svg>
            </button>
            <div class="tooltip" data-tooltip-key="history-${key}">${description}</div>
          ` : ''}
        </div>
      </label>
    `
  }).join('')
  
  // History view with chart
  const historyHtml = `
    <div class="history-view">
      <div class="history-controls">
        <div class="period-selector">
          <button class="period-btn ${historyPeriod === 'daily' ? 'active' : ''}" onclick="window.setHistoryPeriod('daily')">Daily</button>
          <button class="period-btn ${historyPeriod === 'monthly' ? 'active' : ''}" onclick="window.setHistoryPeriod('monthly')">Monthly</button>
          <button class="period-btn ${historyPeriod === 'yearly' ? 'active' : ''}" onclick="window.setHistoryPeriod('yearly')">Yearly</button>
        </div>
        
        <div class="date-range">
          <label>
            From:
            <input type="date" id="historyStart" value="${historyStartDate}" 
                   onchange="window.setHistoryDates(document.getElementById('historyStart').value, document.getElementById('historyEnd').value)">
          </label>
          <label>
            To:
            <input type="date" id="historyEnd" value="${historyEndDate}"
                   onchange="window.setHistoryDates(document.getElementById('historyStart').value, document.getElementById('historyEnd').value)">
          </label>
        </div>
        
        <button class="refresh-btn" onclick="window.refreshHistory()">
          <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M23 4v6h-6M1 20v-6h6"/>
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
          </svg>
          Refresh
        </button>
      </div>
      
      <div class="register-selector">
        <h3>Select Registers:</h3>
        <div class="register-checkboxes">
          ${registerCheckboxes}
        </div>
      </div>
      
      <div class="chart-container">
        <canvas id="historyChart"></canvas>
      </div>
    </div>
  `

  // Reports view placeholder
  const reportsHtml = `
    <div class="reports-view">
      <h2>Reports</h2>
      <p>Reports will be displayed here.</p>
    </div>
  `

  app.innerHTML = `
    <div class="container">
      <header>
        <h1>Solis Monitor</h1>
        <p>Monitor your Solis inverter in real-time</p>
        <div class="status-bar">
          <span class="status-indicator ${statusClass}"></span>
          <span class="status-text">${statusText}</span>
          ${state.lastUpdated ? `<span class="timestamp">Updated: ${state.lastUpdated.toLocaleTimeString()}</span>` : ''}
        </div>
      </header>
      
      ${navHtml}
      
      <main>
        ${currentPage === 'dashboard' ? `
          <div class="dashboard">
            ${cardsHtml || '<div class="empty-state">No cards configured. Update src/config/registers.ts</div>'}
          </div>
        ` : currentPage === 'history' ? historyHtml : reportsHtml}
        
        ${state.isLoading ? '<div class="loading">Loading data...</div>' : ''}
      </main>
      
      <footer>
        <p>Solis Inverter Monitor</p>
      </footer>
    </div>
  `
}

// Handle clicks outside tooltips to hide them on mobile
document.addEventListener('click', (event) => {
  const target = event.target as HTMLElement
  // Check if click is outside a tooltip or info icon
  if (!target.closest('.info-icon') && !target.closest('.tooltip') && !target.closest('.register-checkbox')) {
    hideTooltip()
  }
})

// Initial render
render()

// Start the store and re-render when data is loaded
store.initialize().then(() => {
  render()
  startAutoRefresh()
})

// Update timestamp every second to update timestamps (only on Dashboard)
setInterval(() => {
  if (currentPage === 'dashboard') {
    updateDataValues()
  }
}, 1000)
