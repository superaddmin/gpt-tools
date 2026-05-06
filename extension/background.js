/**
 * ChatAdd 凭证提取器 - Service Worker
 * 负责无痕窗口生命周期管理、登录状态检测、凭证提取与 JSON 下载
 */

'use strict';

const DEFAULT_CONFIG = {
  targetUrl: 'https://chatgpt.com/',
  loginCheckInterval: 2000,
  loginTimeout: 120000,
  sessionCookieName: '__Secure-next-auth.session-token',
  loginSuccessUrls: ['https://chatgpt.com/', 'https://chatgpt.com/?'],
  loginPagePatterns: ['auth.openai.com', 'chatgpt.com/auth/login']
};

let activeSession = null;

/**
 * 从 storage 中读取配置，若不存在则使用默认值
 * @returns {Promise<Object>} 合并后的配置对象
 */
async function loadConfig() {
  try {
    const stored = await chrome.storage.local.get('extractorConfig');
    return Object.assign({}, DEFAULT_CONFIG, stored.extractorConfig || {});
  } catch (_e) {
    return Object.assign({}, DEFAULT_CONFIG);
  }
}

/**
 * 向发起提取请求的 content script 发送状态更新
 * @param {string} status - 状态标识
 * @param {string} message - 状态描述
 */
function notifyStatus(status, message) {
  chrome.runtime.sendMessage({
    action: 'extractionStatus',
    status: status,
    message: message
  }).catch(function () {});
}

/**
 * 判断当前 URL 是否为登录成功后的目标页面
 * @param {string} url - 当前标签页 URL
 * @param {string[]} successUrls - 成功登录的 URL 列表
 * @returns {boolean}
 */
function isLoginSuccessUrl(url, successUrls) {
  if (!url) return false;
  const normalized = url.replace(/\/+$/, '');
  return successUrls.some(function (su) {
    return normalized === su.replace(/\/+$/, '') || normalized.startsWith(su.replace(/\/+$/, '') + '/');
  });
}

/**
 * 判断当前 URL 是否仍在登录流程页面
 * @param {string} url - 当前标签页 URL
 * @param {string[]} patterns - 登录页面 URL 特征
 * @returns {boolean}
 */
function isLoginPage(url, patterns) {
  if (!url) return false;
  return patterns.some(function (p) { return url.indexOf(p) !== -1; });
}

/**
 * 通过检查 session cookie 是否存在来判断登录状态
 * @param {string} cookieName - 会话 Cookie 名称
 * @param {string} domain - 目标域名
 * @returns {Promise<boolean>}
 */
async function checkSessionCookie(cookieName, domain) {
  try {
    const cookie = await chrome.cookies.get({ name: cookieName, url: domain });
    return !!(cookie && cookie.value);
  } catch (_e) {
    return false;
  }
}

/**
 * 获取无痕模式的 cookie store ID
 * @returns {Promise<string|null>}
 */
async function getIncognitoStoreId() {
  try {
    const stores = await chrome.cookies.getAllCookieStores();
    for (const store of stores) {
      if (store.incognito) {
        return store.id;
      }
    }
    return null;
  } catch (_e) {
    return null;
  }
}

/**
 * 从无痕窗口中提取所有 Cookies
 * @param {string} domain - 目标域名
 * @param {string} storeId - 无痕 cookie store ID
 * @returns {Promise<Array>} Cookie 对象数组
 */
async function extractCookies(domain, storeId) {
  try {
    const cookies = await chrome.cookies.getAll({ domain: domain, storeId: storeId });
    return cookies.map(function (c) {
      return {
        name: c.name,
        value: c.value,
        domain: c.domain,
        path: c.path,
        secure: c.secure,
        httpOnly: c.httpOnly,
        sameSite: c.sameSite,
        expirationDate: c.expirationDate
      };
    });
  } catch (_e) {
    return [];
  }
}

/**
 * 在无痕标签页中执行脚本，提取 LocalStorage 和 SessionStorage
 * @param {number} tabId - 无痕标签页 ID
 * @returns {Promise<{localStorage: Object, sessionStorage: Object}>}
 */
