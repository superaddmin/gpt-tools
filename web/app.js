const form = document.querySelector("#checkoutForm");
const submitButton = document.querySelector("#submitButton");
const output = document.querySelector("#output");
const healthBadge = document.querySelector("#healthBadge");
const latency = document.querySelector("#latency");
const statusText = document.querySelector("#statusText");
const sessionText = document.querySelector("#sessionText");
const hostText = document.querySelector("#hostText");
const finalTransactionText = document.querySelector("#finalTransactionText");
const finalSettlementText = document.querySelector("#finalSettlementText");
const checkoutUrlLink = document.querySelector("#checkoutUrlLink");
const copyCheckoutLinkButton = document.querySelector("#copyCheckoutLinkButton");
const openInIncognitoBtn = document.querySelector("#openInIncognitoBtn");
const openIncognitoBtn = document.querySelector("#openIncognitoBtn");
const fetchSessionBtn = document.querySelector("#fetchSessionBtn");
const alertBox = document.querySelector("#alertBox");
const alertTitle = document.querySelector("#alertTitle");
const alertBadge = document.querySelector("#alertBadge");
const alertMessage = document.querySelector("#alertMessage");
const alertHint = document.querySelector("#alertHint");
const alertSteps = document.querySelector("#alertSteps");
const alertStepsList = document.querySelector("#alertStepsList");
const stepFlowItems = Array.from(document.querySelectorAll(".step-flow-item"));
const stepInputText = document.querySelector("#stepInputText");
const stepTokenText = document.querySelector("#stepTokenText");
const stepCheckoutText = document.querySelector("#stepCheckoutText");
const stepCookieText = document.querySelector("#stepCookieText");
const stepLinkText = document.querySelector("#stepLinkText");

const wechatGroupCard = document.querySelector("#wechatGroupCard");
const wechatGroupToggleBtn = document.querySelector("#wechatGroupToggleBtn");
const wechatGroupPreviewBtn = document.querySelector("#wechatGroupPreviewBtn");
const wechatGroupCloseBtn = document.querySelector("#wechatGroupCloseBtn");
const wechatGroupModal = document.querySelector("#wechatGroupModal");
const wechatGroupBackdrop = document.querySelector("#wechatGroupBackdrop");
const wechatGroupModalCloseBtn = document.querySelector("#wechatGroupModalCloseBtn");

let latestCheckoutURL = "";
let latestOpenedCheckoutURL = "";
let incognitoWindowOpened = false;
const clientPerfState = {
  longTasks: [],
  slowInteractions: [],
  errors: [],
};

function safePerfURL(rawURL) {
  try {
    const parsed = new URL(String(rawURL || ""), window.location.href);
    const params = [];
    parsed.searchParams.forEach(function (_value, key) {
      if (!params.includes(key)) params.push(key);
    });
    const query = params.length ? "?" + params.map(function (key) { return encodeURIComponent(key) + "=***"; }).join("&") : "";
    var hash = parsed.hash || "";
    if (hash.includes("?")) {
      const parts = hash.split("?");
      const route = parts.shift();
      const hashParams = new URLSearchParams(parts.join("?"));
      const hashNames = [];
      hashParams.forEach(function (_value, key) {
        if (!hashNames.includes(key)) hashNames.push(key);
      });
      hash = route + (hashNames.length ? "?" + hashNames.map(function (key) { return encodeURIComponent(key) + "=***"; }).join("&") : "");
    }
    return parsed.origin + parsed.pathname + query + hash;
  } catch (_error) {
    return String(rawURL || "").split("?")[0].slice(0, 180);
  }
}

function initClientPerformanceObservers() {
  window.addEventListener("error", function (event) {
    clientPerfState.errors.push({
      type: "error",
      message: String(event.message || "script error").slice(0, 240),
      source: safePerfURL(event.filename || ""),
      line: event.lineno || 0,
      column: event.colno || 0,
    });
  });
  window.addEventListener("unhandledrejection", function (event) {
    clientPerfState.errors.push({
      type: "unhandledrejection",
      message: String(event.reason?.message || event.reason || "promise rejection").slice(0, 240),
    });
  });
  if (!window.PerformanceObserver) return;
  try {
    if (PerformanceObserver.supportedEntryTypes?.includes("longtask")) {
      new PerformanceObserver(function (list) {
        list.getEntries().slice(-20).forEach(function (entry) {
          clientPerfState.longTasks.push({
            name: entry.name,
            start_ms: Math.round(entry.startTime),
            duration_ms: Math.round(entry.duration),
          });
        });
        clientPerfState.longTasks = clientPerfState.longTasks.slice(-20);
      }).observe({ entryTypes: ["longtask"] });
    }
  } catch (_error) {}
  try {
    if (PerformanceObserver.supportedEntryTypes?.includes("event")) {
      new PerformanceObserver(function (list) {
        list.getEntries().slice(-20).forEach(function (entry) {
          if (entry.duration >= 40) {
            clientPerfState.slowInteractions.push({
              name: entry.name,
              start_ms: Math.round(entry.startTime),
              duration_ms: Math.round(entry.duration),
            });
          }
        });
        clientPerfState.slowInteractions = clientPerfState.slowInteractions.slice(-20);
      }).observe({ type: "event", buffered: true, durationThreshold: 40 });
    }
  } catch (_error) {}
}

function buildClientPerformancePayload() {
  const nav = performance.getEntriesByType("navigation")[0];
  const resources = performance.getEntriesByType("resource").map(function (entry) {
    return {
      url: safePerfURL(entry.name),
      initiator_type: entry.initiatorType,
      start_ms: Math.round(entry.startTime),
      duration_ms: Math.round(entry.duration),
      transfer_size: entry.transferSize || 0,
      encoded_size: entry.encodedBodySize || 0,
      decoded_size: entry.decodedBodySize || 0,
      response_end_ms: Math.round(entry.responseEnd || 0),
    };
  }).slice(-80);
  return {
    page_url: safePerfURL(window.location.href),
    recorded_at: new Date().toISOString(),
    nav: nav ? {
      duration_ms: Math.round(nav.duration),
      dom_content_loaded_ms: Math.round(nav.domContentLoadedEventEnd),
      load_event_ms: Math.round(nav.loadEventEnd),
      response_start_ms: Math.round(nav.responseStart),
      response_end_ms: Math.round(nav.responseEnd),
      transfer_size: nav.transferSize || 0,
      encoded_size: nav.encodedBodySize || 0,
      decoded_size: nav.decodedBodySize || 0,
    } : null,
    resources: resources,
    resource_totals: {
      count: resources.length,
      transfer_size: resources.reduce(function (sum, item) { return sum + item.transfer_size; }, 0),
      decoded_size: resources.reduce(function (sum, item) { return sum + item.decoded_size; }, 0),
    },
    long_tasks: clientPerfState.longTasks.slice(-20),
    slow_interactions: clientPerfState.slowInteractions.slice(-20),
    errors: clientPerfState.errors.slice(-20),
  };
}

function sendClientPerformanceLog() {
  try {
    const body = JSON.stringify(buildClientPerformancePayload());
    if (navigator.sendBeacon) {
      const blob = new Blob([body], { type: "application/json" });
      if (navigator.sendBeacon("/api/perf/client", blob)) return;
    }
    fetch("/api/perf/client", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: body,
      keepalive: true,
    }).catch(function () {});
  } catch (_error) {}
}

initClientPerformanceObservers();
window.addEventListener("load", function () {
  window.setTimeout(sendClientPerformanceLog, 2500);
});

const fields = {
  token: document.querySelector("#token"),
  entryPoint: document.querySelector("#entryPoint"),
  planName: document.querySelector("#planName"),
  country: document.querySelector("#country"),
  currency: document.querySelector("#currency"),
  promoCampaignId: document.querySelector("#promoCampaignId"),
  couponFromQuery: document.querySelector("#couponFromQuery"),
  checkoutUIMode: document.querySelector("#checkoutUIMode"),
  checkoutCookie: document.querySelector("#checkoutCookie"),
  checkoutUserAgent: document.querySelector("#checkoutUserAgent"),
  customerEmail: document.querySelector("#customerEmail"),
};

function setText(node, value) {
  if (node) {
    node.textContent = value;
  }
}

function setWechatGroupCollapsed(collapsed) {
  if (!wechatGroupCard || !wechatGroupToggleBtn) {
    return;
  }
  wechatGroupCard.classList.toggle("is-collapsed", collapsed);
  wechatGroupCard.classList.toggle("is-expanded", !collapsed);
  wechatGroupToggleBtn.setAttribute("aria-expanded", collapsed ? "false" : "true");
}

function openWechatGroupModal() {
  if (!wechatGroupModal) {
    return;
  }
  wechatGroupModal.hidden = false;
  document.body.style.overflow = "hidden";
}

function closeWechatGroupModal() {
  if (!wechatGroupModal) {
    return;
  }
  wechatGroupModal.hidden = true;
  document.body.style.overflow = "";
}

function bindWechatGroupCard() {
  if (wechatGroupToggleBtn) {
    wechatGroupToggleBtn.addEventListener("click", function () {
      var collapsed = wechatGroupCard?.classList.contains("is-collapsed");
      setWechatGroupCollapsed(!collapsed);
    });
  }

  if (wechatGroupPreviewBtn) {
    wechatGroupPreviewBtn.addEventListener("click", function () {
      openWechatGroupModal();
    });
  }

  if (wechatGroupCloseBtn) {
    wechatGroupCloseBtn.addEventListener("click", function () {
      if (wechatGroupCard) {
        wechatGroupCard.classList.add("is-hidden");
      }
      closeWechatGroupModal();
    });
  }

  if (wechatGroupBackdrop) {
    wechatGroupBackdrop.addEventListener("click", function () {
      closeWechatGroupModal();
    });
  }

  if (wechatGroupModalCloseBtn) {
    wechatGroupModalCloseBtn.addEventListener("click", function () {
      closeWechatGroupModal();
    });
  }

  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape") {
      closeWechatGroupModal();
    }
  });
}

