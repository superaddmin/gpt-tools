/**
 * ChatAdd 凭证提取器 - Content Script
 * 在 chatgpt.com 页面注入浮动操作按钮，负责与 Service Worker 通信
 */
(function () {
  'use strict';

  if (window.__chataddInjected) return;
  window.__chataddInjected = true;

  const CONTAINER_ID = '__chatadd_container';
  const BUTTON_ID = '__chatadd_extract_btn';
  const STATUS_ID = '__chatadd_status';

  /**
   * 创建注入 UI 的样式
   */
  function injectStyles() {
    const css = document.createElement('style');
    css.textContent = `
      #${CONTAINER_ID} {
        position: fixed;
        bottom: 24px;
        right: 24px;
        z-index: 2147483647;
        display: flex;
        flex-direction: column;
        align-items: flex-end;
        gap: 10px;
        font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      }
      #${BUTTON_ID} {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 12px 20px;
        background: linear-gradient(135deg, #10a37f 0%, #1a7f64 100%);
        color: #fff;
        border: none;
        border-radius: 28px;
        font-size: 14px;
        font-weight: 600;
        cursor: pointer;
        box-shadow: 0 4px 16px rgba(16, 163, 127, 0.4);
        transition: all 0.25s ease;
        letter-spacing: 0.3px;
        user-select: none;
      }
      #${BUTTON_ID}:hover {
        transform: translateY(-2px);
        box-shadow: 0 6px 24px rgba(16, 163, 127, 0.55);
        background: linear-gradient(135deg, #1a7f64 0%, #10a37f 100%);
      }
      #${BUTTON_ID}:active {
        transform: translateY(0);
        box-shadow: 0 2px 8px rgba(16, 163, 127, 0.3);
      }
      #${BUTTON_ID}:disabled {
        background: #888;
        cursor: not-allowed;
        box-shadow: 0 2px 8px rgba(0,0,0,0.15);
        transform: none;
      }
      #${BUTTON_ID} .btn-icon {
        width: 18px;
        height: 18px;
        flex-shrink: 0;
      }
      #${BUTTON_ID} .btn-spinner {
        display: none;
        width: 16px;
        height: 16px;
        border: 2px solid rgba(255,255,255,0.3);
        border-top-color: #fff;
        border-radius: 50%;
        animation: __chatadd_spin 0.7s linear infinite;
      }
      #${BUTTON_ID}.loading .btn-icon { display: none; }
      #${BUTTON_ID}.loading .btn-spinner { display: inline-block; }
      @keyframes __chatadd_spin {
        to { transform: rotate(360deg); }
      }
      #${STATUS_ID} {
        max-width: 320px;
        padding: 10px 16px;
        background: #1f1f1f;
        color: #e0e0e0;
        border-radius: 12px;
        font-size: 12px;
        line-height: 1.5;
        box-shadow: 0 4px 16px rgba(0,0,0,0.25);
        display: none;
        word-break: break-word;
      }
      #${STATUS_ID}.visible { display: block; }
      #${STATUS_ID}.success { background: #0d3b2c; color: #8ff0a4; }
      #${STATUS_ID}.error { background: #3b0d0d; color: #f08f8f; }
      #${STATUS_ID}.info { background: #0d2c3b; color: #8fcff0; }
    `;
    document.head.appendChild(css);
  }

  /**
   * 创建浮动按钮和状态提示的 DOM 结构
   */
  function createUI() {
    const container = document.createElement('div');
    container.id = CONTAINER_ID;

    const statusEl = document.createElement('div');
    statusEl.id = STATUS_ID;

    const button = document.createElement('button');
    button.id = BUTTON_ID;
    button.title = '提取当前会话凭证';
    button.innerHTML = `
      <svg class="btn-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
        <polyline points="7 10 12 15 17 10"/>
        <line x1="12" y1="15" x2="12" y2="3"/>
      </svg>
      <span class="btn-spinner"></span>
      <span class="btn-label">提取凭证</span>
    `;

    container.appendChild(statusEl);
    container.appendChild(button);
    document.body.appendChild(container);

    return { button, statusEl, labelEl: button.querySelector('.btn-label') };
  }

  /**
   * 更新状态提示的显示内容和样式
   * @param {string} message - 提示文本
   * @param {'success'|'error'|'info'} type - 提示类型
   * @param {number} [duration] - 自动隐藏时间(ms)，0 表示不自动隐藏
   */
  function showStatus(statusEl, message, type, duration) {
    statusEl.textContent = message;
    statusEl.className = type + ' visible';
    if (duration && duration > 0) {
      clearTimeout(statusEl.__timeout);
      statusEl.__timeout = setTimeout(function () {
        statusEl.classList.remove('visible');
      }, duration);
    }
  }

  /**
   * 设置按钮加载状态
   * @param {HTMLButtonElement} button
   * @param {boolean} loading
   */
  function setButtonLoading(button, loading) {
    if (loading) {
      button.classList.add('loading');
      button.disabled = true;
    } else {
      button.classList.remove('loading');
      button.disabled = false;
    }
  }

  /**
   * 处理提取凭证按钮点击事件
   */
  function handleExtract(button, statusEl) {
    setButtonLoading(button, true);
    showStatus(statusEl, '正在启动无痕窗口...', 'info', 0);

    chrome.runtime.sendMessage({ action: 'startExtraction' }, function (response) {
      if (chrome.runtime.lastError) {
        showStatus(statusEl, '通信失败: ' + chrome.runtime.lastError.message, 'error', 5000);
        setButtonLoading(button, false);
        return;
      }
      if (response && response.success) {
        showStatus(statusEl, '无痕窗口已启动，请在新窗口中完成登录...', 'info', 0);
      } else {
        showStatus(statusEl, '启动失败: ' + ((response && response.error) || '未知错误'), 'error', 5000);
        setButtonLoading(button, false);
      }
    });
  }

  /**
   * 监听来自 Service Worker 的状态更新消息
   */
  function listenForUpdates(button, statusEl) {
    chrome.runtime.onMessage.addListener(function (message, sender, sendResponse) {
      if (message.action === 'extractionStatus') {
        switch (message.status) {
          case 'monitoring':
            showStatus(statusEl, message.message || '正在监控登录状态...', 'info', 0);
            break;
          case 'loggedIn':
            showStatus(statusEl, message.message || '检测到登录态，正在提取凭证...', 'info', 0);
            break;
          case 'extracting':
            showStatus(statusEl, message.message || '正在提取 Cookies / Storage...', 'info', 0);
            break;
          case 'complete':
            showStatus(statusEl, message.message || '凭证提取完成，JSON 文件已下载！', 'success', 8000);
            setButtonLoading(button, false);
            break;
          case 'error':
            showStatus(statusEl, '错误: ' + (message.message || '未知错误'), 'error', 8000);
            setButtonLoading(button, false);
            break;
          case 'timeout':
            showStatus(statusEl, '登录超时，请重试', 'error', 5000);
            setButtonLoading(button, false);
            break;
          case 'cancelled':
            showStatus(statusEl, '操作已取消', 'info', 3000);
            setButtonLoading(button, false);
            break;
          default:
            break;
        }
      }
    });
  }

  /**
   * 初始化入口
   */
  function init() {
    injectStyles();
    var ui = createUI();
    ui.button.addEventListener('click', function () {
      handleExtract(ui.button, ui.statusEl);
    });
    listenForUpdates(ui.button, ui.statusEl);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
