// EdgeSensorWave Admin JavaScript

// Global app state
const EdgeAdmin = {
    version: '1.0.5',
    config: {
        refreshInterval: 10000, // 10 seconds
        notificationTimeout: 5000, // 5 seconds
        maxNotifications: 5
    },
    notifications: []
};

// Notification system
function showNotification(message, type = 'info', timeout = EdgeAdmin.config.notificationTimeout) {
    const container = document.getElementById('notifications');
    if (!container) return;
    
    // Remove old notifications if too many
    while (EdgeAdmin.notifications.length >= EdgeAdmin.config.maxNotifications) {
        const oldest = EdgeAdmin.notifications.shift();
        if (oldest && oldest.element && oldest.element.parentNode) {
            oldest.element.parentNode.removeChild(oldest.element);
        }
    }
    
    // Create notification element
    const notification = document.createElement('div');
    notification.className = `notification ${type}`;
    notification.innerHTML = `
        <div class="notification-content">
            <span class="notification-message">${message}</span>
            <button class="notification-close" onclick="closeNotification(this)">&times;</button>
        </div>
    `;
    
    // Add to container
    container.appendChild(notification);
    
    // Track notification
    const notificationData = {
        element: notification,
        timestamp: Date.now()
    };
    EdgeAdmin.notifications.push(notificationData);
    
    // Auto-remove after timeout
    if (timeout > 0) {
        setTimeout(() => {
            closeNotification(notification.querySelector('.notification-close'));
        }, timeout);
    }
    
    return notification;
}

function closeNotification(closeButton) {
    const notification = closeButton.closest('.notification');
    if (notification) {
        // Animate out
        notification.style.animation = 'slideOut 0.3s ease forwards';
        
        setTimeout(() => {
            if (notification.parentNode) {
                notification.parentNode.removeChild(notification);
            }
            
            // Remove from tracking
            EdgeAdmin.notifications = EdgeAdmin.notifications.filter(n => n.element !== notification);
        }, 300);
    }
}

// Add slideOut animation
const style = document.createElement('style');
style.textContent = `
    @keyframes slideOut {
        to {
            transform: translateX(100%);
            opacity: 0;
        }
    }
    
    .notification-content {
        display: flex;
        justify-content: space-between;
        align-items: center;
        gap: 1rem;
    }
    
    .notification-close {
        background: none;
        border: none;
        font-size: 1.2rem;
        cursor: pointer;
        color: inherit;
        opacity: 0.7;
        transition: opacity 0.2s;
    }
    
    .notification-close:hover {
        opacity: 1;
    }
`;
document.head.appendChild(style);

// Utility functions
function formatBytes(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function formatDuration(seconds) {
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const secs = seconds % 60;
    
    if (hours > 0) {
        return `${hours}h ${minutes}m`;
    } else if (minutes > 0) {
        return `${minutes}m ${secs}s`;
    } else {
        return `${secs}s`;
    }
}

function formatDate(date) {
    if (typeof date === 'string') {
        date = new Date(date);
    }
    return date.toLocaleDateString() + ' ' + date.toLocaleTimeString();
}

function debounce(func, wait) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

// API helper functions
async function apiRequest(url, options = {}) {
    try {
        const response = await fetch(url, {
            headers: {
                'Content-Type': 'application/json',
                ...options.headers
            },
            ...options
        });
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        
        // Check if response is JSON
        const contentType = response.headers.get('content-type');
        if (contentType && contentType.includes('application/json')) {
            return await response.json();
        }
        
        return response;
    } catch (error) {
        console.error('API Request failed:', error);
        throw error;
    }
}

// Auto-refresh functionality
let autoRefreshEnabled = true;
let refreshIntervals = new Map();

function startAutoRefresh(elementId, url, interval = EdgeAdmin.config.refreshInterval) {
    stopAutoRefresh(elementId); // Stop existing if any
    
    const refresh = async () => {
        if (!autoRefreshEnabled) return;
        
        const element = document.getElementById(elementId);
        if (!element) {
            stopAutoRefresh(elementId);
            return;
        }
        
        try {
            const response = await fetch(url);
            if (response.ok) {
                const html = await response.text();
                element.innerHTML = html;
            }
        } catch (error) {
            console.error(`Auto-refresh failed for ${elementId}:`, error);
        }
    };
    
    const intervalId = setInterval(refresh, interval);
    refreshIntervals.set(elementId, intervalId);
}

function stopAutoRefresh(elementId) {
    const intervalId = refreshIntervals.get(elementId);
    if (intervalId) {
        clearInterval(intervalId);
        refreshIntervals.delete(elementId);
    }
}

function toggleAutoRefresh() {
    autoRefreshEnabled = !autoRefreshEnabled;
    
    if (autoRefreshEnabled) {
        showNotification('Auto-refresh habilitado', 'success');
    } else {
        showNotification('Auto-refresh deshabilitado', 'info');
        // Stop all intervals
        refreshIntervals.forEach((intervalId) => clearInterval(intervalId));
        refreshIntervals.clear();
    }
}

// Chart utilities
function createLineChart(canvasId, config = {}) {
    const canvas = document.getElementById(canvasId);
    if (!canvas) {
        console.error(`Canvas with id '${canvasId}' not found`);
        return null;
    }
    
    const ctx = canvas.getContext('2d');
    
    const defaultConfig = {
        type: 'line',
        data: {
            labels: [],
            datasets: []
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                y: {
                    beginAtZero: true
                }
            },
            plugins: {
                legend: {
                    display: true,
                    position: 'top'
                }
            }
        }
    };
    
    // Merge configurations
    const mergedConfig = {
        ...defaultConfig,
        ...config,
        options: {
            ...defaultConfig.options,
            ...config.options
        }
    };
    
    return new Chart(ctx, mergedConfig);
}