function extractAccessToken(rawValue) {
  const value = (rawValue || "").trim();
  if (!value) {
    return "";
  }
  if (value.startsWith("eyJ")) {
    return value;
  }
  try {
    const parsed = JSON.parse(value);
    const token = parsed?.accessToken || parsed?.access_token;
    if (typeof token === "string" && token.trim()) {
      return token.trim();
    }
  } catch (error) {
    const markers = ['"accessToken":"', '"accessToken": "', '"access_token":"', '"access_token": "'];
    for (const marker of markers) {
      const start = value.indexOf(marker);
      if (start >= 0) {
        const valueStart = start + marker.length;
        const end = value.indexOf('"', valueStart);
        if (end > valueStart) {
          return value.slice(valueStart, end).trim();
        }
      }
    }
  }
  return value;
}

function randomGmail() {
  const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789";
  const suffix = Array.from({ length: 10 }, () => alphabet[Math.floor(Math.random() * alphabet.length)]).join("");
  return `gpt.billing.${suffix}@gmail.com`;
}

async function checkHealth() {
  try {
    const response = await fetch("/api/health");
    if (!response.ok) {
      throw new Error("health check failed");
    }
    setText(healthBadge, "在线");
    healthBadge?.classList.remove("error");
  } catch (error) {
    setText(healthBadge, "离线");
    healthBadge?.classList.add("error");
  }
}

function buildStartPayload() {
  fields.token.value = extractAccessToken(fields.token.value);
  fields.customerEmail.value = randomGmail();
  return {
    token: fields.token.value.trim(),
    entry_point: fields.entryPoint.value.trim(),
    plan_name: fields.planName.value.trim(),
    billing_details: {
      country: fields.country.value.trim(),
      currency: fields.currency.value.trim(),
    },
    promo_campaign: {
      promo_campaign_id: fields.promoCampaignId.value.trim(),
      is_coupon_from_query_param: fields.couponFromQuery.checked,
    },
    checkout_ui_mode: fields.checkoutUIMode.value,
    checkout_session: {
      cookie: fields.checkoutCookie.value.trim(),
      user_agent: fields.checkoutUserAgent.value.trim(),
    },
    customer_email: fields.customerEmail.value.trim(),
  };
}

function buildRequestHeaders(extraHeaders) {
  var headers = { "Content-Type": "application/json" };
  var email = (fields.customerEmail?.value || "").trim();
  if (email) {
    headers["X-Account-Email"] = email;
  }
  if (extraHeaders && typeof extraHeaders === "object") {
    Object.assign(headers, extraHeaders);
  }
  return headers;
}

async function postJSON(url, payload) {
  const response = await fetch(url, {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify(payload),
  });
  const data = await response.json();
  return { response, data };
}

async function copyText(text) {
  if (!text) {
    return false;
  }
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch (error) {
    const fallback = document.createElement("textarea");
    fallback.value = text;
    fallback.setAttribute("readonly", "");
    fallback.style.position = "absolute";
    fallback.style.left = "-9999px";
    document.body.append(fallback);
    fallback.select();
    const copied = document.execCommand("copy");
    fallback.remove();
    return copied;
  }
}

async function copyCurrentCheckoutURL() {
  if (!latestCheckoutURL || !copyCheckoutLinkButton) {
    return;
  }
  copyCheckoutLinkButton.disabled = true;
  const copied = await copyText(latestCheckoutURL);
  setText(copyCheckoutLinkButton, copied ? "已复制" : "复制失败");
  window.setTimeout(() => {
    setText(copyCheckoutLinkButton, "复制链接");
    copyCheckoutLinkButton.disabled = false;
  }, 1400);
}

/**
 * 调用后端 API，在系统 Chrome 无痕窗口中打开指定 URL
 * @param {string} url - 要打开的 URL
 * @param {boolean} newWindow - 是否创建新的无痕窗口（仅首次为 true）
 * @returns {Promise<boolean>} 是否成功
 */
async function openIncognito(url, newWindow) {
  try {
    const response = await fetch("/api/incognito/open", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({ url: url, new_window: newWindow }),
    });
    const data = await response.json();
    if (response.ok && data.ok) {
      incognitoWindowOpened = true;
      return true;
    }
    console.error("incognito open failed:", data);
    return false;
  } catch (error) {
    console.error("incognito open error:", error);
    return false;
  }
}

const autoFillCheckoutBtn = document.querySelector("#autoFillCheckoutBtn");

/**
 * 确保无痕窗口已打开，若未打开则先创建 ChatGPT 首页窗口
 * 所有后续操作必须通过此函数统一管理，保证共用同一个无痕窗口
 * @returns {Promise<boolean>} 是否成功
 */
async function ensureIncognitoWindow() {
  if (incognitoWindowOpened) return true;
  const ok = await openIncognito("https://chatgpt.com/", true);
  if (!ok) return false;
  await new Promise(function (r) { setTimeout(r, 1500); });
  return true;
}

async function openChatGPTInSameIncognito() {
  return await openIncognito("https://chatgpt.com/", false);
}

async function waitForSessionLoginCompletion(maxWaitMs) {
  var startedAt = Date.now();
  while (Date.now() - startedAt < maxWaitMs) {
    await new Promise(function (r) { setTimeout(r, 2500); });
    try {
      var result = await fetchLatestSessionJSON({ skipEnsureWindow: true, allowRecovery: false, silentConclusion: true });
      if (result) {
        return result;
      }
    } catch (_err) {
    }
  }
  throw new Error("等待登录超时，请在无痕窗口中完成 ChatGPT 登录后重试。");
}

async function handleSessionAutoAction(conclusion) {
  if (!conclusion) return null;
  var action = conclusion.auto_action || "";

  if (action === "continue_checkout" || action === "manual_review") {
    return null;
  }

  if (action === "open_chatgpt_home") {
    appendMonitorEvent({ domain: "Log", method: "session-auto-action", summary: "自动恢复：打开 chatgpt.com", ts: Date.now() });
    var opened = await openChatGPTInSameIncognito();
    if (!opened) {
      throw new Error("自动打开 chatgpt.com 失败，请手动检查无痕窗口。");
    }
    await new Promise(function (r) { setTimeout(r, 1800); });
    return await fetchLatestSessionJSON({ skipEnsureWindow: true, allowRecovery: false, silentConclusion: true });
  }

  if (action === "wait_and_retry" || action === "retry_session_fetch") {
    appendMonitorEvent({ domain: "Log", method: "session-auto-action", summary: "自动恢复：等待后重试 Session 获取", ts: Date.now() });
    await new Promise(function (r) { setTimeout(r, 2000); });
    return await fetchLatestSessionJSON({ skipEnsureWindow: true, allowRecovery: false, silentConclusion: true });
  }

  if (action === "wait_for_login") {
    appendMonitorEvent({ domain: "Log", method: "session-auto-action", summary: "自动恢复：等待你在无痕窗口完成登录", ts: Date.now() });
    if (monitorBadge) { monitorBadge.textContent = "等待登录中"; monitorBadge.className = "badge"; }
    return await waitForSessionLoginCompletion(120000);
  }

  return null;
}

/**
 * 自动从当前无痕 Chrome 会话提取最新 Session JSON，并回填到左侧输入框
 * @returns {Promise<string>} 最新 Session JSON 文本
 */
async function fetchLatestSessionJSON(options) {
  options = options || {};
  const skipEnsureWindow = !!options.skipEnsureWindow;
  const allowRecovery = options.allowRecovery !== false;
  const silentConclusion = !!options.silentConclusion;

  if (!skipEnsureWindow) {
    const ensured = await ensureIncognitoWindow();
    if (!ensured) {
      throw new Error("无法启动 Chrome 无痕窗口，请确认系统已安装 Chrome 浏览器。");
    }
  }

  sessionMonitorAbortController = new AbortController();
  clearMonitor();
  if (gopayMonitor) gopayMonitor.hidden = false;
  if (monitorBadge) { monitorBadge.textContent = "Session 监控中"; monitorBadge.className = "badge"; }

  const response = await fetch("/api/session/fetch", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify({ stream: true }),
    signal: sessionMonitorAbortController.signal,
  });

  if (!response.ok) {
    var errorText = "";
    try {
      errorText = await response.text();
    } catch (_err) {
    }
    throw new Error(errorText || "获取 Session JSON 失败，请确认已在无痕窗口中登录 ChatGPT");
  }

  if (!response.body) {
    throw new Error("浏览器未返回可读取的流式响应，请刷新页面后重试。");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let fullOutput = [];
  let resultJSON = "";
  let diagnostics = null;
  let finalError = null;

  while (true) {
    const chunk = await reader.read();
    if (chunk.done) break;
    buffer += decoder.decode(chunk.value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() || "";

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i].trim();
      if (!line) continue;
      const msg = JSON.parse(line);
      fullOutput.push(msg);

      if (monitorOutput) {
        monitorOutput.hidden = false;
        monitorOutput.textContent = JSON.stringify(fullOutput, null, 2);
      }

      if (msg.type === "started") {
        appendMonitorEvent({ domain: "Log", method: "session-monitor-started", summary: (msg.targets || []).join(" | "), ts: Date.now() });
      } else if (msg.type === "event" && msg.event) {
        appendMonitorEvent(msg.event);
      } else if (msg.type === "data" || msg.type === "done") {
        resultJSON = msg.data?.json || resultJSON;
        diagnostics = msg.data?.diagnostics || diagnostics;
      } else if (msg.type === "error") {
        diagnostics = msg.data || diagnostics;
        finalError = msg.error || "获取 Session JSON 失败";
        appendMonitorEvent({ domain: "Error", method: "session-fetch-error", summary: finalError, ts: Date.now() });
      }
    }
  }

  sessionMonitorAbortController = null;

  if (diagnostics && monitorOutput) {
    monitorOutput.hidden = false;
    monitorOutput.textContent = JSON.stringify({ stream: fullOutput, diagnostics: diagnostics }, null, 2);
  }

  if (diagnostics?.conclusion && !silentConclusion) {
    renderSessionConclusion(diagnostics.conclusion);
  }

  if (resultJSON) {
    fields.token.value = resultJSON;
    if (sessionText) setText(sessionText, "已提取 Session JSON");
    if (monitorBadge) { monitorBadge.textContent = "Session 已获取"; monitorBadge.className = "badge"; }
    return resultJSON;
  }

  if (diagnostics?.conclusion && allowRecovery) {
    var recovered = await handleSessionAutoAction(diagnostics.conclusion);
    if (recovered) {
      return recovered;
    }
  }

  if (monitorBadge) { monitorBadge.textContent = "Session 失败"; monitorBadge.className = "badge error"; }
  throw new Error(finalError || diagnostics?.session_preview || "获取 Session JSON 失败，请确认已在无痕窗口中登录 ChatGPT");
}

