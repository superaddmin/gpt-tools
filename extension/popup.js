/**
 * ChatAdd 凭证提取器 - Popup 配置面板
 * 提供目标 URL、检测规则的可视化配置，支持手动触发提取
 */

'use strict';

var DEFAULT_CONFIG = {
  targetUrl: 'https://chatgpt.com/',
  loginCheckInterval: 2000,
  loginTimeout: 120,
  sessionCookieName: '__Secure-next-auth.session-token',
  loginSuccessUrls: ['https://chatgpt.com/'],
  loginPagePatterns: ['auth.openai.com', 'chatgpt.com/auth/login']
};

var elements = {};

/**
 * 显示 Toast 提示
 * @param {string} message - 提示文本
 * @param {'success'|'error'} type - 提示类型
 */
function showToast(message, type) {
  var toast = elements.toast;
  toast.textContent = message;
  toast.className = 'toast ' + type + ' show';
  clearTimeout(toast.__timer);
  toast.__timer = setTimeout(function () {
    toast.classList.remove('show');
  }, 2500);
}

/**
 * 从 storage 加载配置并填充表单
 */
async function loadConfigToForm() {
  try {
    var stored = await chrome.storage.local.get('extractorConfig');
    var config = Object.assign({}, DEFAULT_CONFIG, stored.extractorConfig || {});
    elements.targetUrl.value = config.targetUrl || '';
    elements.sessionCookieName.value = config.sessionCookieName || '';
    elements.loginSuccessUrls.value = (config.loginSuccessUrls || []).join('\n');
    elements.loginTimeout.value = config.loginTimeout || 120;
  } catch (_e) {
    resetForm();
  }
}

/**
 * 从表单收集当前配置
 * @returns {Object} 配置对象
 */
function collectConfigFromForm() {
  return {
    targetUrl: elements.targetUrl.value.trim() || DEFAULT_CONFIG.targetUrl,
    loginCheckInterval: DEFAULT_CONFIG.loginCheckInterval,
    loginTimeout: parseInt(elements.loginTimeout.value, 10) || DEFAULT_CONFIG.loginTimeout,
    sessionCookieName: elements.sessionCookieName.value.trim() || DEFAULT_CONFIG.sessionCookieName,
    loginSuccessUrls: elements.loginSuccessUrls.value
      .split('\n')
      .map(function (s) { return s.trim(); })
      .filter(function (s) { return s.length > 0; }),
    loginPagePatterns: DEFAULT_CONFIG.loginPagePatterns
  };
}

/**
 * 保存配置到 storage
 */
async function saveConfig() {
  var config = collectConfigFromForm();
  try {
    await chrome.storage.local.set({ extractorConfig: config });
    showToast('配置已保存', 'success');
  } catch (err) {
    showToast('保存失败: ' + err.message, 'error');
  }
}

/**
 * 恢复默认配置
 */
function resetForm() {
  elements.targetUrl.value = DEFAULT_CONFIG.targetUrl;
  elements.sessionCookieName.value = DEFAULT_CONFIG.sessionCookieName;
  elements.loginSuccessUrls.value = DEFAULT_CONFIG.loginSuccessUrls.join('\n');
  elements.loginTimeout.value = DEFAULT_CONFIG.loginTimeout;
  showToast('已恢复默认配置', 'success');
}

/**
 * 查询当前提取任务状态
 */
async function refreshStatus() {
  try {
    var response = await chrome.runtime.sendMessage({ action: 'getStatus' });
    updateStatusUI(response);
  } catch (_e) {
    updateStatusUI({ active: false });
  }
}

/**
 * 根据任务状态更新 UI
 * @param {Object} status - 状态对象
 */
function updateStatusUI(status) {
  var dot = elements.statusDot;
  var text = elements.statusText;
  var extractBtn = elements.extractBtn;
  var cancelBtn = elements.cancelBtn;

  if (status && status.active) {
    dot.className = 'status-dot active';
    var elapsed = status.session ? Math.round(status.session.elapsed / 1000) : 0;
    text.textContent = '提取中... 已运行 ' + elapsed + ' 秒';
    extractBtn.disabled = true;
    cancelBtn.disabled = false;
  } else {
    dot.className = 'status-dot';
    text.textContent = '就绪';
    extractBtn.disabled = false;
    cancelBtn.disabled = true;
  }
}

/**
 * 触发凭证提取
 */
function triggerExtraction() {
  saveConfig().then(function () {
    chrome.runtime.sendMessage({ action: 'startExtraction' }, function (response) {
      if (chrome.runtime.lastError) {
        showToast('通信失败: ' + chrome.runtime.lastError.message, 'error');
        return;
      }
      if (response && response.success) {
        showToast('无痕窗口已启动', 'success');
        refreshStatus();
      }
    });
  });
}

/**
 * 取消当前提取任务
 */
function cancelTask() {
  chrome.runtime.sendMessage({ action: 'cancelExtraction' }, function () {
    refreshStatus();
  });
}

/**
 * 监听来自 Service Worker 的状态更新
 */
function listenForStatusUpdates() {
  chrome.runtime.onMessage.addListener(function (message) {
    if (message.action === 'extractionStatus') {
      if (message.status === 'complete' || message.status === 'error' ||
          message.status === 'timeout' || message.status === 'cancelled') {
        refreshStatus();
      }
    }
  });
}

/**
 * 初始化事件绑定
 */
function bindEvents() {
  elements.saveBtn.addEventListener('click', saveConfig);
  elements.resetBtn.addEventListener('click', resetForm);
  elements.extractBtn.addEventListener('click', triggerExtraction);
  elements.cancelBtn.addEventListener('click', cancelTask);
}

/**
 * 入口
 */
document.addEventListener('DOMContentLoaded', function () {
  elements.targetUrl = document.getElementById('targetUrl');
  elements.sessionCookieName = document.getElementById('sessionCookieName');
  elements.loginSuccessUrls = document.getElementById('loginSuccessUrls');
  elements.loginTimeout = document.getElementById('loginTimeout');
  elements.saveBtn = document.getElementById('saveBtn');
  elements.resetBtn = document.getElementById('resetBtn');
  elements.extractBtn = document.getElementById('extractBtn');
  elements.cancelBtn = document.getElementById('cancelBtn');
  elements.statusDot = document.getElementById('statusDot');
  elements.statusText = document.getElementById('statusText');
  elements.toast = document.getElementById('toast');

  bindEvents();
  loadConfigToForm();
  refreshStatus();
  listenForStatusUpdates();

  setInterval(refreshStatus, 3000);
});