async function extractStorageFromTab(tabId) {
  try {
    const results = await chrome.scripting.executeScript({
      target: { tabId: tabId },
      func: function () {
        var ls = {};
        var ss = {};
        try {
          for (var i = 0; i < localStorage.length; i++) {
            var key = localStorage.key(i);
            ls[key] = localStorage.getItem(key);
          }
        } catch (_e) {}
        try {
          for (var j = 0; j < sessionStorage.length; j++) {
            var skey = sessionStorage.key(j);
            ss[skey] = sessionStorage.getItem(skey);
          }
        } catch (_e) {}
        return { localStorage: ls, sessionStorage: ss };
      }
    });
    if (results && results.length > 0 && results[0].result) {
      return results[0].result;
    }
    return { localStorage: {}, sessionStorage: {} };
  } catch (_e) {
    return { localStorage: {}, sessionStorage: {} };
  }
}

/**
 * 在无痕标签页中执行脚本，检测页面 DOM 中的登录状态
 * @param {number} tabId - 无痕标签页 ID
 * @returns {Promise<boolean>}
 */
async function checkDomLoginStatus(tabId) {
  try {
    const results = await chrome.scripting.executeScript({
      target: { tabId: tabId },
      func: function () {
        var hasSession = !!(document.cookie.indexOf('__Secure-next-auth.session-token') !== -1);
        var hasChatUI = !!(document.querySelector('nav') && document.querySelector('textarea'));
        var notOnLogin = !window.location.href.includes('auth.openai.com') &&
                         !window.location.href.includes('/auth/login');
        return hasSession || (hasChatUI && notOnLogin);
      }
    });
    if (results && results.length > 0 && results[0].result) {
      return results[0].result;
    }
    return false;
  } catch (_e) {
    return false;
  }
}

/**
 * 将凭证数据打包为 JSON 并触发浏览器下载
 * @param {Object} data - 包含 cookies, localStorage, sessionStorage 的数据对象
 */
function downloadCredentialsJson(data) {
  var jsonStr = JSON.stringify(data, null, 2);
  var blob = new Blob([jsonStr], { type: 'application/json' });
  var reader = new FileReader();
  reader.onloadend = function () {
    var dataUrl = reader.result;
    var timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
    chrome.downloads.download({
      url: dataUrl,
      filename: 'chatadd_credentials_' + timestamp + '.json',
      saveAs: false
    }).catch(function (_e) {});
  };
  reader.readAsDataURL(blob);
}

/**
 * 清理活跃会话状态
 */
function cleanupSession() {
  if (activeSession && activeSession.timerId) {
    clearTimeout(activeSession.timerId);
  }
  activeSession = null;
}

/**
 * 执行完整的凭证提取流程
 * @param {number} tabId - 无痕标签页 ID
 * @param {Object} config - 配置对象
 * @param {string} storeId - 无痕 cookie store ID
 */
async function performExtraction(tabId, config, storeId) {
  notifyStatus('extracting', '正在提取 Cookies / Storage...');

  try {
    var tab = await chrome.tabs.get(tabId);
    var url = new URL(tab.url);
    var domain = url.hostname;

    var cookies = await extractCookies(domain, storeId);
    var storage = await extractStorageFromTab(tabId);

    var credentials = {
      extractedAt: new Date().toISOString(),
      sourceUrl: tab.url,
      domain: domain,
      cookies: cookies,
      localStorage: storage.localStorage,
      sessionStorage: storage.sessionStorage,
      summary: {
        cookieCount: cookies.length,
        localStorageKeys: Object.keys(storage.localStorage).length,
        sessionStorageKeys: Object.keys(storage.sessionStorage).length
      }
    };

    downloadCredentialsJson(credentials);
    notifyStatus('complete', '凭证提取完成！JSON 文件已自动下载。共提取 ' +
      cookies.length + ' 个 Cookie、' +
      Object.keys(storage.localStorage).length + ' 个 LocalStorage 键、' +
      Object.keys(storage.sessionStorage).length + ' 个 SessionStorage 键。');

    cleanupSession();
  } catch (err) {
    notifyStatus('error', '提取过程出错: ' + (err.message || '未知错误'));
    cleanupSession();
  }
}

/**
 * 登录状态轮询检测循环
 * @param {number} tabId - 无痕标签页 ID
 * @param {Object} config - 配置对象
 * @param {string} storeId - 无痕 cookie store ID
 * @param {number} startTime - 开始时间戳
 */