function renderSessionConclusion(conclusion) {
  if (!conclusion) return;
  if (conclusion.message) {
    appendMonitorEvent({ domain: "Log", method: "session-conclusion", summary: conclusion.message, ts: Date.now() });
  }
  if (alertBox) {
    alertBox.hidden = false;
    alertBox.classList.remove("cookie", "token");
    if (alertTitle) setText(alertTitle, "Session 诊断结论");
    if (alertBadge) setText(alertBadge, conclusion.status || "诊断");
    if (alertMessage) setText(alertMessage, conclusion.message || "-");
    if (alertHint) {
      alertHint.hidden = false;
      setText(alertHint, conclusion.status || "-");
    }
    renderAlertSteps(Array.isArray(conclusion.next_steps) ? conclusion.next_steps : []);
  }
}

function showAutoFillNotice(title, badge, message, hint, kind) {
  if (!alertBox) {
    return;
  }
  alertBox.hidden = false;
  alertBox.classList.remove("cookie", "token");
  if (alertTitle) setText(alertTitle, title || "自动填地址");
  if (alertBadge) setText(alertBadge, badge || "提示");
  if (alertMessage) setText(alertMessage, message || "-");
  if (alertHint) {
    alertHint.hidden = !hint;
    setText(alertHint, hint || "-");
  }
  if (alertSteps) alertSteps.hidden = true;
  if (alertStepsList) alertStepsList.innerHTML = "";
  if (kind === "error") {
    alertBox.classList.add("token");
  }
}

/**
 * 在已有的无痕窗口中打开指定 URL（不创建新窗口）
 * @param {string} url - 要打开的 URL
 */
async function openInSameIncognito(url) {
  const ensured = await ensureIncognitoWindow();
  if (!ensured) {
    showAutoFillNotice("打开无痕窗口失败", "错误", "无法启动 Chrome 无痕窗口，请确认系统已安装 Chrome 浏览器。", "", "error");
    return false;
  }
  const opened = await openIncognito(url, false);
  if (!opened) {
    return false;
  }
  latestOpenedCheckoutURL = url;
  try {
    await new Promise(function (r) { setTimeout(r, 1800); });
    const resp = await fetch("/api/checkout/resolve-target", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({ opened_url: url }),
    });
    const data = await resp.json();
    if (resp.ok && data.ok && data.current_url) {
      latestOpenedCheckoutURL = data.current_url;
    } else {
      console.error("resolve checkout target failed:", data);
    }
  } catch (error) {
    console.error("resolve checkout target error:", error);
  }
  return true;
}

/**
 * 在无痕窗口中打开支付链接
 */
async function openPaymentInIncognito() {
  if (!latestCheckoutURL) return;
  await openInSameIncognito(latestCheckoutURL);
}

function resetResult() {
  latestCheckoutURL = "";
  setText(statusText, "生成中");
  setText(sessionText, "-");
  setText(hostText, "-");
  setText(finalTransactionText, "请求上游");
  setText(finalSettlementText, "-");
  setText(output, "{}");
  setStepState("input", "active", "正在识别输入内容");
  setStepState("token", "", "等待提取 token");
  setStepState("checkout", "", "等待请求 checkout");
  setStepState("cookie", "", "等待判断是否需要浏览器会话");
  setStepState("link", "", "等待生成最终链接");
  if (alertBox) {
    alertBox.hidden = true;
    alertBox.classList.remove("cookie", "token");
  }
  if (alertHint) {
    alertHint.hidden = true;
    setText(alertHint, "-");
  }
  if (alertSteps) {
    alertSteps.hidden = true;
  }
  if (alertStepsList) {
    alertStepsList.innerHTML = "";
  }
  if (checkoutUrlLink) {
    checkoutUrlLink.hidden = true;
    checkoutUrlLink.href = "#";
    setText(checkoutUrlLink, "打开链接");
  }
  if (copyCheckoutLinkButton) {
    copyCheckoutLinkButton.hidden = true;
    copyCheckoutLinkButton.disabled = true;
    setText(copyCheckoutLinkButton, "复制链接");
  }
  if (openInIncognitoBtn) {
    openInIncognitoBtn.hidden = true;
    openInIncognitoBtn.disabled = true;
  }
}

function renderAlertSteps(steps) {
  if (!alertSteps || !alertStepsList) {
    return;
  }
  alertStepsList.innerHTML = "";
  if (!Array.isArray(steps) || steps.length === 0) {
    alertSteps.hidden = true;
    return;
  }
  for (const step of steps) {
    const item = document.createElement("li");
    item.textContent = step;
    alertStepsList.append(item);
  }
  alertSteps.hidden = false;
}

function setStepState(step, state, text) {
  const item = stepFlowItems.find((node) => node.dataset.step === step);
  if (!item) {
    return;
  }
  item.classList.remove("active", "done", "error", "warning");
  if (state) {
    item.classList.add(state);
  }
  const textMap = {
    input: stepInputText,
    token: stepTokenText,
    checkout: stepCheckoutText,
    cookie: stepCookieText,
    link: stepLinkText,
  };
  const target = textMap[step];
  setText(target, text || "-");
}

function renderStepFlow(data, checkoutURL) {
  const inputValue = fields.token?.value?.trim() || "";
  const looksLikeSessionJSON = inputValue.startsWith("{") && inputValue.includes("accessToken");

  setStepState("input", "done", looksLikeSessionJSON ? "已识别为 Session JSON" : (inputValue ? "已识别为手动输入" : "等待提交"));
  setStepState("token", data?.token_invalidated ? "error" : "done", data?.token_invalidated ? "提取成功，但 token 已失效" : "已提取可用 token 格式");

  if (data?.stage === "checkout_upstream" && data?.status === 401) {
    setStepState("checkout", "error", "上游拒绝了当前 token");
  } else if (data?.stage === "checkout_upstream" && data?.status === 403) {
    setStepState("checkout", "warning", "checkout 已返回 403，需要浏览器态");
  } else if (data?.stage === "checkout_trial_validation") {
    setStepState("checkout", "done", "checkout 会话已拿到，正在校验试用资格");
  } else if (data?.stage === "checkout_link") {
    setStepState("checkout", "done", "checkout 与 Stripe 初始化已完成");
  } else if (checkoutURL) {
    setStepState("checkout", "done", "checkout 初始化成功");
  } else {
    setStepState("checkout", "active", "等待上游返回 checkout 结果");
  }

  if (data?.requires_cookie) {
    setStepState("cookie", "warning", "当前账号需要补 chatgpt.com Cookie");
  } else if (data?.stage === "checkout_upstream" && data?.status === 401) {
    setStepState("cookie", "", "此轮失败不是 Cookie 问题");
  } else if (checkoutURL || data?.stage === "checkout_link" || data?.stage === "checkout_trial_validation") {
    setStepState("cookie", "done", "当前请求未被 Cookie 阻断");
  } else {
    setStepState("cookie", "", "如遇 403，再补 Cookie");
  }

  if (checkoutURL) {
    setStepState("link", "done", "已生成有效支付链接");
  } else if (data?.stage === "checkout_trial_validation") {
    setStepState("link", "warning", "卡在 0 元试用资格校验");
  } else if (data?.stage === "checkout_link") {
    setStepState("link", "error", "Stripe 已返回，但未提取到可用链接");
  } else if (data?.stage === "checkout_upstream") {
    setStepState("link", "", "上游未进入可生成链接阶段");
  } else {
    setStepState("link", "active", "等待生成最终支付链接");
  }
}

function errorPresentation(data) {
  if (data?.token_invalidated) {
    return {
      type: "token",
      title: "登录凭证已失效",
      badge: "Token",
      message: "当前 access token 已失效，系统无法继续生成支付链接。",
      hint: data.hint || "请重新登录 ChatGPT，并重新粘贴新的 access token 或完整 /api/auth/session JSON。",
      steps: [
        "在浏览器里重新登录 ChatGPT，确保当前账号处于已登录状态。",
        "打开 /api/auth/session 或获取新的 access token。",
        "把新的 access token 或整份 session JSON 粘贴回左侧输入框后重试。",
      ],
    };
  }
  if (data?.requires_cookie) {
    return {
      type: "cookie",
      title: "需要浏览器会话",
      badge: "Cookie",
      message: "当前账号在初始化 checkout 时需要 chatgpt.com 的浏览器会话 Cookie。",
      hint: data.hint || "请在“可选浏览器态”里补充当前 chatgpt.com 会话 Cookie，必要时再补 User-Agent。",
      steps: [
        "在当前已登录 ChatGPT 的浏览器里打开 chatgpt.com。",
        "按 F12 打开开发者工具，进入 Network 或 Application / Storage 查看 Cookie。",
        "复制 chatgpt.com 当前会话相关 Cookie，粘贴到左侧“可选浏览器态”里的 Cookie 输入框。",
        "如果仍失败，再把当前浏览器的 User-Agent 一并填入后重试。",
      ],
    };
  }
  return {
    type: "generic",
    title: "生成支付链接失败",
    badge: "失败",
    message: data?.error || "请求未成功完成。",
    hint: data?.hint || "请检查输入参数、网络环境或上游返回内容。",
    steps: [
      "先确认 access token 是否最新且未失效。",
      "如果返回 403，再补 chatgpt.com 的 Cookie。",
      "如仍失败，请查看下方原始返回内容继续排查。",
    ],
  };
}