function updateChartData(chart, newData, maxPoints = 20) {
    if (!chart || !chart.data) return;
    
    // Add new data
    if (Array.isArray(newData.labels)) {
        chart.data.labels = [...chart.data.labels, ...newData.labels];
    }
    
    if (Array.isArray(newData.datasets)) {
        newData.datasets.forEach((newDataset, index) => {
            if (chart.data.datasets[index]) {
                chart.data.datasets[index].data = [
                    ...chart.data.datasets[index].data,
                    ...newDataset.data
                ];
            }
        });
    }
    
    // Trim data to maxPoints
    if (chart.data.labels.length > maxPoints) {
        const excess = chart.data.labels.length - maxPoints;
        chart.data.labels.splice(0, excess);
        
        chart.data.datasets.forEach(dataset => {
            dataset.data.splice(0, excess);
        });
    }
    
    chart.update();
}

// Form validation
function validateForm(formElement) {
    const inputs = formElement.querySelectorAll('input[required], select[required], textarea[required]');
    let isValid = true;
    const errors = [];
    
    inputs.forEach(input => {
        if (!input.value.trim()) {
            isValid = false;
            errors.push(`${input.name || input.id} es requerido`);
            input.classList.add('error');
        } else {
            input.classList.remove('error');
        }
        
        // Email validation
        if (input.type === 'email' && input.value) {
            const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
            if (!emailRegex.test(input.value)) {
                isValid = false;
                errors.push('Email inválido');
                input.classList.add('error');
            }
        }
        
        // Number validation
        if (input.type === 'number' && input.value) {
            const min = parseFloat(input.min);
            const max = parseFloat(input.max);
            const value = parseFloat(input.value);
            
            if (!isNaN(min) && value < min) {
                isValid = false;
                errors.push(`${input.name || input.id} debe ser mayor a ${min}`);
                input.classList.add('error');
            }
            
            if (!isNaN(max) && value > max) {
                isValid = false;
                errors.push(`${input.name || input.id} debe ser menor a ${max}`);
                input.classList.add('error');
            }
        }
    });
    
    if (!isValid) {
        errors.forEach(error => showNotification(error, 'error'));
    }
    
    return isValid;
}

// Add error styles
const errorStyle = document.createElement('style');
errorStyle.textContent = `
    .error {
        border-color: var(--danger-color) !important;
        box-shadow: 0 0 0 3px rgba(244, 67, 54, 0.1) !important;
    }
`;
document.head.appendChild(errorStyle);

// Local storage utilities
const Storage = {
    get(key, defaultValue = null) {
        try {
            const item = localStorage.getItem(key);
            return item ? JSON.parse(item) : defaultValue;
        } catch (error) {
            console.error('LocalStorage get error:', error);
            return defaultValue;
        }
    },
    
    set(key, value) {
        try {
            localStorage.setItem(key, JSON.stringify(value));
            return true;
        } catch (error) {
            console.error('LocalStorage set error:', error);
            return false;
        }
    },
    
    remove(key) {
        try {
            localStorage.removeItem(key);
            return true;
        } catch (error) {
            console.error('LocalStorage remove error:', error);
            return false;
        }
    },
    
    clear() {
        try {
            localStorage.clear();
            return true;
        } catch (error) {
            console.error('LocalStorage clear error:', error);
            return false;
        }
    }
};