async function pollLoginStatus(tabId, config, storeId, startTime) {
  if (!activeSession || activeSession.cancelled) {
    cleanupSession();
    return;
  }

  var elapsed = Date.now() - startTime;
  if (elapsed > config.loginTimeout) {
    notifyStatus('timeout', '登录超时（' + Math.round(config.loginTimeout / 1000) + '秒），请重试');
    cleanupSession();
    return;
  }

  try {
    var tab = await chrome.tabs.get(tabId);
    if (!tab) {
      notifyStatus('error', '无痕窗口已关闭');
      cleanupSession();
      return;
    }

    var url = tab.url || '';

    var cookieOk = await checkSessionCookie(config.sessionCookieName, config.targetUrl);
    var urlOk = isLoginSuccessUrl(url, config.loginSuccessUrls);
    var domOk = false;

    if (urlOk && !isLoginPage(url, config.loginPagePatterns)) {
      domOk = await checkDomLoginStatus(tabId);
    }

    if (cookieOk || domOk) {
      notifyStatus('loggedIn', '检测到登录态，开始提取凭证...');
      await performExtraction(tabId, config, storeId);
      return;
    }
  } catch (_e) {}

  activeSession.timerId = setTimeout(function () {
    pollLoginStatus(tabId, config, storeId, startTime);
  }, config.loginCheckInterval);
}

/**
 * 监听无痕标签页的 URL 变化，辅助快速检测登录完成
 * @param {number} tabId
 * @param {Object} changeInfo
 * @param {Object} tab
 */
function handleTabUpdate(tabId, changeInfo, tab) {
  if (!activeSession || activeSession.tabId !== tabId) return;
  if (!changeInfo.url && changeInfo.status !== 'complete') return;

  var url = changeInfo.url || tab.url || '';
  if (isLoginSuccessUrl(url, activeSession.config.loginSuccessUrls)) {
    notifyStatus('monitoring', '页面已跳转至目标地址，正在确认登录状态...');
  }
}

/**
 * 监听无痕窗口关闭事件
 * @param {number} windowId
 */
function handleWindowRemoved(windowId) {
  if (activeSession && activeSession.windowId === windowId) {
    notifyStatus('cancelled', '无痕窗口已关闭');
    cleanupSession();
  }
}

/**
 * 启动提取流程：创建无痕窗口并开始监控
 * @param {Object} config - 配置对象
 */
async function startExtraction(config) {
  if (activeSession) {
    notifyStatus('error', '已有正在进行的提取任务，请等待完成');
    return;
  }

  try {
    var win = await chrome.windows.create({
      url: config.targetUrl,
      incognito: true,
      focused: true,
      state: 'normal'
    });

    if (!win || !win.tabs || win.tabs.length === 0) {
      notifyStatus('error', '无法创建无痕窗口，请确认浏览器已允许扩展在无痕模式下运行');
      return;
    }

    var tabId = win.tabs[0].id;
    var storeId = await getIncognitoStoreId();

    activeSession = {
      windowId: win.id,
      tabId: tabId,
      config: config,
      storeId: storeId,
      startTime: Date.now(),
      timerId: null,
      cancelled: false
    };

    notifyStatus('monitoring', '无痕窗口已打开，请在新窗口中登录账号...');

    setTimeout(function () {
      pollLoginStatus(tabId, config, storeId, activeSession.startTime);
    }, config.loginCheckInterval);

  } catch (err) {
    notifyStatus('error', '启动失败: ' + (err.message || '未知错误'));
    cleanupSession();
  }
}

/**
 * 取消当前提取任务
 */
function cancelExtraction() {
  if (activeSession) {
    activeSession.cancelled = true;
    cleanupSession();
    notifyStatus('cancelled', '提取任务已取消');
  }
}

// --- 事件监听注册 ---

chrome.runtime.onMessage.addListener(function (message, sender, sendResponse) {
  if (message.action === 'startExtraction') {
    loadConfig().then(function (config) {
      startExtraction(config);
    });
    sendResponse({ success: true });
    return true;
  }

  if (message.action === 'cancelExtraction') {
    cancelExtraction();
    sendResponse({ success: true });
    return true;
  }

  if (message.action === 'getStatus') {
    sendResponse({
      active: !!activeSession,
      session: activeSession ? {
        startTime: activeSession.startTime,
        elapsed: Date.now() - activeSession.startTime
      } : null
    });
    return true;
  }
});

chrome.tabs.onUpdated.addListener(handleTabUpdate);
chrome.windows.onRemoved.addListener(handleWindowRemoved);