function renderResult(data, elapsedMs, ok) {
  const checkoutURL = data.checkout_url || data.url || "";
  latestCheckoutURL = checkoutURL;

  let host = "-";
  if (checkoutURL) {
    try {
      host = new URL(checkoutURL).hostname || "-";
    } catch (error) {
      host = "-";
    }
  }

  setText(latency, `${elapsedMs}ms`);
  latency?.classList.toggle("error", !ok);
  latency?.classList.toggle("neutral", ok);
  setText(statusText, checkoutURL ? "checkout_link_ready" : (data.stage || data.error || "failed"));
  setText(sessionText, data.checkout_session_id || data.session_id || "-");
  setText(hostText, host);
  setText(finalTransactionText, checkoutURL ? "生成成功" : "生成失败");
  setText(finalSettlementText, checkoutURL ? "只返回官方长链接" : (data.hint || data.error || "-"));
  setText(output, checkoutURL || JSON.stringify(data, null, 2));
  renderStepFlow(data, checkoutURL);

  if (alertBox) {
    if (checkoutURL) {
      alertBox.hidden = true;
      alertBox.classList.remove("cookie", "token");
    } else {
      const presentation = errorPresentation(data);
      alertBox.hidden = false;
      alertBox.classList.remove("cookie", "token");
      if (presentation.type === "cookie") {
        alertBox.classList.add("cookie");
      }
      if (presentation.type === "token") {
        alertBox.classList.add("token");
      }
      setText(alertTitle, presentation.title);
      setText(alertBadge, presentation.badge);
      setText(alertMessage, presentation.message);
      setText(alertHint, presentation.hint || "-");
      renderAlertSteps(presentation.steps);
      if (alertHint) {
        alertHint.hidden = !presentation.hint;
      }
    }
  }

  if (checkoutUrlLink) {
    checkoutUrlLink.hidden = !checkoutURL;
    checkoutUrlLink.href = checkoutURL || "#";
    setText(checkoutUrlLink, checkoutURL || "打开链接");
  }
  if (copyCheckoutLinkButton) {
    copyCheckoutLinkButton.hidden = !checkoutURL;
    copyCheckoutLinkButton.disabled = !checkoutURL;
    setText(copyCheckoutLinkButton, "复制链接");
  }
  if (openInIncognitoBtn) {
    openInIncognitoBtn.hidden = !checkoutURL;
    openInIncognitoBtn.disabled = !checkoutURL;
  }
}

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  resetResult();
  submitButton.disabled = true;
  setText(submitButton, "生成中");
  setText(latency, "运行中");

  const startedAt = performance.now();
  try {
    const { response, data } = await postJSON("/api/checkout/start", buildStartPayload());
    renderResult(data, Math.round(performance.now() - startedAt), response.ok);
    if (response.ok && latestCheckoutURL) {
      void copyCurrentCheckoutURL();
      void openPaymentInIncognito();
    }
  } catch (error) {
    renderResult({ error: error.message }, Math.round(performance.now() - startedAt), false);
  } finally {
    submitButton.disabled = false;
    setText(submitButton, "生成支付链接");
  }
});

/**
 * 全自动管道：提取 Session JSON → 粘贴到输入框 → 自动生成支付链接 → 自动打开
 */
async function autoFetchAndGenerate() {
  if (!fetchSessionBtn) return;
  fetchSessionBtn.disabled = true;
  const origText = "获取 Session JSON";
  setText(fetchSessionBtn, "启动窗口...");

  try {
    setText(fetchSessionBtn, "监控并读取 Session...");
    await fetchLatestSessionJSON();
    setText(fetchSessionBtn, "生成链接...");
    form.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    return;
  } catch (error) {
    if (monitorBadge) { monitorBadge.textContent = "Session 失败"; monitorBadge.className = "badge error"; }
    showAutoFillNotice("获取 Session 失败", "错误", error.message || "获取 Session JSON 失败，请确认已在无痕窗口中登录 ChatGPT", "", "error");
  } finally {
    sessionMonitorAbortController = null;
    fetchSessionBtn.disabled = false;
    setText(fetchSessionBtn, origText);
  }
}

copyCheckoutLinkButton?.addEventListener("click", () => {
  void copyCurrentCheckoutURL();
});

openInIncognitoBtn?.addEventListener("click", () => {
  void openPaymentInIncognito();
});

openIncognitoBtn?.addEventListener("click", () => {
  void ensureIncognitoWindow();
});

fetchSessionBtn?.addEventListener("click", () => {
  void autoFetchAndGenerate();
});

autoFillCheckoutBtn?.addEventListener("click", async () => {
  if (!autoFillCheckoutBtn) return;
  autoFillCheckoutBtn.disabled = true;
  var origText = "自动填地址";
  setText(autoFillCheckoutBtn, "探测表单...");

  try {
    var ensured = await ensureIncognitoWindow();
    if (!ensured) {
      showAutoFillNotice("自动填地址失败", "错误", "无法启动 Chrome 无痕窗口", "", "error");
      return;
    }
    if (!latestOpenedCheckoutURL) {
      showAutoFillNotice("自动填地址失败", "错误", "未找到本工具最近一次打开的支付链接页面。", "请先用本工具打开支付链接后再试。", "error");
      return;
    }

    setText(autoFillCheckoutBtn, "生成地址...");
    await new Promise(function (r) { setTimeout(r, 500); });

    var resp = await fetch("/api/checkout/auto-fill", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({ expected_url: latestOpenedCheckoutURL }),
    });
    var data = await resp.json();

    if (resp.ok && data.ok) {
      var a = data.address;
      var fields = data.filled || {};
      var validation = data.address_validation || fields.validation || {};
      var msg = "随机美国地址已填入：" + "\n" +
        "姓名: " + (a.first_name || "") + " " + (a.last_name || "") + "\n" +
        "地址: " + (a.line1 || "") + "\n" +
        "城市: " + (a.city || "") + ", " + (a.state || "") + " " + (a.zip_code || "") + "\n\n" +
        "GoPay: " + (data.gopay_selected ? "已选择" : "已保持当前选择") + "\n" +
        "地址校验: " + (validation.ok === false ? "未通过" : "通过") + "\n" +
        "填入结果: " + JSON.stringify(fields);
      showAutoFillNotice("安全辅助完成", "成功", msg, "请你本人在支付页面确认条款复选框，并手动点击订阅。系统会在提交后自动触发 GoPay 一键绑定。", "");
      setText(autoFillCheckoutBtn, "已填入 ✓");
      startCheckoutSubmitWatcher(data);
    } else {
      showAutoFillNotice("自动填地址失败", "错误", data.error || "自动填地址失败", "请先在无痕窗口中手动进入 ChatGPT Plus 升级结账页面。", "error");
    }
  } catch (err) {
    showAutoFillNotice("自动填地址失败", "错误", "网络错误: " + (err.message || "未知"), "", "error");
  } finally {
    autoFillCheckoutBtn.disabled = false;
    setText(autoFillCheckoutBtn, origText);
  }
});

// =================================
// GoPay 一键绑定
// =================================
const gopayAccountId = document.querySelector("#gopayAccountId");
const gopayCountryCode = document.querySelector("#gopayCountryCode");
const gopayPhone = document.querySelector("#gopayPhone");
const gopayOTPChannel = document.querySelector("#gopayOTPChannel");
const gopayOTPInput = document.querySelector("#gopayOTP");
const gopayPINCodeInput = document.querySelector("#gopayPINCode");
const gopayAggressiveRetryInput = document.querySelector("#gopayAggressiveRetry");
const gopayLinkBtn = document.querySelector("#gopayLinkBtn");
const gopayBadge = document.querySelector("#gopayBadge");
const gopayStepFlow = document.querySelector("#gopayStepFlow");
const gopayResult = document.querySelector("#gopayResult");
const gopayLatency = document.querySelector("#gopayLatency");
const gopayLinkStatus = document.querySelector("#gopayLinkStatus");
const gopayRefID = document.querySelector("#gopayRefID");
const gopayOutput = document.querySelector("#gopayOutput");
let gopayPaymentFlowRunning = false;
let gopayAutoTriggerRunning = false;
let gopayCheckoutWatcherActive = false;
let stopGopayOTPAutoCapture = null;
let checkoutSubmitWatcherId = 0;
const gopayAutoTriggeredCheckoutKeys = new Set();

const gopayStepItems = Array.from(document.querySelectorAll("[data-gopay-step]"));
const gopayStepTexts = {
  linking:  document.querySelector("#gopayLinkingText"),
  reference: document.querySelector("#gopayReferenceText"),
  consent:  document.querySelector("#gopayConsentText"),
  otp:      document.querySelector("#gopayOTPText"),
  pin:      document.querySelector("#gopayPINText"),
  success:  document.querySelector("#gopaySuccessText"),
};