// Performance monitoring
const Performance = {
    marks: {},
    
    mark(name) {
        this.marks[name] = performance.now();
    },
    
    measure(name, startMark) {
        if (!this.marks[startMark]) {
            console.warn(`Start mark '${startMark}' not found`);
            return null;
        }
        
        const duration = performance.now() - this.marks[startMark];
        console.log(`${name}: ${duration.toFixed(2)}ms`);
        return duration;
    }
};

// Keyboard shortcuts
document.addEventListener('keydown', function(event) {
    // Ctrl+R or F5: Refresh current page
    if ((event.ctrlKey && event.key === 'r') || event.key === 'F5') {
        event.preventDefault();
        location.reload();
    }
    
    // Ctrl+/ : Show help (placeholder)
    if (event.ctrlKey && event.key === '/') {
        event.preventDefault();
        showNotification('Ayuda: Ctrl+R (actualizar), Ctrl+Q (consultas), Ctrl+A (alertas)', 'info', 10000);
    }
    
    // Quick navigation
    if (event.ctrlKey && !event.shiftKey && !event.altKey) {
        switch(event.key) {
            case '1':
                event.preventDefault();
                window.location.href = '/';
                break;
            case '2':
                event.preventDefault();
                window.location.href = '/consulta';
                break;
            case '3':
                event.preventDefault();
                window.location.href = '/alertas';
                break;
            case '4':
                event.preventDefault();
                window.location.href = '/exportar';
                break;
            case '5':
                event.preventDefault();
                window.location.href = '/mantenimiento';
                break;
        }
    }
});

// Page visibility handling
document.addEventListener('visibilitychange', function() {
    if (document.visibilityState === 'visible') {
        // Page became visible, resume auto-refresh
        autoRefreshEnabled = true;
        console.log('Page visible: auto-refresh resumed');
    } else {
        // Page hidden, pause auto-refresh to save resources
        autoRefreshEnabled = false;
        console.log('Page hidden: auto-refresh paused');
    }
});

// Error handling for unhandled promises
window.addEventListener('unhandledrejection', function(event) {
    console.error('Unhandled promise rejection:', event.reason);
    showNotification('Error inesperado en la aplicación', 'error');
    event.preventDefault();
});

// Initialize app when DOM is ready
document.addEventListener('DOMContentLoaded', function() {
    console.log(`EdgeSensorWave Admin v${EdgeAdmin.version} initialized`);
    
    // Add navigation active states
    const currentPath = window.location.pathname;
    const navItems = document.querySelectorAll('.nav-item');
    
    navItems.forEach(item => {
        const href = item.getAttribute('href');
        if (href === currentPath || (currentPath === '/' && href === '/')) {
            item.classList.add('active');
        }
    });
    
    // Initialize tooltips if any
    const tooltipElements = document.querySelectorAll('[title]');
    tooltipElements.forEach(element => {
        element.addEventListener('mouseenter', function() {
            // Simple tooltip implementation (could be enhanced)
            const title = this.getAttribute('title');
            if (title) {
                this.setAttribute('data-original-title', title);
                this.removeAttribute('title');
            }
        });
    });
    
    // Show welcome notification on first visit
    const isFirstVisit = !Storage.get('hasVisited');
    if (isFirstVisit) {
        Storage.set('hasVisited', true);
        setTimeout(() => {
            showNotification('¡Bienvenido a EdgeSensorWave Admin!', 'success', 8000);
        }, 1000);
    }
    
    // Performance mark
    Performance.mark('appInitialized');
});

// Export utilities for global use
window.EdgeAdmin = EdgeAdmin;
window.showNotification = showNotification;
window.apiRequest = apiRequest;
window.Storage = Storage;
window.Performance = Performance;
window.formatBytes = formatBytes;
window.formatDuration = formatDuration;
window.formatDate = formatDate;
window.validateForm = validateForm;
window.createLineChart = createLineChart;
window.updateChartData = updateChartData;
window.startAutoRefresh = startAutoRefresh;
window.stopAutoRefresh = stopAutoRefresh;
window.toggleAutoRefresh = toggleAutoRefresh;