// GoPay 表单记忆功能
(function initGopayFormMemory() {
  var GPM = "gopay_form_";
  try {
    var retryDefaultMigrationKey = GPM + "aggressive_retry_default_v2";
    if (gopayAggressiveRetryInput && localStorage.getItem(retryDefaultMigrationKey) !== "1") {
      localStorage.setItem(GPM + "aggressive_retry", "0");
      localStorage.setItem(retryDefaultMigrationKey, "1");
    }
  } catch (_err) {}
  var fields = [
    { el: gopayAccountId,    key: "account_id" },
    { el: gopayCountryCode,  key: "country_code" },
    { el: gopayPhone,        key: "phone_number" },
    { el: gopayOTPChannel,   key: "otp_channel" },
    { el: gopayPINCodeInput, key: "pin_code" },
    { el: gopayAggressiveRetryInput, key: "aggressive_retry", type: "checkbox", defaultValue: "0" },
  ];

  fields.forEach(function (f) {
    if (!f.el) return;
    // 恢复上次值
    var saved = localStorage.getItem(GPM + f.key);
    if (f.type === "checkbox") {
      if (saved !== null && saved !== undefined) {
        f.el.checked = saved === "1";
      } else if (f.defaultValue !== undefined) {
        f.el.checked = f.defaultValue === "1";
      }
    } else if (saved !== null && saved !== undefined) {
      f.el.value = saved;
    }
    // 监听变化并保存
    var persistField = function () {
      localStorage.setItem(GPM + f.key, f.type === "checkbox" ? (f.el.checked ? "1" : "0") : f.el.value);
    };
    f.el.addEventListener("input", persistField);
    f.el.addEventListener("change", persistField);
  });
})();

function setGopayStep(step, state, text) {
  const item = gopayStepItems.find(function (el) { return el.dataset.gopayStep === step; });
  if (!item) return;
  item.classList.remove("active", "done", "error", "warning");
  if (state) item.classList.add(state);
  const target = gopayStepTexts[step];
  if (target) setText(target, text || "-");
}

function resetGopaySteps() {
  ["linking","reference","consent","otp","pin","success"].forEach(function (s) {
    setGopayStep(s, "", "等待提交");
  });
  if (gopayStepFlow) gopayStepFlow.hidden = false;
  if (gopayResult) gopayResult.hidden = true;
  if (gopayOutput) setText(gopayOutput, "{}");
  if (gopayBadge) { setText(gopayBadge, "运行中"); gopayBadge.className = "badge"; }
  if (gopayLinkStatus) setText(gopayLinkStatus, "-");
  if (gopayRefID) setText(gopayRefID, "-");
  if (gopayLatency) setText(gopayLatency, "-");
}

function renderGopayStageResults(stages) {
  if (!Array.isArray(stages)) return;
  var stageMap = {};
  stages.forEach(function (s) { if (s.name) stageMap[s.name] = s; });

  // force-link-api
  if (stageMap["force-link-api"]) {
    setGopayStep("linking", stageMap["force-link-api"].ok ? "done" : "error",
      stageMap["force-link-api"].message || (
        stageMap["force-link-api"].ok
          ? "已获取 reference_id"
          : (stageMap["force-link-api"].error || "请求失败")
      ));
  }

  // validate-reference
  if (stageMap["validate-reference"]) {
    setGopayStep("reference", stageMap["validate-reference"].ok ? "done" : "error",
      stageMap["validate-reference"].message || (stageMap["validate-reference"].ok ? "引用有效" : (stageMap["validate-reference"].error || "验证失败")));
  }

  // user-consent
  if (stageMap["user-consent"]) {
    setGopayStep("consent", stageMap["user-consent"].ok ? "done" : "error",
      stageMap["user-consent"].message || (stageMap["user-consent"].ok ? "OTP 已发送" : (stageMap["user-consent"].error || "失败")));
  }

  // otp-enum
  if (stageMap["otp-enum"]) {
    if (stageMap["otp-enum"].message) {
      setGopayStep("otp", "done", stageMap["otp-enum"].message);
    } else if (stageMap["otp-enum"].ok) {
      setGopayStep("otp", "done", "OTP 通过 (" + (stageMap["otp-enum"].otp || "?") + ")");
    } else if (stageMap["otp-enum"].tried && stageMap["otp-enum"].tried.length > 0) {
      setGopayStep("otp", "warning", "自动尝试 " + stageMap["otp-enum"].tried.length + " 个码均失败");
    } else {
      setGopayStep("otp", "error", stageMap["otp-enum"].error || "失败");
    }
  }

  // pin-enum
  if (stageMap["pin-enum"]) {
    if (stageMap["pin-enum"].message) {
      setGopayStep("pin", "done", stageMap["pin-enum"].message);
    } else if (stageMap["pin-enum"].ok) {
      setGopayStep("pin", "done", "PIN 通过 (" + (stageMap["pin-enum"].pin || "?") + ")");
    } else if (stageMap["pin-enum"].tried && stageMap["pin-enum"].tried.length > 0) {
      setGopayStep("pin", "warning", "自动尝试 " + stageMap["pin-enum"].tried.length + " 个 PIN 均失败");
    } else {
      setGopayStep("pin", "error", stageMap["pin-enum"].error || "失败");
    }
  }

  // validate-pin
  if (stageMap["validate-pin"]) {
    setGopayStep("success", stageMap["validate-pin"].ok ? "done" : "error",
      stageMap["validate-pin"].message || (stageMap["validate-pin"].ok ? "✅ GoPay 绑定成功！" : (stageMap["validate-pin"].error || "失败")));
  }
}

function startGopayOTPAutoCapture() {
  if (typeof stopGopayOTPAutoCapture === "function") {
    stopGopayOTPAutoCapture();
  }
  var stop = false;
  var attempts = 0;
  var maxAttempts = 120;
  var pollDelayMs = 1500;
  var lastStageKey = "";

  var poll = async function () {
    if (stop || attempts >= maxAttempts) return;
    attempts++;

    try {
      var resp = await fetch("/api/gopay/cdp-otp", {
        method: "POST",
        headers: buildRequestHeaders(),
        body: JSON.stringify({
          otp: (gopayOTPInput?.value || "").trim(),
          pin: (gopayPINCodeInput?.value || "").trim(),
        }),
      });
      var data = await resp.json();
      var result = data.result || {};
      var pageStage = (data.stage || result.page_stage || "").trim();
      var pinStage = (data.pin_stage || result.pin_stage || "").trim();
      var otpManualRequired = !!(data.otp_manual_required ?? result.otp_manual_required);
      var pinAutoFilled = !!(data.pin_auto_filled ?? result.pin_auto_filled);
      var pinAutoSubmitted = !!(data.pin_auto_submitted ?? result.pin_auto_submitted);
      var hasOTPField = !!(data.has_otp_field ?? result.has_otp_field ?? result.hasOTPField);
      var hasPinField = !!(data.has_pin_field ?? result.has_pin_field);
      var inputStrategy = (data.pin_input_strategy || result.pin_input_strategy || "").trim();
      var balanceState = (data.balance_state || result.balance_state || "").trim();
      var balanceAmount = data.balance_amount ?? result.balance_amount;
      var hubungkanAutoClicked = !!(data.hubungkan_auto_clicked ?? result.hubungkan_auto_clicked);
      var payNowAutoClicked = !!(data.pay_now_auto_clicked ?? result.pay_now_auto_clicked);
      var autoActionPaused = !!(data.auto_action_paused ?? result.auto_action_paused);
      var autoActionStage = (data.auto_action_stage || result.auto_action_stage || "").trim();
      var stageKey = [
        pageStage,
        pinStage,
        autoActionStage,
        balanceState,
        balanceAmount,
        hubungkanAutoClicked ? "hubungkan-clicked" : "",
        payNowAutoClicked ? "pay-now-clicked" : "",
        autoActionPaused ? "paused" : "",
        otpManualRequired ? "otp-manual" : "",
        pinAutoFilled ? "pin-filled" : "",
        pinAutoSubmitted ? "pin-submitted" : "",
        inputStrategy,
      ].join("|");

      if (stageKey && stageKey !== lastStageKey) {
        lastStageKey = stageKey;
        if (pageStage === "gopay_consent_hubungkan" || hubungkanAutoClicked) {
          setGopayStep("consent", "done", "Hubungkan 已出现并自动确认");
          appendCheckoutWatcherEvent("gopay-hubungkan", "检测到 Hubungkan，已执行一次自动确认");
          if (gopayBadge) {
            setText(gopayBadge, "已确认");
            gopayBadge.className = "badge";
          }
        }
        if (pageStage === "balance_wait_rp0" || autoActionPaused || balanceState === "rp0") {
          setGopayStep("success", "warning", "余额 Rp0，已暂停自动操作并等待余额变化");
          appendCheckoutWatcherEvent("gopay-balance-wait", "检测到账户余额 Rp0，自动操作已暂停");
          if (gopayBadge) {
            setText(gopayBadge, "等余额");
            gopayBadge.className = "badge neutral";
          }
        } else if (pageStage === "pay_now_rp1" || payNowAutoClicked) {
          setGopayStep("success", "active", "余额 Rp1，已自动点击 Pay now");
          appendCheckoutWatcherEvent("gopay-pay-now", "检测到账户余额 Rp1，已点击 Pay now");
          if (gopayBadge) {
            setText(gopayBadge, "Pay now");
            gopayBadge.className = "badge";
          }
        } else if (pageStage === "balance_rp1_observed" || balanceState === "rp1") {
          setGopayStep("success", "active", "余额 Rp1，正在等待 Pay now 可点击");
          appendCheckoutWatcherEvent("gopay-pay-now-wait", "检测到账户余额 Rp1，等待 Pay now 按钮可点击");
        }
        if (pageStage === "otp_entry" || hasOTPField || otpManualRequired) {
          setGopayStep("otp", "active", "检测到 OTP 页面，等待手动输入");
          appendCheckoutWatcherEvent("gopay-otp-manual", "已进入 OTP 页面，OTP 保持手动输入");
          if (gopayBadge) {
            setText(gopayBadge, "等 OTP");
            gopayBadge.className = "badge neutral";
          }
        }
        if (pageStage === "pin_entry_binding" || pageStage === "pin_entry_payment" || hasPinField) {
          var pinLabel = pinStage === "payment" ? "支付确认 PIN" : "绑定授权 PIN";
          var pinMessage = "检测到 " + pinLabel + " 页面";
          var pinState = "active";
          if (pinAutoSubmitted) {
            pinMessage = pinLabel + " 已自动填入并提交";
            pinState = "done";
          } else if (pinAutoFilled) {
            pinMessage = pinLabel + " 已自动填入，等待页面继续";
          } else if (!(gopayPINCodeInput?.value || "").trim()) {
            pinMessage = "检测到 " + pinLabel + " 页面，但面板里还没有可复用的 PIN";
            pinState = "warning";
          }
          setGopayStep("pin", pinState, pinMessage);
          appendCheckoutWatcherEvent(
            pageStage === "pin_entry_payment" ? "gopay-payment-pin" : "gopay-binding-pin",
            pinMessage + (inputStrategy ? "（" + inputStrategy + "）" : "")
          );
          if (gopayBadge) {
            setText(gopayBadge, pinStage === "payment" ? "支付 PIN" : "绑定 PIN");
            gopayBadge.className = pinAutoSubmitted ? "badge" : "badge neutral";
          }
        }
      }
    } catch (error) {
      console.error("otp auto capture failed:", error);
    }

    if (!stop && attempts < maxAttempts) {
      window.setTimeout(poll, pollDelayMs);
    }
  };

  window.setTimeout(poll, 800);
  stopGopayOTPAutoCapture = function () { stop = true; };
  return stopGopayOTPAutoCapture;
}

function checkoutAutoTriggerKey(rawURL) {
  var value = (rawURL || "").trim();
  if (!value) return "";
  try {
    var parsed = new URL(value);
    if (parsed.hostname.toLowerCase() !== "pay.openai.com") return "";
    if (!parsed.pathname.startsWith("/c/pay/cs_")) return "";
    return parsed.protocol + "//" + parsed.host + parsed.pathname;
  } catch (_err) {
    return "";
  }
}

function checkoutAutoFillProbeText(data) {
  var chunks = [];
  ["probe_after_gopay", "probe_fallback", "probe"].forEach(function (key) {
    var text = data?.[key]?.text;
    if (typeof text === "string" && text.trim()) {
      chunks.push(text.trim());
    }
  });
  return chunks.join(" ");
}

function checkoutURLIndicatesSubmitted(currentURL, checkoutKey) {
  var value = (currentURL || "").trim();
  if (!value) return false;
  try {
    var parsed = new URL(value);
    var host = parsed.hostname.toLowerCase();
    var path = parsed.pathname.toLowerCase();
    if (host.includes("midtrans") || host.includes("gopay")) return true;
    if (path.includes("/checkout/verify") || path.includes("/snap/") || path.includes("/redirection/")) return true;
    var currentKey = checkoutAutoTriggerKey(value);
    if (checkoutKey && currentKey && currentKey !== checkoutKey) return true;
    if (checkoutKey && !currentKey && host !== "pay.openai.com") return true;
    return false;
  } catch (_err) {
    var lower = value.toLowerCase();
    return lower.includes("midtrans.com") || lower.includes("app.gopay") || lower.includes("/snap/");
  }
}

function midtransLinkingAccountID(currentURL) {
  var value = (currentURL || "").trim();
  if (!value) return "";
  try {
    var parsed = new URL(value);
    if (!parsed.hostname.toLowerCase().includes("midtrans.com")) return "";
    if (!parsed.hash.toLowerCase().includes("gopay-tokenization/linking")) return "";
    var match = parsed.pathname.match(/\/snap\/v4\/redirection\/([^/?#]+)/i);
    return match ? decodeURIComponent(match[1]) : "";
  } catch (_err) {
    return "";
  }
}

function midtransRedirectionAccountID(currentURL) {
  var value = (currentURL || "").trim();
  if (!value) return "";
  try {
    var parsed = new URL(value);
    if (!parsed.hostname.toLowerCase().includes("midtrans.com")) return "";
    var match = parsed.pathname.match(/\/snap\/v4\/redirection\/([^/?#]+)/i);
    return match ? decodeURIComponent(match[1]) : "";
  } catch (_err) {
    return "";
  }
}

function appendCheckoutWatcherEvent(method, summary) {
  if (gopayMonitor) {
    gopayMonitor.hidden = false;
  }
  if (typeof appendMonitorEvent === "function") {
    appendMonitorEvent({ domain: "Log", method: method, summary: summary, ts: Date.now() });
  }
}

function prepareCheckoutWatcherMonitor() {
  if (gopayMonitor) {
    gopayMonitor.hidden = false;
  }
  if (!gopayMonitorRunning && !sessionMonitorAbortController) {
    clearMonitor();
  }
  if (monitorBadge) {
    monitorBadge.textContent = "等待订阅";
    monitorBadge.className = "badge";
  }
}

async function checkGopayAutoTriggerReady(payload) {
  var response = await fetch("/api/gopay/auto-trigger-check", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify(payload),
  });
  var data = await response.json();
  if (!response.ok) {
    throw new Error(data.error || "GoPay 自动触发检查失败");
  }
  return data;
}

async function fillCurrentMidtransLinkingPage(targetURL, checkoutURL) {
  var countryCode = (gopayCountryCode?.value || "86").trim();
  var phoneNumber = (gopayPhone?.value || "18120322232").trim();
  var aggressiveRetry = !!gopayAggressiveRetryInput?.checked;
  var debugNetworkOnce = false;
  try {
    debugNetworkOnce = window.localStorage?.getItem("gopay_debug_network_once") === "1";
    if (debugNetworkOnce) window.localStorage?.removeItem("gopay_debug_network_once");
  } catch (_err) {
    debugNetworkOnce = false;
  }
  var response = await fetch("/api/gopay/midtrans-linking-fill", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify({
      target_url: targetURL,
      checkout_url: checkoutURL || "",
      country_code: countryCode,
      phone_number: phoneNumber,
      aggressive_retry: aggressiveRetry,
      debug_network: debugNetworkOnce,
    }),
  });
  var data = await response.json();
  if (!response.ok) {
    throw new Error(data.error || "Midtrans GoPay 页面填充失败");
  }
  return data;
}

function startCheckoutSubmitWatcher(autoFillData) {
  var checkoutURL = autoFillData?.current_url || autoFillData?.expected_url || latestOpenedCheckoutURL;
  var checkoutKey = checkoutAutoTriggerKey(checkoutURL);
  var pageText = checkoutAutoFillProbeText(autoFillData);

  if (!checkoutKey) {
    appendCheckoutWatcherEvent("auto-trigger-skip", "未识别到 pay.openai.com checkout URL，跳过自动触发监控");
    return;
  }
  if (gopayAutoTriggeredCheckoutKeys.has(checkoutKey)) {
    appendCheckoutWatcherEvent("auto-trigger-skip", "该 checkout 已触发过 GoPay 一键绑定，跳过重复监控");
    return;
  }

  checkoutSubmitWatcherId += 1;
  var watcherId = checkoutSubmitWatcherId;
  var attempts = 0;
  var maxAttempts = 45;
  var lastCurrentURL = "";
  var watcherButtonText = gopayLinkBtn ? gopayLinkBtn.textContent : "";
  var finishWatcher = function () {
    if (watcherId !== checkoutSubmitWatcherId) return;
    gopayCheckoutWatcherActive = false;
    if (gopayLinkBtn && !gopayPaymentFlowRunning) {
      gopayLinkBtn.disabled = false;
      setText(gopayLinkBtn, watcherButtonText || "GoPay 一键绑定");
      gopayLinkBtn.removeAttribute("title");
    }
  };

  prepareCheckoutWatcherMonitor();
  gopayCheckoutWatcherActive = true;
  if (gopayLinkBtn) {
    gopayLinkBtn.disabled = true;
    gopayLinkBtn.title = "自动监控已接管本次 checkout，请等待 Midtrans linking 页面出现。";
    setText(gopayLinkBtn, "自动监控中...");
  }
  appendCheckoutWatcherEvent("auto-trigger-watch", "已开始等待支付页订阅提交：" + checkoutKey);

  void checkGopayAutoTriggerReady({
    source: "checkout-auto-fill-watch",
    checkout_url: checkoutURL,
    page_text: pageText,
    submitted: false,
  }).then(function (decision) {
    appendCheckoutWatcherEvent("auto-trigger-wait", "自动触发判定：" + (decision.reason || "waiting"));
  }).catch(function (error) {
    appendCheckoutWatcherEvent("auto-trigger-check-error", error.message || "自动触发预检查失败");
  });

  var poll = async function () {
    if (watcherId !== checkoutSubmitWatcherId || gopayAutoTriggeredCheckoutKeys.has(checkoutKey)) {
      finishWatcher();
      return;
    }
    attempts += 1;

    try {
      var response = await fetch("/api/checkout/resolve-target", {
        method: "POST",
        headers: buildRequestHeaders(),
        body: JSON.stringify({ opened_url: checkoutURL }),
      });
      var data = await response.json();
      var currentURL = data.current_url || data.target?.url || "";
      var candidateURLs = Array.isArray(data.candidate_urls) ? data.candidate_urls : [];
      var submittedCandidateURL = candidateURLs.find(function (url) {
        return checkoutURLIndicatesSubmitted(url, checkoutKey);
      }) || "";
      if (!currentURL && submittedCandidateURL) {
        currentURL = submittedCandidateURL;
      }
      var linkingURL = midtransRedirectionAccountID(currentURL) ? currentURL : "";
      if (!linkingURL) {
        linkingURL = candidateURLs.find(function (url) {
          return !!midtransRedirectionAccountID(url);
        }) || "";
      }
      var hasMidtransCandidate = !!linkingURL;
      if (currentURL && currentURL !== lastCurrentURL) {
        lastCurrentURL = currentURL;
        appendCheckoutWatcherEvent("checkout-url", currentURL);
      } else if (hasMidtransCandidate && linkingURL !== lastCurrentURL) {
        lastCurrentURL = linkingURL;
        appendCheckoutWatcherEvent("checkout-url", linkingURL);
      }

      if (response.ok && (data.ok || submittedCandidateURL || hasMidtransCandidate) && (checkoutURLIndicatesSubmitted(currentURL, checkoutKey) || hasMidtransCandidate)) {
        appendCheckoutWatcherEvent("checkout-submitted", hasMidtransCandidate ? "检测到 Midtrans GoPay redirection 页面，开始 GoPay 自动触发检查" : "检测到支付页已提交，开始 GoPay 自动触发检查");
        var decision = await checkGopayAutoTriggerReady({
          source: "checkout-submit-watch",
          checkout_url: checkoutURL,
          page_text: pageText,
          submitted: true,
        });

        if (decision.ready) {
          if (monitorBadge) {
            monitorBadge.textContent = "自动触发";
            monitorBadge.className = "badge";
          }
          if (!linkingURL) {
            appendCheckoutWatcherEvent("auto-trigger-wait", "支付页已提交，继续等待 Midtrans GoPay redirection 页面");
            if (monitorBadge) {
              monitorBadge.textContent = "等 Midtrans";
              monitorBadge.className = "badge neutral";
            }
          } else if (!gopayAutoTriggerRunning) {
            appendCheckoutWatcherEvent("auto-trigger-ready", "条件满足，填写当前 Midtrans GoPay 页面：" + midtransRedirectionAccountID(linkingURL));
            gopayAutoTriggerRunning = true;
            try {
              var fillResult = await fillCurrentMidtransLinkingPage(linkingURL, checkoutURL);
              if (gopayOutput) setText(gopayOutput, JSON.stringify(fillResult, null, 2));
              if (fillResult.ok) {
                gopayAutoTriggeredCheckoutKeys.add(checkoutKey);
                appendCheckoutWatcherEvent("midtrans-linking-fill", "已填写 +" + (fillResult.country_code || "86") + " / " + (fillResult.phone_number || "18120322232") + " 并提交 Link and pay");
                if (monitorBadge) {
                  monitorBadge.textContent = "已提交绑定";
                  monitorBadge.className = "badge";
                }
                startGopayOTPAutoCapture();
                finishWatcher();
                return;
              } else {
                var retryHint = fillResult.aggressive_retry ? "（连续重试已开启）" : "";
                appendCheckoutWatcherEvent("midtrans-linking-fill", "页面填充未完成：" + (fillResult.stage || fillResult.error || "unknown") + retryHint);
                if (monitorBadge) {
                  monitorBadge.textContent = "继续等待";
                  monitorBadge.className = "badge neutral";
                }
              }
            } finally {
              gopayAutoTriggerRunning = false;
            }
          }
          if (gopayAutoTriggeredCheckoutKeys.has(checkoutKey)) return;
        }

        if (!decision.ready) {
          appendCheckoutWatcherEvent("auto-trigger-skip", "自动触发检查未通过：" + (decision.reason || "unknown"));
          if (monitorBadge) {
            monitorBadge.textContent = "未触发";
            monitorBadge.className = "badge neutral";
          }
          finishWatcher();
          return;
        }
      }
    } catch (error) {
      appendCheckoutWatcherEvent("auto-trigger-error", error.message || "自动触发监控失败");
    }

    if (attempts < maxAttempts) {
      window.setTimeout(poll, 2000);
    } else {
      appendCheckoutWatcherEvent("auto-trigger-timeout", "等待订阅提交超时，未自动触发 GoPay 一键绑定");
      if (monitorBadge) {
        monitorBadge.textContent = "等待超时";
        monitorBadge.className = "badge neutral";
      }
      finishWatcher();
    }
  };

  window.setTimeout(poll, 1500);
}

async function runGopayFullLinkPayment(options) {
  options = options || {};
  var triggerSource = options.triggerSource || "manual";
  if (gopayPaymentFlowRunning) {
    showAutoFillNotice("GoPay 付款进行中", "提示", "当前一键绑定付款流程仍在执行，请等待本次流程完成。", "", "");
    return null;
  }

  var countryCode = (gopayCountryCode?.value || "86").trim();
  var phoneNumber = (gopayPhone?.value || "18120322232").trim();
  var otpChannel = gopayOTPChannel?.value || "whatsapp";
  var otpCode = (gopayOTPInput?.value || "").trim();
  var pinCode = (gopayPINCodeInput?.value || "").trim();
  var accessToken = extractAccessToken(fields.token?.value || "");

  if (!phoneNumber) {
    showAutoFillNotice("GoPay 绑定失败", "错误", "请输入手机号。", "", "error");
    return;
  }

  resetGopaySteps();
  gopayPaymentFlowRunning = true;
  if (gopayLinkBtn) gopayLinkBtn.disabled = true;
  if (gopayMonitorBtn) gopayMonitorBtn.disabled = true;
  var origText = gopayLinkBtn ? gopayLinkBtn.textContent : "";
  setText(gopayLinkBtn, triggerSource === "auto-trigger" ? "自动触发中..." : "提取 Session 中...");

  var startedAt = performance.now();

  try {
    if (!accessToken) {
      await fetchLatestSessionJSON();
      accessToken = extractAccessToken(fields.token?.value || "");
    }
    if (!accessToken) {
      throw new Error("无法自动提取最新 Session JSON 或 access token");
    }

    setText(gopayLinkBtn, "创建新链路中...");
    setGopayStep("linking", "active", "正在提取最新 Session 并生成新的 Plus 结账链路...");
    startGopayOTPAutoCapture();

    var payload = {
      access_token: accessToken,
      country_code: countryCode,
      phone_number: phoneNumber,
      otp_channel: otpChannel,
    };
    if (otpCode) payload.otp = otpCode;
    if (pinCode) payload.pin = pinCode;

    var resp = await fetch("/api/gopay/full-link", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify(payload),
    });
    var data = await resp.json();
    var elapsed = Math.round(performance.now() - startedAt);

    if (data.account_id && gopayAccountId) {
      gopayAccountId.value = data.account_id;
      localStorage.setItem("gopay_form_account_id", data.account_id);
    }

    if (gopayStepFlow) gopayStepFlow.hidden = false;
    if (gopayResult) gopayResult.hidden = false;
    if (gopayLatency) setText(gopayLatency, elapsed + "ms");
    if (gopayLinkStatus) {
      var okText = "✅ 已绑定";
      if (data.stage === "gopay_complete") {
        okText = "✅ 支付已完成";
      } else if (data.reused_existing) {
        okText = "✅ 已复用已绑定账号";
      }
      setText(gopayLinkStatus, data.ok ? okText : "❌ " + (data.stage || "失败"));
    }
    if (gopayRefID) setText(gopayRefID, (data.reference_id || data.account_id || "-").slice(0, 36));
    if (gopayOutput) setText(gopayOutput, JSON.stringify(data, null, 2));

    if (data.stages) {
      renderGopayStageResults(data.stages);
    } else {
      var manualStages = [];
      if (data.stage === "linking_success" || data.stage === "gopay_complete") {
        manualStages = [
          { name: "force-link-api", ok: true },
          { name: "validate-reference", ok: true },
          { name: "user-consent", ok: true },
          { name: "otp-enum", ok: true, otp: data.summary?.otp },
          { name: "pin-enum", ok: true, pin: data.summary?.pin || data.summary?.payment_pin },
          { name: "validate-pin", ok: true },
        ];
      } else if (data.stage) {
        var stageToErrorMap = {
          checkout_api_failed: "linking",
          no_account_id: "linking",
          force_link_failed: "linking",
          validate_reference_failed: "reference",
          user_consent_failed: "consent",
          otp_all_failed: "otp",
          pin_all_failed: "pin",
          payment_pin_all_failed: "pin",
          gopay_payment_failed: "pin",
          validate_pin_failed: "success",
        };
        var uiStage = stageToErrorMap[data.stage];
        if (uiStage) {
          manualStages.push({ name: uiStage === "linking" ? "force-link-api" : uiStage, ok: false, error: data.error || data.stage });
        }
      }
      if (manualStages.length > 0) renderGopayStageResults(manualStages);
    }

    if (data.ok) {
      var successText = "✅ GoPay 绑定成功！";
      var badgeText = data.reused_existing ? "已复用" : "已绑定";
      if (data.stage === "gopay_complete") {
        successText = "✅ 支付已完成";
        badgeText = "支付完成";
      } else if (data.reused_existing) {
        successText = "✅ 已绑定账号，已直接复用";
      }
      setGopayStep("success", "done", successText);
      if (gopayBadge) { setText(gopayBadge, badgeText); gopayBadge.className = "badge"; }
    } else if (data.stage === "otp_all_failed") {
      if (gopayBadge) { setText(gopayBadge, "需真实验证码"); gopayBadge.className = "badge error"; }
      showAutoFillNotice("需要真实 OTP", "提示", "自动尝试沙箱测试码均失败。", "请输入从 WhatsApp/SMS 收到的 6 位真实 OTP 验证码后重试。", "error");
    } else if (data.stage === "pin_all_failed" || data.stage === "payment_pin_all_failed") {
      if (gopayBadge) { setText(gopayBadge, data.stage === "payment_pin_all_failed" ? "需支付 PIN" : "PIN 未通过"); gopayBadge.className = "badge error"; }
      showAutoFillNotice(
        data.stage === "payment_pin_all_failed" ? "需要支付 PIN" : "需要真实 GoPay PIN",
        "提示",
        data.stage === "payment_pin_all_failed"
          ? "当前账号已复用到支付阶段，但自动尝试支付 PIN 失败。"
          : "需要输入真实 GoPay PIN，当前自动尝试的 PIN 均未通过。",
        data.stage === "payment_pin_all_failed"
          ? "请在 GoPay PIN 输入框里填入真实 6 位 PIN 后重试。"
          : "",
        "error"
      );
    } else {
      if (gopayBadge) { setText(gopayBadge, "失败"); gopayBadge.className = "badge error"; }
    }

    return data;
  } catch (err) {
    var elapsed = Math.round(performance.now() - startedAt);
    if (gopayLatency) setText(gopayLatency, elapsed + "ms");
    if (gopayLinkStatus) setText(gopayLinkStatus, "❌ 网络错误");
    if (gopayOutput) setText(gopayOutput, JSON.stringify({ error: err.message }));
    if (gopayBadge) { setText(gopayBadge, "错误"); gopayBadge.className = "badge error"; }
    setGopayStep("linking", "error", "网络错误: " + err.message);
    return null;
  } finally {
    gopayPaymentFlowRunning = false;
    if (gopayLinkBtn) gopayLinkBtn.disabled = false;
    if (gopayMonitorBtn) gopayMonitorBtn.disabled = false;
    setText(gopayLinkBtn, origText);
  }
}

gopayLinkBtn?.addEventListener("click", async function () {
  if (gopayCheckoutWatcherActive) {
    appendCheckoutWatcherEvent("manual-full-link-blocked", "自动监控已接管本次 checkout，已阻止手动 full-link 路径");
    showAutoFillNotice("自动监控运行中", "提示", "当前 checkout 已由自动监控接管。请在支付页点击订阅后等待程序自动填写 Midtrans GoPay 页面。", "手动 GoPay 一键绑定会新建后端 full-link 流程，容易造成回跳状态错乱。", "");
    return;
  }
  await runGopayFullLinkPayment({ triggerSource: "manual" });
});

// =================================
// GoPay 流程监控
// =================================
const gopayMonitorBtn = document.querySelector("#gopayMonitorBtn");
const gopayMonitor = document.querySelector("#gopayMonitor");
const monitorBadge = document.querySelector("#monitorBadge");
const monitorSummary = document.querySelector("#monitorSummary");
const monitorLog = document.querySelector("#monitorLog");
const monitorOutput = document.querySelector("#monitorOutput");
const monitorNetCount = document.querySelector("#monitorNetCount");
const monitorPageCount = document.querySelector("#monitorPageCount");
const monitorAPICount = document.querySelector("#monitorAPICount");
const monitorErrorCount = document.querySelector("#monitorErrorCount");

let gopayMonitorAbortController = null;
let gopayMonitorRunning = false;
let sessionMonitorAbortController = null;

const DOMAIN_ICONS = {
  Network:  "\uD83C\uDF10",
  Page:     "\uD83D\uDCC4",
  Console:  "\uD83D\uDCDD",
  Error:    "\u274C",
  Log:      "\u2139\uFE0F",
};

const DOMAIN_COLORS = {
  Network:  "#2563eb",
  Page:     "#059669",
  Console:  "#d97706",
  Error:    "#dc2626",
  Log:      "#6b7280",
};

function clearMonitor() {
  if (monitorLog) monitorLog.innerHTML = '<div class="monitor-empty">等待数据...</div>';
  if (monitorSummary) monitorSummary.hidden = true;
  if (monitorOutput) { monitorOutput.hidden = true; monitorOutput.textContent = "{}"; }
  if (monitorNetCount) monitorNetCount.textContent = "0";
  if (monitorPageCount) monitorPageCount.textContent = "0";
  if (monitorAPICount) monitorAPICount.textContent = "0";
  if (monitorErrorCount) monitorErrorCount.textContent = "0";
}

function setMonitorSummary(summary) {
  if (!summary) return;
  if (monitorSummary) monitorSummary.hidden = false;
  if (monitorNetCount) monitorNetCount.textContent = summary.network_calls || 0;
  if (monitorPageCount) monitorPageCount.textContent = summary.page_navigations || 0;
  if (monitorAPICount) {
    monitorAPICount.textContent =
      (summary.openai_api_calls || 0) +
      (summary.stripe_api_calls || 0) +
      (summary.snap_api_calls || 0) +
      (summary.gopay_api_calls || 0);
  }
  if (monitorErrorCount) monitorErrorCount.textContent = summary.errors || 0;
}

async function startGopayMonitorStream(durationS) {
  gopayMonitorAbortController = new AbortController();
  gopayMonitorRunning = true;

  var resp = await fetch("/api/gopay/monitor", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify({ duration_s: durationS, stream: true }),
    signal: gopayMonitorAbortController.signal,
  });

  if (!resp.ok || !resp.body) {
    throw new Error("监控启动失败");
  }

  var reader = resp.body.getReader();
  var decoder = new TextDecoder();
  var buffer = "";
  var fullOutput = [];

  while (true) {
    var chunk = await reader.read();
    if (chunk.done) break;
    buffer += decoder.decode(chunk.value, { stream: true });
    var lines = buffer.split("\n");
    buffer = lines.pop() || "";

    for (var i = 0; i < lines.length; i++) {
      var line = lines[i].trim();
      if (!line) continue;
      var msg = JSON.parse(line);
      fullOutput.push(msg);

      if (monitorOutput) {
        monitorOutput.hidden = false;
        monitorOutput.textContent = JSON.stringify(fullOutput, null, 2);
      }

      if (msg.type === "started") {
        appendMonitorEvent({ domain: "Log", method: "monitor-started", summary: (msg.targets || []).join(" | "), ts: Date.now() });
      } else if (msg.type === "event" && msg.event) {
        appendMonitorEvent(msg.event);
      } else if ((msg.type === "summary" || msg.type === "done") && msg.summary) {
        setMonitorSummary(msg.summary);
      } else if (msg.type === "error") {
        appendMonitorEvent({ domain: "Error", method: "monitor-error", summary: msg.error || "未知错误", ts: Date.now() });
      }
    }
  }
}

function appendMonitorEvent(evt) {
  if (!monitorLog) return;
  if (monitorLog.querySelector(".monitor-empty")) {
    monitorLog.innerHTML = "";
  }

  var icon = DOMAIN_ICONS[evt.domain] || "\u25CF";
  var color = DOMAIN_COLORS[evt.domain] || "#6b7280";
  var time = new Date(evt.ts).toLocaleTimeString();

  var row = document.createElement("div");
  row.className = "monitor-row";
  row.style.borderLeftColor = color;
  row.innerHTML =
    '<span class="monitor-icon">' + icon + '</span>' +
    '<span class="monitor-time">' + time + '</span>' +
    '<span class="monitor-method" style="color:' + color + '">[' + evt.method + ']</span>' +
    '<span class="monitor-text">' + escapeHTML(evt.summary) + '</span>';
  monitorLog.appendChild(row);

  var MAX_ROWS = 300;
  while (monitorLog.children.length > MAX_ROWS) {
    monitorLog.removeChild(monitorLog.firstChild);
  }
  monitorLog.scrollTop = monitorLog.scrollHeight;
}

function escapeHTML(str) {
  return String(str).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

gopayMonitorBtn?.addEventListener("click", async function () {
  if (!gopayMonitor) return;

  if (gopayPaymentFlowRunning) {
    showAutoFillNotice("GoPay 付款进行中", "提示", "一键绑定付款流程运行期间暂不启动或停止流程监控，避免中断付款状态。", "", "");
    return;
  }

  if (gopayMonitorRunning) {
    if (gopayMonitorAbortController) {
      gopayMonitorAbortController.abort();
    }
    gopayMonitorRunning = false;
    gopayMonitorBtn.textContent = "开始流程监控";
    if (monitorBadge) { monitorBadge.textContent = "已停止"; monitorBadge.className = "badge neutral"; }
    appendMonitorEvent({ domain: "Log", method: "monitor-stop", summary: "用户已停止监控", ts: Date.now() });
    return;
  }

  if (sessionMonitorAbortController) {
    sessionMonitorAbortController.abort();
    sessionMonitorAbortController = null;
    if (monitorBadge) { monitorBadge.textContent = "Session 监控已停止"; monitorBadge.className = "badge neutral"; }
    appendMonitorEvent({ domain: "Log", method: "session-monitor-stop", summary: "用户已停止 Session 监控", ts: Date.now() });
    return;
  }

  gopayMonitor.hidden = false;
  gopayMonitorBtn.textContent = "停止监控";
  clearMonitor();

  if (monitorBadge) { monitorBadge.textContent = "监控中"; monitorBadge.className = "badge"; }
  if (monitorSummary) monitorSummary.hidden = true;

  var durationS = 25;

  try {
    await startGopayMonitorStream(durationS);
    if (monitorBadge) { monitorBadge.textContent = "完成"; monitorBadge.className = "badge"; }
  } catch (err) {
    if (err && err.name === "AbortError") {
      if (monitorBadge) { monitorBadge.textContent = "已停止"; monitorBadge.className = "badge neutral"; }
    } else {
      if (monitorBadge) { monitorBadge.textContent = "错误"; monitorBadge.className = "badge error"; }
      appendMonitorEvent({ domain: "Error", method: "monitor-error", summary: err.message, ts: Date.now() });
    }
  } finally {
    gopayMonitorRunning = false;
    gopayMonitorAbortController = null;
    gopayMonitorBtn.textContent = "开始流程监控";
  }
});

bindWechatGroupCard();
void checkHealth();
