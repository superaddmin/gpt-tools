const form = document.querySelector("#checkoutForm");
const submitButton = document.querySelector("#submitButton");
const paypalSubmitButton = document.querySelector("#paypalSubmitButton");
const paymentSubmitButtons = [submitButton, paypalSubmitButton].filter(Boolean);
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
const exportSub2APIBtn = document.querySelector("#exportSub2APIBtn");
const incognitoDiagBadge = document.querySelector("#incognitoDiagBadge");
const incognitoDiagCDP = document.querySelector("#incognitoDiagCDP");
const incognitoDiagTargets = document.querySelector("#incognitoDiagTargets");
const incognitoDiagReadyState = document.querySelector("#incognitoDiagReadyState");
const incognitoDiagLoad = document.querySelector("#incognitoDiagLoad");
const incognitoDiagMessage = document.querySelector("#incognitoDiagMessage");
const incognitoDiagRefreshBtn = document.querySelector("#incognitoDiagRefreshBtn");
const incognitoDiagOpenBtn = document.querySelector("#incognitoDiagOpenBtn");
const incognitoDiagSlow = document.querySelector("#incognitoDiagSlow");
const incognitoDiagOutput = document.querySelector("#incognitoDiagOutput");
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
let activePaymentLabel = "Popay";
let incognitoWindowOpened = false;
let loginClickReady = false;
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

function parseJSONText(rawValue) {
  try {
    return JSON.parse(String(rawValue || ""));
  } catch (_error) {
    return null;
  }
}

function decodeBase64URL(value) {
  var input = String(value || "").replace(/-/g, "+").replace(/_/g, "/");
  while (input.length % 4) input += "=";
  return decodeURIComponent(Array.prototype.map.call(atob(input), function (char) {
    return "%" + ("00" + char.charCodeAt(0).toString(16)).slice(-2);
  }).join(""));
}

function decodeJWTPayload(token) {
  var parts = String(token || "").split(".");
  if (parts.length < 2) return {};
  return parseJSONText(decodeBase64URL(parts[1])) || {};
}

function isoFromUnixSeconds(value) {
  var number = Number(value || 0);
  if (!Number.isFinite(number) || number <= 0) return "";
  return new Date(number * 1000).toISOString();
}

function sanitizeFilenamePart(value) {
  return String(value || "openai").replace(/[^a-zA-Z0-9._-]+/g, "_").replace(/^_+|_+$/g, "").slice(0, 80) || "openai";
}

function downloadJSONFile(filename, payload) {
  var blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" });
  var url = URL.createObjectURL(blob);
  var link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function buildSub2APIExport(sessionTextValue) {
  var parsed = parseJSONText(sessionTextValue) || {};
  var accessToken = extractAccessToken(sessionTextValue);
  var idToken = parsed.idToken || parsed.id_token || "";
  var refreshToken = parsed.refreshToken || parsed.refresh_token || "";
  var accessPayload = decodeJWTPayload(accessToken);
  var idPayload = decodeJWTPayload(idToken);
  var auth = accessPayload["https://api.openai.com/auth"] || idPayload["https://api.openai.com/auth"] || {};
  var profile = accessPayload["https://api.openai.com/profile"] || {};
  var email = parsed?.user?.email || parsed.email || profile.email || idPayload.email || "";
  var accountName = email || parsed?.user?.name || "openai";
  var exportedAt = new Date().toISOString();
  var datePart = exportedAt.slice(0, 10);
  var credentials = {
    access_token: accessToken,
    expires_at: isoFromUnixSeconds(accessPayload.exp) || parsed.expires || parsed.expires_at,
    refresh_token: refreshToken,
    id_token: idToken,
    email: email,
    chatgpt_account_id: auth.chatgpt_account_id || "",
    chatgpt_user_id: auth.chatgpt_user_id || accessPayload["https://api.openai.com/auth"]?.user_id || "",
    plan_type: auth.chatgpt_plan_type || "",
    subscription_expires_at: auth.chatgpt_subscription_active_until || ""
  };
  return {
    filename: sanitizeFilenamePart(accountName) + "_sub2api_" + datePart + ".json",
    payload: {
      exported_at: exportedAt,
      proxies: [],
      accounts: [{
        name: accountName,
        platform: "openai",
        type: "oauth",
        credentials: credentials,
        concurrency: 0,
        priority: 0
      }],
      type: "sub2api-data",
      version: 1
    },
    missing: Object.keys(credentials).filter(function (key) { return !credentials[key]; })
  };
}

async function exportSub2APICredentials() {
  if (!exportSub2APIBtn) return;
  if (!window.confirm("即将导出 sub2api 登录凭证 JSON。该文件包含可用登录凭证，请只在本人账号和本机环境使用，并妥善保存。是否继续？")) return;
  var originalText = exportSub2APIBtn.textContent;
  exportSub2APIBtn.disabled = true;
  if (fetchSessionBtn) fetchSessionBtn.disabled = true;
  setText(exportSub2APIBtn, "读取 Session...");
  try {
    var sessionJSON = await fetchLatestSessionJSON();
    setText(exportSub2APIBtn, "生成 JSON...");
    var result = buildSub2APIExport(sessionJSON);
    if (!result.payload.accounts[0].credentials.access_token) {
      throw new Error("未提取到 access_token，无法生成 sub2api 凭证文件");
    }
    downloadJSONFile(result.filename, result.payload);
    if (monitorBadge) { monitorBadge.textContent = "凭证已导出"; monitorBadge.className = "badge"; }
    showAutoFillNotice("sub2api 凭证已导出", "成功", "已按参考文件格式生成 JSON 下载文件。" + (result.missing.length ? " 未获取字段：" + result.missing.join(", ") : ""), "请妥善保存该文件，不要上传到公开仓库或聊天窗口。", "");
  } catch (error) {
    if (monitorBadge) { monitorBadge.textContent = "导出失败"; monitorBadge.className = "badge error"; }
    showAutoFillNotice("sub2api 导出失败", "错误", error.message || "导出 sub2api 凭证失败", "请确认无痕窗口已登录 ChatGPT 后重试。", "error");
  } finally {
    sessionMonitorAbortController = null;
    exportSub2APIBtn.disabled = false;
    if (fetchSessionBtn) fetchSessionBtn.disabled = false;
    setText(exportSub2APIBtn, originalText || "导出 sub2api 凭证");
  }
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

function buildStartPayload(options) {
  options = options || {};
  var billingDetails = options.billingDetails || {};
  var paymentMethod = String(options.paymentMethod || "gopay").trim().toLowerCase();
  var country = String(billingDetails.country || fields.country.value || "").trim().toUpperCase();
  var currency = String(billingDetails.currency || fields.currency.value || "").trim().toUpperCase();
  fields.token.value = extractAccessToken(fields.token.value);
  fields.customerEmail.value = randomGmail();
  return {
    token: fields.token.value.trim(),
    entry_point: fields.entryPoint.value.trim(),
    plan_name: fields.planName.value.trim(),
    payment_method: paymentMethod,
    billing_details: {
      country: country,
      currency: currency,
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
      loginClickReady = false;
      return true;
    }
    console.error("incognito open failed:", data);
    return false;
  } catch (error) {
    console.error("incognito open error:", error);
    return false;
  }
}

function formatDiagMs(value) {
  var number = Number(value || 0);
  if (!Number.isFinite(number) || number <= 0) return "-";
  return Math.round(number) + "ms";
}

function renderIncognitoDiagnostics(data) {
  if (incognitoDiagOutput) {
    incognitoDiagOutput.textContent = JSON.stringify(data || {}, null, 2);
  }
  var conclusion = data?.conclusion || {};
  var status = String(conclusion.status || (data?.cdp_ready ? "ready" : "cdp_not_ready"));
  var ok = !!data?.cdp_ready && status !== "cdp_not_ready" && status !== "probe_failed";
  if (incognitoDiagBadge) {
    setText(incognitoDiagBadge, status);
    incognitoDiagBadge.className = ok ? "badge" : (data?.cdp_ready ? "badge neutral" : "badge error");
  }
  setText(incognitoDiagCDP, data?.cdp_ready ? "LISTENING" : "未就绪");
  setText(incognitoDiagTargets, typeof data?.target_count === "number" ? String(data.target_count) : "-");
  setText(incognitoDiagReadyState, data?.page_probe?.ready_state || "-");
  setText(incognitoDiagLoad, formatDiagMs(data?.page_probe?.nav?.duration_ms));
  setText(incognitoDiagMessage, conclusion.message || data?.probe_error || data?.target_error || "诊断完成");

  var slow = Array.isArray(data?.page_probe?.slow_resources) ? data.page_probe.slow_resources : [];
  if (incognitoDiagSlow) {
    incognitoDiagSlow.hidden = slow.length === 0;
    incognitoDiagSlow.innerHTML = "";
    slow.slice(0, 6).forEach(function (item) {
      var row = document.createElement("div");
      row.className = "incognito-diag-slow-row";
      row.innerHTML =
        '<strong>' + escapeHTML(item.host || "unknown") + '</strong>' +
        '<span>' + escapeHTML(item.type || "resource") + " · " + formatDiagMs(item.duration_ms) + '</span>' +
        '<small>' + escapeHTML(item.path || "-") + '</small>';
      incognitoDiagSlow.appendChild(row);
    });
  }
}

async function refreshIncognitoDiagnostics(options) {
  options = options || {};
  var activeButton = options.openFirst ? incognitoDiagOpenBtn : incognitoDiagRefreshBtn;
  var originalText = activeButton?.textContent || "刷新诊断";
  if (activeButton) {
    activeButton.disabled = true;
    setText(activeButton, options.openFirst ? "打开中..." : "诊断中...");
  }
  if (incognitoDiagBadge) {
    setText(incognitoDiagBadge, "诊断中");
    incognitoDiagBadge.className = "badge neutral";
  }
  try {
    if (options.openFirst) {
      var opened = await ensureIncognitoWindow();
      if (!opened) throw new Error("无法打开无痕窗口");
      await new Promise(function (r) { setTimeout(r, 1500); });
    }
    var response = await fetch("/api/incognito/diagnostics", { method: "GET", headers: buildRequestHeaders() });
    var data = await response.json();
    renderIncognitoDiagnostics(data);
    if (!response.ok) {
      throw new Error(data.error || "无痕诊断请求失败");
    }
    return data;
  } catch (error) {
    renderIncognitoDiagnostics({ ok: false, cdp_ready: false, error: error.message, conclusion: { status: "diagnostics_failed", message: error.message || "无痕诊断失败" } });
    return null;
  } finally {
    if (activeButton) {
      activeButton.disabled = false;
      setText(activeButton, originalText);
    }
  }
}

async function runLoginClickAfterIncognitoOpen(options) {
  var returnData = !!(options && options.returnData);
  if (loginClickReady) return returnData ? { ok: true, stage: "login_click_cached_ready" } : true;
  if (luckmailResult) luckmailResult.hidden = false;
  if (luckmailBadge) { setText(luckmailBadge, "点登录中"); luckmailBadge.className = "badge neutral"; }
  setText(luckmailMailMeta, "无痕窗口已打开，正在监控首页加载、网络和登录按钮状态；若判断为网络延迟会持续等待，其他异常会自动切换处理策略...");
  announceLuckMailScene("login_click_started", "无痕窗口已打开，正在监控登录入口与页面状态");
  renderLuckMailOutput({ stage: "login_click_started" });
  try {
    const response = await fetch("/api/login/click", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({ timeout_s: 45 }),
    });
    const data = await response.json();
    renderLuckMailOutput(data);
    var alreadyLoggedIn = data.stage === "already_logged_in";
    loginClickReady = !!(response.ok && data.ok && (data.email_input_ready || data.email_mode_switched || data.login_surface_ready || alreadyLoggedIn));
    var targetSwitching = data.stage && String(data.stage).includes("target_switching");
    if (luckmailBadge) {
      setText(luckmailBadge, alreadyLoggedIn ? "已登录" : (loginClickReady ? "登录窗已开" : (targetSwitching ? "页面跳转中" : "点击失败")));
      luckmailBadge.className = loginClickReady || targetSwitching ? "badge neutral" : "badge error";
    }
    setText(luckmailMailMeta, alreadyLoggedIn ? "检测到当前无痕页已处于登录状态，无需再点登录按钮，可直接进入 Session 读取或后续流程。" : (loginClickReady ? (data.email_mode_switched ? "已点击使用电子邮箱继续，登录/注册邮箱窗口已出现。" : (data.login_surface_ready && !data.email_input_ready ? "登录/注册窗口已出现，正在进入邮箱填入步骤。" : "已点击登录按钮，登录/注册邮箱窗口已出现。")) : (targetSwitching ? "登录页面正在跳转或切换目标页，系统会自动重试。" : (data.error || "未检测到登录/注册邮箱窗口。"))));
    if (alreadyLoggedIn) {
      announceLuckMailScene("already_logged_in", "检测到当前页面已登录，无需再点登录按钮");
    } else if (loginClickReady) {
      announceLuckMailScene("login_window_ready", data.email_mode_switched ? "已切换为电子邮箱登录，邮箱输入窗口已出现" : "登录窗口已出现，可以填写邮箱");
    } else if (targetSwitching) {
      announceLuckMailScene("login_target_switching", "登录页面正在跳转，系统正在重试识别");
    } else {
      announceLuckMailScene("login_click_failed", data.error || "未检测到登录窗口，需要检查无痕页面");
    }
    return returnData ? data : loginClickReady;
  } catch (error) {
    loginClickReady = false;
    if (luckmailBadge) { setText(luckmailBadge, "错误"); luckmailBadge.className = "badge error"; }
    setText(luckmailMailMeta, error.message || "登录按钮自动点击请求失败");
    announceLuckMailScene("login_click_error", error.message || "登录按钮自动点击请求失败");
    renderLuckMailOutput({ ok: false, stage: "login_click_error", error: error.message });
    return returnData ? { ok: false, stage: "login_click_error", error: error.message } : false;
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
      message: "当前 access token 已失效，系统无法继续 Popay。",
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
    title: activePaymentLabel + " 失败",
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

async function runCheckoutGenerate(activeSubmitButton, options) {
  options = options || {};
  if (!options.paymentMethod && activeSubmitButton?.dataset?.paymentMethod) {
    options.paymentMethod = activeSubmitButton.dataset.paymentMethod;
  }
  resetResult();
  paymentSubmitButtons.forEach(function (button) { button.disabled = true; });
  var activeSubmitLabel = activeSubmitButton ? activeSubmitButton.textContent : "Popay";
  activePaymentLabel = activeSubmitLabel || "Popay";
  setText(activeSubmitButton, options.fetchSessionFirst ? "读取 Session..." : "生成中");
  setText(latency, options.fetchSessionFirst ? "读取 Session" : "运行中");

  const startedAt = performance.now();
  try {
    if (options.fetchSessionFirst) {
      await fetchLatestSessionJSON();
      setText(activeSubmitButton, "生成中");
      setText(latency, "运行中");
    }
    const { response, data } = await postJSON("/api/checkout/start", buildStartPayload(options));
    renderResult(data, Math.round(performance.now() - startedAt), response.ok);
    if (response.ok && latestCheckoutURL) {
      void copyCurrentCheckoutURL();
      void openPaymentInIncognito();
    }
  } catch (error) {
    renderResult({ error: error.message }, Math.round(performance.now() - startedAt), false);
  } finally {
    paymentSubmitButtons.forEach(function (button) { button.disabled = false; });
    setText(activeSubmitButton, activeSubmitLabel);
  }
}

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  var activeSubmitButton = event.submitter && paymentSubmitButtons.includes(event.submitter) ? event.submitter : submitButton;
  await runCheckoutGenerate(activeSubmitButton, { fetchSessionFirst: false });
});

paypalSubmitButton?.addEventListener("click", async () => {
  await runCheckoutGenerate(paypalSubmitButton, {
    paymentMethod: "paypal",
    fetchSessionFirst: true,
    billingDetails: {
      country: "US",
      currency: "USD",
    },
  });
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

openIncognitoBtn?.addEventListener("click", async () => {
  var opened = await ensureIncognitoWindow();
  if (!opened) return;
  var loginReady = await runLoginClickAfterIncognitoOpen();
  if (!loginReady) return;
  if (!purchasedEmailValue()) {
    await runLuckMailLoadPurchase();
  }
  if (purchasedEmailValue()) {
    await runLuckMailLoginEmailFill({ codeRetry: { maxAttempts: 3 } });
  }
});

fetchSessionBtn?.addEventListener("click", () => {
  void autoFetchAndGenerate();
});

incognitoDiagRefreshBtn?.addEventListener("click", () => {
  void refreshIncognitoDiagnostics();
});

incognitoDiagOpenBtn?.addEventListener("click", () => {
  void refreshIncognitoDiagnostics({ openFirst: true });
});

exportSub2APIBtn?.addEventListener("click", () => {
  void exportSub2APICredentials();
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
let gopayMidtransRetryNotBefore = 0;
let gopayMidtransCooldownNoticeKey = "";
const gopayAutoTriggeredCheckoutKeys = new Set();
const gptPlusSuccessRecordKeys = new Set();

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
      var paymentCompleted = !!(data.payment_completed ?? result.payment_completed);
      var stageKey = [
        pageStage,
        pinStage,
        autoActionStage,
        paymentCompleted ? "payment-completed" : "",
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

      if (paymentCompleted || pageStage === "gopay_complete") {
        setGopayStep("success", "done", "✅ 支付已完成");
        appendCheckoutWatcherEvent("gopay-complete", "检测到 GoPay 支付完成，开始保存 GPT Plus 成功记录");
        if (gopayBadge) {
          setText(gopayBadge, "支付完成");
          gopayBadge.className = "badge";
        }
        await saveGPTPlusSuccessRecord(Object.assign({}, data, { stage: "gopay_complete" }));
        stop = true;
        return;
      }

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
            var cooldownRemainingMs = gopayMidtransRetryNotBefore - Date.now();
            if (cooldownRemainingMs > 0) {
              var cooldownSeconds = Math.ceil(cooldownRemainingMs / 1000);
              var cooldownKey = linkingURL + "|" + Math.ceil(gopayMidtransRetryNotBefore / 5000);
              if (cooldownKey !== gopayMidtransCooldownNoticeKey) {
                gopayMidtransCooldownNoticeKey = cooldownKey;
                appendCheckoutWatcherEvent("midtrans-linking-cooldown", "Midtrans 页面冷却中，约 " + cooldownSeconds + " 秒后再尝试");
              }
              if (monitorBadge) {
                monitorBadge.textContent = "冷却中";
                monitorBadge.className = "badge neutral";
              }
            } else {
            appendCheckoutWatcherEvent("auto-trigger-ready", "条件满足，填写当前 Midtrans GoPay 页面：" + midtransRedirectionAccountID(linkingURL));
            gopayAutoTriggerRunning = true;
            try {
              var fillResult = await fillCurrentMidtransLinkingPage(linkingURL, checkoutURL);
              if (gopayOutput) setText(gopayOutput, JSON.stringify(fillResult, null, 2));
              if (fillResult.ok) {
                gopayMidtransRetryNotBefore = 0;
                gopayMidtransCooldownNoticeKey = "";
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
                var retryHint = fillResult.aggressive_retry ? "（保守恢复已开启）" : "";
                var fillStage = fillResult.stage || fillResult.error || "unknown";
                var retryAfterMs = Number(fillResult.retry_after_ms || fillResult.cooldown_ms || 0);
                if (!Number.isFinite(retryAfterMs)) retryAfterMs = 0;
                if (!retryAfterMs && fillStage === "midtrans_linking_rate_limited") retryAfterMs = 90000;
                if (!retryAfterMs && (fillStage === "midtrans_linking_blank_shell" || fillStage === "midtrans_linking_loading_stuck")) retryAfterMs = 30000;
                if (retryAfterMs > 0) {
                  retryAfterMs = Math.min(Math.max(retryAfterMs, 5000), 120000);
                  gopayMidtransRetryNotBefore = Date.now() + retryAfterMs;
                  gopayMidtransCooldownNoticeKey = "";
                  appendCheckoutWatcherEvent("midtrans-linking-cooldown", "页面处于 " + fillStage + "，冷却 " + Math.ceil(retryAfterMs / 1000) + " 秒后再尝试");
                  if (monitorBadge) {
                    monitorBadge.textContent = fillStage === "midtrans_linking_rate_limited" ? "已限流" : "加载中";
                    monitorBadge.className = "badge neutral";
                  }
                }
                appendCheckoutWatcherEvent("midtrans-linking-fill", "页面填充未完成：" + fillStage + retryHint);
                if (monitorBadge) {
                  monitorBadge.textContent = retryAfterMs > 0 ? "冷却中" : "继续等待";
                  monitorBadge.className = "badge neutral";
                }
              }
            } finally {
              gopayAutoTriggerRunning = false;
            }
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
      if (data.stage === "gopay_complete") await saveGPTPlusSuccessRecord(data);
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
// LuckMail 接码
// =================================
const luckmailProjectCode = document.querySelector("#luckmailProjectCode");
const luckmailEmailType = document.querySelector("#luckmailEmailType");
const luckmailDomain = document.querySelector("#luckmailDomain");
const luckmailSpecifiedEmail = document.querySelector("#luckmailSpecifiedEmail");
const luckmailTimeoutS = document.querySelector("#luckmailTimeoutS");
const luckmailIntervalS = document.querySelector("#luckmailIntervalS");
const luckmailToken = document.querySelector("#luckmailToken");
const luckmailPurchasedEmail = document.querySelector("#luckmailPurchasedEmail");
const luckmailManualCode = document.querySelector("#luckmailManualCode");
const luckmailModeHint = document.querySelector("#luckmailModeHint");
const luckmailTokenTimeoutS = document.querySelector("#luckmailTokenTimeoutS");
const luckmailTokenIntervalS = document.querySelector("#luckmailTokenIntervalS");
const luckmailDeviceLoginGuideBtn = document.querySelector("#luckmailDeviceLoginGuideBtn");
const luckmailLoadPurchaseBtn = document.querySelector("#luckmailLoadPurchaseBtn");
const luckmailLoginEmailFillBtn = document.querySelector("#luckmailLoginEmailFillBtn");
const luckmailManualCodeFillBtn = document.querySelector("#luckmailManualCodeFillBtn");
const luckmailTokenCodeBtn = document.querySelector("#luckmailTokenCodeBtn");
const luckmailTokenMailsBtn = document.querySelector("#luckmailTokenMailsBtn");
const luckmailShareCodeLinkBtn = document.querySelector("#luckmailShareCodeLinkBtn");
const luckmailCreateAndWaitBtn = document.querySelector("#luckmailCreateAndWaitBtn");
const luckmailCopyCodeBtn = document.querySelector("#luckmailCopyCodeBtn");
const luckmailBadge = document.querySelector("#luckmailBadge");
const luckmailVoicePromptToggle = document.querySelector("#luckmailVoicePromptToggle");
const luckmailVoicePromptText = document.querySelector("#luckmailVoicePromptText");
const luckmailResult = document.querySelector("#luckmailResult");
const luckmailEmailAddress = document.querySelector("#luckmailEmailAddress");
const luckmailVerificationCode = document.querySelector("#luckmailVerificationCode");
const luckmailLatency = document.querySelector("#luckmailLatency");
const luckmailMailMeta = document.querySelector("#luckmailMailMeta");
const luckmailOutput = document.querySelector("#luckmailOutput");
const luckmailMailsPanel = document.querySelector("#luckmailMailsPanel");
const luckmailMailsCount = document.querySelector("#luckmailMailsCount");
const luckmailMailsEmail = document.querySelector("#luckmailMailsEmail");
const luckmailMailsList = document.querySelector("#luckmailMailsList");
let latestLuckMailCode = "";
let luckMailVerificationSinceUnixMS = 0;
let lastVoicePromptKey = "";

function voicePromptEnabled() {
  return !!(luckmailVoicePromptToggle && luckmailVoicePromptToggle.checked && "speechSynthesis" in window);
}

function announceLuckMailScene(key, message, force) {
  if (!message) return;
  if (luckmailVoicePromptText) setText(luckmailVoicePromptText, message);
  if (!voicePromptEnabled()) return;
  var promptKey = String(key || message);
  if (!force && lastVoicePromptKey === promptKey) return;
  lastVoicePromptKey = promptKey;
  window.speechSynthesis.cancel();
  var utterance = new SpeechSynthesisUtterance(message);
  utterance.lang = "zh-CN";
  utterance.rate = 1;
  utterance.pitch = 1;
  window.speechSynthesis.speak(utterance);
}

luckmailVoicePromptToggle?.addEventListener("change", function () {
  if (luckmailVoicePromptToggle.checked) {
    announceLuckMailScene("voice_enabled", "语音播报已开启", true);
  } else {
    if ("speechSynthesis" in window) window.speechSynthesis.cancel();
    lastVoicePromptKey = "";
    if (luckmailVoicePromptText) setText(luckmailVoicePromptText, "未开启");
  }
});

luckmailToken?.addEventListener("input", function () {
  if (hasLuckMailToken()) {
    setLuckMailMode("detect", "检测到 Token，读取成功后将自动切换为自动模式");
  } else {
    setLuckMailMode("manual", "未填写 Token，请使用手动邮箱和验证码");
  }
});

luckmailPurchasedEmail?.addEventListener("input", function () {
  if (!hasLuckMailToken() && purchasedEmailValue()) {
    setLuckMailMode("manual", "已填写手动邮箱，验证码需手动输入或提交");
  }
});

function numberInputValue(node, fallback) {
  var value = Number((node?.value || "").trim());
  if (!Number.isFinite(value) || value <= 0) return fallback;
  return Math.round(value);
}

function renderLuckMailData(data, elapsed) {
  if (data?.email_address && hasLuckMailToken()) {
    applyLuckMailPurchase({ email_address: data.email_address, luckmail_token: (luckmailToken?.value || "").trim() });
  }
  latestLuckMailCode = data.verification_code || "";
  setText(luckmailEmailAddress, data.email_address || "-");
  setText(luckmailVerificationCode, latestLuckMailCode || "-");
  setText(luckmailLatency, (data.elapsed_ms || elapsed) + "ms");
  setText(luckmailMailMeta, [data.mail_from || data.project, data.mail_subject || (data.has_new_mail ? "已发现新邮件" : "未发现新邮件")].filter(Boolean).join(" · ") || (data.error || data.status || "-"));
  renderLuckMailOutput(data);
  if (luckmailCopyCodeBtn) luckmailCopyCodeBtn.disabled = !latestLuckMailCode;
  if (luckmailBadge) {
    setText(luckmailBadge, data.ok ? "已收到" : "未收到");
    luckmailBadge.className = data.ok ? "badge" : "badge error";
  }
  if (latestLuckMailCode) {
    announceLuckMailScene("token_code_received", "验证码已收到", true);
  } else if (data.ok) {
    announceLuckMailScene("token_mail_checked", data.has_new_mail ? "已发现新邮件，但未提取到验证码" : "已查询邮箱，暂未发现验证码");
  } else {
    announceLuckMailScene("token_code_failed", data.error || "验证码查询失败");
  }
}

async function runLuckMailCodeFill(code) {
  var cleanCode = String(code || "").trim();
  if (!cleanCode) return null;
  if (luckmailBadge) { setText(luckmailBadge, "填验证码"); luckmailBadge.className = "badge neutral"; }
  setText(luckmailMailMeta, "已收到验证码，正在无痕窗口验证码页面自动填入并提交...");
  announceLuckMailScene("code_fill_started", "正在填写验证码并提交");
  var response = await fetch("/api/login/code-fill", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify({ code: cleanCode, timeout_s: 60 }),
  });
  var data = await response.json();
  renderLuckMailOutput(data);
  if (luckmailBadge) {
    setText(luckmailBadge, data.login_completed ? "登录完成" : (data.profile_filled ? "资料已填" : (data.code_rejected ? "验证码错误" : (data.code_filled ? "已填验证码" : "填码失败"))));
    luckmailBadge.className = data.ok ? "badge" : "badge error";
  }
  var profileMessage = data.profile_filled ? ("已自动填写资料页：" + [data.profile_name, data.profile_age ? (data.profile_age + "岁") : ""].filter(Boolean).join(" · ")) : "";
  setText(luckmailMailMeta, data.login_completed ? "验证码已自动填入并提交，登录流程已完成。" : (profileMessage || (data.code_rejected ? "页面提示验证码错误，准备等待新的验证码。" : (data.code_filled ? "验证码已自动填入并提交，等待页面完成跳转。" : (data.error || "验证码自动填入失败")))));
  if (data.login_completed) {
    clearLuckMailResolvedEmailAfterCompletion();
    announceLuckMailScene("login_completed", "验证码已提交，登录流程已完成", true);
  } else if (data.profile_filled) {
    announceLuckMailScene("login_profile_filled", profileMessage || "资料页已自动填写并提交", true);
  } else if (data.code_rejected) {
    announceLuckMailScene("code_rejected", "页面提示验证码错误，准备等待新的验证码");
  } else if (data.code_filled) {
    announceLuckMailScene("code_filled", "验证码已填入并提交，等待页面跳转");
  } else {
    announceLuckMailScene("code_fill_failed", data.error || "验证码自动填入失败");
  }
  return data;
}

async function waitTokenCodeAndFillLoginCode(options) {
  if (!hasLuckMailToken()) {
    switchLuckMailManualMode("验证码页面已出现，但未读取到有效 Token，请手动填写收到的验证码后点击手动填验证码。");
    return { ok: false, stage: "manual_code_required", manual_mode: true, error: "luckmail_token is required" };
  }
  var maxAttempts = Math.max(1, Number(options?.maxAttempts || 1));
  var lastResult = null;
  for (var attempt = 1; attempt <= maxAttempts; attempt += 1) {
    setText(luckmailMailMeta, attempt > 1 ? ("验证码页仍在等待，正在第 " + attempt + " 次获取新的 LuckMail 验证码...") : "验证码页已出现，正在通过 LuckMail 已购邮箱 Token 等待验证码...");
    announceLuckMailScene(attempt > 1 ? "verification_code_retry_waiting" : "verification_page_ready", attempt > 1 ? "正在等待新的邮箱验证码" : "已进入验证码页面，正在等待邮箱验证码");
    if (luckmailBadge) { setText(luckmailBadge, attempt > 1 ? ("重试 " + attempt + "/" + maxAttempts) : "等验证码"); luckmailBadge.className = "badge neutral"; }
    var response = await fetch("/api/luckmail/token-code", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify(luckMailTokenPayload()),
    });
    var data = await response.json();
    if (data.email_address) applyLuckMailPurchase({ email_address: data.email_address, luckmail_token: (luckmailToken?.value || "").trim() });
    latestLuckMailCode = data.verification_code || "";
    setText(luckmailEmailAddress, data.email_address || purchasedEmailValue() || "-");
    setText(luckmailVerificationCode, latestLuckMailCode || "-");
    setText(luckmailLatency, (data.elapsed_ms || "-") + "ms");
    renderLuckMailOutput(data);
    if (!latestLuckMailCode) {
      if (luckmailBadge) { setText(luckmailBadge, "手动模式"); luckmailBadge.className = "badge neutral"; }
      switchLuckMailManualMode(tokenFallbackMessage(data));
      renderLuckMailOutput(Object.assign({}, data, { manual_mode: true }));
      return data;
    }
    announceLuckMailScene("verification_code_received", attempt > 1 ? "新的验证码已收到，正在自动填入" : "验证码已收到，正在自动填入", true);
    if (luckmailCopyCodeBtn) luckmailCopyCodeBtn.disabled = false;
    lastResult = await runLuckMailCodeFill(latestLuckMailCode);
    if (!lastResult?.code_rejected) return lastResult;
    if (attempt >= maxAttempts) return lastResult;
    luckMailVerificationSinceUnixMS = Date.now() - 1000;
    latestLuckMailCode = "";
    setText(luckmailVerificationCode, "-");
    setText(luckmailMailMeta, "验证码被页面判定错误，已切换为只等待新的邮件验证码后重试。");
    await new Promise(function (resolve) { setTimeout(resolve, 1500); });
  }
  return lastResult;
}

function luckMailTokenPayload() {
  var payload = {
    luckmail_token: (luckmailToken?.value || "").trim(),
    timeout_s: numberInputValue(luckmailTokenTimeoutS, 300),
    interval_s: numberInputValue(luckmailTokenIntervalS, 3),
  };
  if (luckMailVerificationSinceUnixMS > 0) payload.since_unix_ms = luckMailVerificationSinceUnixMS;
  return payload;
}

function luckMailShareCodeLink() {
  var token = (luckmailToken?.value || "").trim();
  if (!token) return "";
  return location.origin + "/mail-code?token=" + encodeURIComponent(token);
}

async function copyLuckMailShareCodeLink() {
  var link = luckMailShareCodeLink();
  if (!link) {
    switchLuckMailManualMode("未填写 LuckMail Token，无法生成验证码查看链接。");
    renderLuckMailOutput({ ok: false, stage: "luckmail_share_link_failed", error: "luckmail_token is required" });
    return;
  }
  await navigator.clipboard.writeText(link);
  setText(luckmailMailMeta, "已复制验证码查看链接，可发送给邮箱使用者打开查看验证码。");
  if (luckmailBadge) { setText(luckmailBadge, "链接已复制"); luckmailBadge.className = "badge"; }
  renderLuckMailOutput({ ok: true, stage: "luckmail_share_link_copied", code_view_url: link });
}

function purchasedEmailValue() {
  return (luckmailPurchasedEmail?.value || "").trim();
}

async function saveGPTPlusSuccessRecord(paymentData) {
  var email = purchasedEmailValue() || (fields.customerEmail?.value || "").trim();
  var token = (luckmailToken?.value || "").trim();
  var checkoutURL = latestOpenedCheckoutURL || latestCheckoutURL || "";
  if (!email || !token) {
    appendCheckoutWatcherEvent("gptpls-record-skipped", "GPT Plus 成功记录未保存：缺少" + (!email && !token ? "邮箱和 LuckMail Token" : (!email ? "邮箱" : "LuckMail Token")));
    return null;
  }
  var recordKey = [email.toLowerCase(), token, checkoutURL].join("|");
  if (gptPlusSuccessRecordKeys.has(recordKey)) {
    appendCheckoutWatcherEvent("gptpls-record-duplicate", "GPT Plus 成功记录已保存过，本次跳过重复写入");
    return null;
  }
  gptPlusSuccessRecordKeys.add(recordKey);
  try {
    var response = await fetch("/api/gptpls/record", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({
        email_address: email,
        luckmail_token: token,
        payment_stage: paymentData?.stage || "gopay_complete",
        payment_method: activePaymentLabel || "GoPay",
        checkout_url: checkoutURL,
      }),
    });
    var data = await response.json();
    if (!data.ok) gptPlusSuccessRecordKeys.delete(recordKey);
    appendCheckoutWatcherEvent("gptpls-record", data.ok ? ("已保存 GPT Plus 成功记录：" + (data.file_path || "gptpls") + (data.code_view_url ? " · 验证码查看链接已生成" : "")) : (data.error || "GPT Plus 成功记录保存失败"));
    return data;
  } catch (error) {
    gptPlusSuccessRecordKeys.delete(recordKey);
    appendCheckoutWatcherEvent("gptpls-record-error", error.message || "GPT Plus 成功记录保存失败");
    return null;
  }
}

function hasLuckMailToken() {
  return !!(luckmailToken?.value || "").trim();
}

function setLuckMailMode(mode, message) {
  if (!luckmailModeHint) return;
  var label = mode === "auto" ? "自动模式" : (mode === "manual" ? "手动模式" : "自动检测");
  luckmailModeHint.textContent = "当前模式：" + label + (message ? " · " + message : "");
  luckmailModeHint.dataset.mode = mode || "detect";
}

function switchLuckMailManualMode(message) {
  setLuckMailMode("manual", message || "请手动填写邮箱地址和验证码");
  if (luckmailBadge) { setText(luckmailBadge, "手动模式"); luckmailBadge.className = "badge neutral"; }
  setText(luckmailMailMeta, message || "未读取到有效 LuckMail Token，请手动填写邮箱地址和验证码。");
  announceLuckMailScene("manual_mode_enabled", message || "已切换为手动模式，请手动填写邮箱和验证码");
}

function switchLuckMailAutoMode(message) {
  setLuckMailMode("auto", message || "将自动读取邮箱并等待验证码");
}

function tokenFallbackMessage(data) {
  var raw = String(data?.error || data?.message || "").trim();
  if (!hasLuckMailToken()) return "未填写 LuckMail Token，已切换为手动模式。";
  if (/expired|expire|过期|invalid|无效|not found|不存在|alive|disabled|不可用/i.test(raw)) {
    return "LuckMail Token 无效、过期或邮箱不可用，已切换为手动模式。";
  }
  return raw ? (raw + "，已切换为手动模式。") : "未读取到有效 LuckMail Token，已切换为手动模式。";
}

function formatLuckMailDate(value) {
  if (!value) return "时间未知";
  var date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function maskLuckMailDisplayValue(value) {
  var text = String(value || "");
  if (text.length <= 8) return "***";
  return text.slice(0, 4) + "***" + text.slice(-4);
}

function sanitizeLuckMailDisplayPayload(value) {
  if (Array.isArray(value)) {
    return value.map(sanitizeLuckMailDisplayPayload);
  }
  if (!value || typeof value !== "object") {
    return value;
  }
  var result = {};
  Object.keys(value).forEach(function (key) {
    var lowered = key.toLowerCase();
    if (lowered === "luckmail_token" || lowered === "token" || lowered.includes("api_key")) {
      result[key] = maskLuckMailDisplayValue(value[key]);
      return;
    }
    result[key] = sanitizeLuckMailDisplayPayload(value[key]);
  });
  return result;
}

function renderLuckMailOutput(data) {
  setText(luckmailOutput, JSON.stringify(sanitizeLuckMailDisplayPayload(data), null, 2));
}

function renderLuckMailMails(data) {
  var mails = Array.isArray(data?.mails) ? data.mails : [];
  if (luckmailMailsPanel) luckmailMailsPanel.hidden = false;
  setText(luckmailMailsCount, mails.length + " 封");
  setText(luckmailMailsEmail, data?.email_address || purchasedEmailValue() || "-");
  if (!luckmailMailsList) return;
  luckmailMailsList.innerHTML = "";
  if (!mails.length) {
    var empty = document.createElement("div");
    empty.className = "luckmail-mails-empty";
    empty.textContent = data?.ok ? "LuckMail 当前没有返回邮件。" : (data?.error || "邮件列表查询失败");
    luckmailMailsList.appendChild(empty);
    return;
  }
  mails.forEach(function (mail, index) {
    var item = document.createElement("article");
    item.className = "luckmail-mail-card";

    var head = document.createElement("div");
    head.className = "luckmail-mail-card__head";

    var number = document.createElement("span");
    number.className = "luckmail-mail-card__number";
    number.textContent = "#" + (index + 1);

    var time = document.createElement("span");
    time.className = "luckmail-mail-card__time";
    time.textContent = formatLuckMailDate(mail.received_at);

    head.appendChild(number);
    head.appendChild(time);

    var subject = document.createElement("strong");
    subject.className = "luckmail-mail-card__subject";
    subject.textContent = mail.subject || "无主题";

    var from = document.createElement("p");
    from.className = "luckmail-mail-card__from";
    from.textContent = mail.from ? ("发件人：" + mail.from) : "发件人未知";

    var message = document.createElement("p");
    message.className = "luckmail-mail-card__id";
    message.textContent = mail.message_id ? ("Message ID: " + mail.message_id) : "无 Message ID";

    item.appendChild(head);
    item.appendChild(subject);
    item.appendChild(from);
    item.appendChild(message);
    luckmailMailsList.appendChild(item);
  });
}

function applyLuckMailPurchase(item) {
  if (!item) return false;
  var email = (item.email_address || "").trim();
  var token = (item.luckmail_token || "").trim();
  if (luckmailPurchasedEmail && email) luckmailPurchasedEmail.value = email;
  if (luckmailToken && token) luckmailToken.value = token;
  if (fields.customerEmail && email) fields.customerEmail.value = email;
  setText(luckmailEmailAddress, email || "-");
  return !!email;
}

function clearLuckMailResolvedEmailAfterCompletion() {
  if (!hasLuckMailToken()) return;
  if (luckmailPurchasedEmail) luckmailPurchasedEmail.value = "";
  if (fields.customerEmail) fields.customerEmail.value = "";
  setText(luckmailEmailAddress, "-");
  luckMailVerificationSinceUnixMS = 0;
}

async function resolveLuckMailEmailByToken() {
  if (!hasLuckMailToken()) {
    return { ok: false, manual_mode: true, error: "luckmail_token is required" };
  }
  setText(luckmailMailMeta, "正在按当前 Token 精确读取对应邮箱...");
  announceLuckMailScene("purchase_loading", "正在按 Token 读取对应邮箱");
  var startedAt = performance.now();
  var tokenValue = luckmailToken?.value?.trim() || "";
  var tokenResp = await fetch("/api/luckmail/token-mails", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify(luckMailTokenPayload()),
  });
  var tokenData = await tokenResp.json();
  var tokenElapsed = Math.round(performance.now() - startedAt);
  var tokenSelected = tokenData.email_address ? { email_address: tokenData.email_address, luckmail_token: tokenValue } : null;
  var tokenApplied = tokenResp.ok && tokenData.ok && applyLuckMailPurchase(tokenSelected);
  setText(luckmailLatency, (tokenData.elapsed_ms || tokenElapsed) + "ms");
  renderLuckMailOutput(tokenApplied ? Object.assign({}, tokenData, { stage: "luckmail_email_preloaded" }) : Object.assign({}, tokenData, { stage: "luckmail_email_preload_failed", manual_mode: true }));
  if (tokenApplied) {
    if (luckmailBadge) { setText(luckmailBadge, "自动模式"); luckmailBadge.className = "badge"; }
    switchLuckMailAutoMode("已按当前 Token 精确读取并填入对应邮箱");
    setText(luckmailMailMeta, "已按当前 Token 读取并填入：" + tokenSelected.email_address);
    announceLuckMailScene("purchase_loaded", "已按当前 Token 读取并填入已购邮箱");
    return { ok: true, email: tokenSelected.email_address, data: tokenData, token_mode: true };
  }
  var fallback = tokenFallbackMessage(tokenData);
  switchLuckMailManualMode(fallback);
  return { ok: false, manual_mode: true, error: fallback, data: tokenData, token_mode: true };
}

async function ensureLuckMailEmailForGuide() {
  if (hasLuckMailToken()) return await resolveLuckMailEmailByToken();
  var existingEmail = purchasedEmailValue();
  if (existingEmail) {
    if (fields.customerEmail) fields.customerEmail.value = existingEmail;
    setText(luckmailEmailAddress, existingEmail);
    setLuckMailMode("manual", "已使用手动邮箱，验证码需要手动填写");
    return { ok: true, email: existingEmail, reused: true };
  }
  if (!hasLuckMailToken()) {
    switchLuckMailManualMode("未读取到有效 LuckMail Token，请先手动填写邮箱地址。填写后可继续执行登录页填入邮箱。");
    return { ok: false, manual_mode: true, error: "luckmail_token is required" };
  }
  return { ok: false, manual_mode: true, error: "luckmail_token is required" };
}

async function runLuckMailDeviceLoginGuide() {
  if (!luckmailDeviceLoginGuideBtn) return;
  var originalText = luckmailDeviceLoginGuideBtn.textContent;
  luckmailDeviceLoginGuideBtn.disabled = true;
  if (luckmailLoadPurchaseBtn) luckmailLoadPurchaseBtn.disabled = true;
  if (luckmailLoginEmailFillBtn) luckmailLoginEmailFillBtn.disabled = true;
  if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = true;
  if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = true;
  if (luckmailResult) luckmailResult.hidden = false;
  if (luckmailBadge) { setText(luckmailBadge, "引导中"); luckmailBadge.className = "badge neutral"; }
  setText(luckmailVerificationCode, "-");
  setText(luckmailMailMeta, "正在启动设备登录引导：读取 Token 与已购邮箱、打开无痕窗口、识别登录入口并等待验证码...");
  announceLuckMailScene("device_login_guide_started", "设备登录引导已启动");
  renderLuckMailOutput({ stage: "device_login_guide_started" });
  setLuckMailMode(hasLuckMailToken() ? "auto" : "manual", hasLuckMailToken() ? "将先读取邮箱再打开登录窗口" : "未填写 Token，将使用手动邮箱和验证码");
  setText(luckmailDeviceLoginGuideBtn, "读取邮箱...");
  try {
    var emailReady = await ensureLuckMailEmailForGuide();
    if (!emailReady.ok) throw new Error(emailReady.error || "未读取到可用邮箱，请手动填写邮箱地址和验证码");
    setText(luckmailDeviceLoginGuideBtn, "启动窗口...");
    setText(luckmailMailMeta, "已准备邮箱，正在启动无痕窗口并识别登录入口...");
    var opened = await ensureIncognitoWindow();
    if (!opened) throw new Error("无法启动 Chrome 无痕窗口");
    setText(luckmailDeviceLoginGuideBtn, "识别登录...");
    setText(luckmailMailMeta, "无痕窗口已打开，正在确认登录/注册邮箱窗口...");
    var loginData = await runLoginClickAfterIncognitoOpen({ returnData: true });
    var loginReady = !!(loginData && loginData.ok && (loginData.email_input_ready || loginData.email_mode_switched || loginData.login_surface_ready));
    if (!loginReady) {
      var detail = Object.assign({}, loginData || {}, { ok: false, stage: "device_login_guide_login_click_failed", device_login_guide_failed: true });
      renderLuckMailOutput(detail);
      throw Object.assign(new Error((loginData && loginData.error) || "登录按钮未触发或未检测到登录/注册邮箱窗口"), { detail: detail });
    }
    if (!purchasedEmailValue()) {
      switchLuckMailManualMode("未读取到可用邮箱，请手动填写邮箱地址和验证码。");
      throw new Error("未读取到可用邮箱，请手动填写邮箱地址和验证码");
    }
    if (fields.customerEmail) fields.customerEmail.value = purchasedEmailValue();
    setText(luckmailDeviceLoginGuideBtn, "填邮箱...");
    setText(luckmailMailMeta, "已准备邮箱，正在填入登录页并等待验证码...");
    var guideResult = await runLuckMailLoginEmailFill({ codeRetry: { maxAttempts: 3 } });
    if (guideResult?.login_completed) {
      if (luckmailBadge) { setText(luckmailBadge, "登录完成"); luckmailBadge.className = "badge"; }
      announceLuckMailScene("device_login_guide_finished", "设备登录引导已完成，登录成功", true);
    } else if (guideResult?.code_rejected) {
      if (luckmailBadge) { setText(luckmailBadge, "验证码错误"); luckmailBadge.className = "badge error"; }
      announceLuckMailScene("device_login_guide_finished_with_code_error", "设备登录引导已重试，但验证码仍被页面判定错误");
    } else {
      if (luckmailBadge && latestLuckMailCode) { setText(luckmailBadge, "已填验证码"); luckmailBadge.className = "badge"; }
      announceLuckMailScene("device_login_guide_finished", latestLuckMailCode ? "设备登录引导已完成，验证码已填入" : "设备登录引导已执行，请查看页面状态", true);
    }
  } catch (error) {
    if (luckmailBadge) { setText(luckmailBadge, "引导失败"); luckmailBadge.className = "badge error"; }
    setText(luckmailMailMeta, error.message || "设备登录引导失败");
    announceLuckMailScene("device_login_guide_failed", error.message || "设备登录引导失败");
    renderLuckMailOutput(error.detail || { ok: false, stage: "device_login_guide_failed", error: error.message });
  } finally {
    luckmailDeviceLoginGuideBtn.disabled = false;
    if (luckmailLoadPurchaseBtn) luckmailLoadPurchaseBtn.disabled = false;
    if (luckmailLoginEmailFillBtn) luckmailLoginEmailFillBtn.disabled = false;
    if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = false;
    if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = false;
    setText(luckmailDeviceLoginGuideBtn, originalText || "设备登录引导");
  }
}

async function runLuckMailLoadPurchase() {
  if (!luckmailLoadPurchaseBtn) return;
  var originalText = luckmailLoadPurchaseBtn.textContent;
  var startedAt = performance.now();
  luckmailLoadPurchaseBtn.disabled = true;
  if (luckmailLoginEmailFillBtn) luckmailLoginEmailFillBtn.disabled = true;
  if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = true;
  if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = true;
  setText(luckmailLoadPurchaseBtn, "读取中...");
  if (luckmailBadge) { setText(luckmailBadge, "读取中"); luckmailBadge.className = "badge neutral"; }
  if (luckmailResult) luckmailResult.hidden = false;
  setText(luckmailMailMeta, (luckmailToken?.value?.trim() ? "正在按当前 Token 精确读取绑定邮箱..." : "正在从 LuckMail 已购邮箱列表读取可用邮箱..."));
  announceLuckMailScene("purchase_loading", "正在读取已购邮箱");
  renderLuckMailOutput({ stage: "luckmail_purchases_loading" });

  try {
    var tokenValue = luckmailToken?.value?.trim() || "";
    if (!tokenValue) {
      switchLuckMailManualMode("未填写 LuckMail Token，请手动填写邮箱地址和验证码。");
      setText(luckmailLatency, Math.round(performance.now() - startedAt) + "ms");
      renderLuckMailOutput({ ok: false, stage: "luckmail_manual_mode", manual_mode: true, error: "luckmail_token is required" });
      return;
    }
    if (tokenValue) {
      await resolveLuckMailEmailByToken();
      return;
    }
    var resp = await fetch("/api/luckmail/purchases", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({ page: 1, page_size: 20 }),
    });
    var data = await resp.json();
    var elapsed = Math.round(performance.now() - startedAt);
    var selected = data.selected || (data.list || []).find(function (item) { return item.email_address && item.luckmail_token && item.user_disabled === 0; }) || null;
    var applied = data.ok && applyLuckMailPurchase(selected);
    setText(luckmailLatency, (data.elapsed_ms || elapsed) + "ms");
    setText(luckmailMailMeta, applied ? ("已读取并填入：" + selected.email_address) : (data.error || "未找到可用已购邮箱"));
    renderLuckMailOutput(data);
    if (luckmailBadge) {
      setText(luckmailBadge, applied ? "已读取" : "未找到");
      luckmailBadge.className = applied ? "badge" : "badge error";
    }
    announceLuckMailScene("purchase_loaded", applied ? "已读取并填入已购邮箱" : (data.error || "未找到可用已购邮箱"));
  } catch (error) {
    var elapsed = Math.round(performance.now() - startedAt);
    setText(luckmailLatency, elapsed + "ms");
    setText(luckmailMailMeta, error.message || "LuckMail 已购邮箱读取失败");
    announceLuckMailScene("purchase_error", error.message || "LuckMail 已购邮箱读取失败");
    renderLuckMailOutput({ ok: false, error: error.message });
    if (luckmailBadge) { setText(luckmailBadge, "错误"); luckmailBadge.className = "badge error"; }
  } finally {
    luckmailLoadPurchaseBtn.disabled = false;
    if (luckmailLoginEmailFillBtn) luckmailLoginEmailFillBtn.disabled = false;
    if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = false;
    if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = false;
    setText(luckmailLoadPurchaseBtn, originalText || "读取已购邮箱");
  }
}

async function runLuckMailLoginEmailFill(options) {
  if (!luckmailLoginEmailFillBtn) return;
  var emailReady = hasLuckMailToken() ? await ensureLuckMailEmailForGuide() : { ok: !!purchasedEmailValue(), email: purchasedEmailValue() };
  var email = String(emailReady?.email || purchasedEmailValue() || "").trim();
  if (fields.customerEmail) fields.customerEmail.value = email;
  if (!emailReady?.ok || !email) {
    if (luckmailResult) luckmailResult.hidden = false;
    var fillMessage = hasLuckMailToken() ? (emailReady?.error || "未按 Token 读取到有效邮箱，请检查 Token 后重试。") : "请先手动填写邮箱地址，再执行登录页填入邮箱。";
    switchLuckMailManualMode(fillMessage);
    renderLuckMailOutput(Object.assign({ ok: false, stage: "manual_email_required", manual_mode: !hasLuckMailToken(), error: hasLuckMailToken() ? "token email is required" : "email is required" }, emailReady?.data ? { token_lookup: emailReady.data } : {}));
    return;
  }

  var originalText = luckmailLoginEmailFillBtn.textContent;
  var startedAt = performance.now();
  luckmailLoginEmailFillBtn.disabled = true;
  if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = true;
  if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = true;
  setText(luckmailLoginEmailFillBtn, "提交邮箱...");
  if (luckmailBadge) { setText(luckmailBadge, "提交邮箱"); luckmailBadge.className = "badge neutral"; }
  if (luckmailResult) luckmailResult.hidden = false;
  setText(luckmailEmailAddress, email);
  setText(luckmailVerificationCode, "-");
  setText(luckmailMailMeta, "正在监控无痕窗口，填入邮箱后点击继续按钮，并等待验证码输入页面...");
  if (hasLuckMailToken()) {
    switchLuckMailAutoMode("邮箱提交后将自动等待并填写验证码");
  } else {
    setLuckMailMode("manual", "邮箱提交后需要手动填写验证码");
  }
  announceLuckMailScene("email_fill_started", "正在填写邮箱并等待验证码页面");
  renderLuckMailOutput({ stage: "login_email_fill_started" });

  try {
    var emailSubmitStartedUnixMS = Date.now();
    var resp = await fetch("/api/login/email-fill", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({ email: email, timeout_s: 120 }),
    });
    var data = await resp.json();
    if (data.email_filled || data.clicked_continue || data.verification_ready) {
      luckMailVerificationSinceUnixMS = Math.max(0, emailSubmitStartedUnixMS - 2000);
    }
    var elapsed = Math.round(performance.now() - startedAt);
    setText(luckmailLatency, (data.elapsed_ms || elapsed) + "ms");
    var successMessage = data.verification_ready ? "已提交邮箱并进入验证码输入页面。" : (data.clicked_continue ? "已点击继续按钮，正在等待验证码页面。" : (data.email_mode_switched ? "已切换为电子邮箱方式并填入邮箱。" : "已在登录或注册页面填入邮箱。"));
    var failureMessage =
      data.operation_timed_out ? "页面提示 Operation timed out，建议稍后重试或检查网络/代理后再次提交邮箱。" :
      (data.stage === "login_email_input_not_ready_after_surface" ? "已看到登录框，系统已延长等待邮箱输入框，但输入框仍未出现；建议检查登录页网络加载状态。" : (data.error || "登录页邮箱提交失败"));
    setText(luckmailMailMeta, data.ok ? successMessage : failureMessage);
    renderLuckMailOutput(data);
    if (luckmailBadge) {
      setText(luckmailBadge, data.ok ? (data.verification_ready ? "待验证码" : "已提交") : (data.operation_timed_out ? "提交超时" : "提交失败"));
      luckmailBadge.className = data.ok ? "badge" : "badge error";
    }
    if (data.ok && data.verification_ready) {
      announceLuckMailScene("email_verification_ready", "邮箱已提交，验证码页面已出现");
      if (!hasLuckMailToken()) {
        switchLuckMailManualMode("验证码页面已出现，请手动输入收到的验证码后点击手动填验证码。");
        return data;
      }
      return await waitTokenCodeAndFillLoginCode(options?.codeRetry || {});
    } else if (data.ok) {
      announceLuckMailScene("email_submitted", successMessage);
    } else if (data.operation_timed_out) {
      announceLuckMailScene("email_operation_timed_out", "页面提示提交超时，请检查网络或代理后重试");
    } else {
      announceLuckMailScene("email_fill_failed", data.error || "登录页邮箱提交失败");
    }
  } catch (error) {
    var elapsed = Math.round(performance.now() - startedAt);
    setText(luckmailLatency, elapsed + "ms");
    setText(luckmailMailMeta, error.message || "登录页邮箱填入请求失败");
    announceLuckMailScene("email_fill_error", error.message || "登录页邮箱填入请求失败");
    renderLuckMailOutput({ ok: false, error: error.message });
    if (luckmailBadge) { setText(luckmailBadge, "错误"); luckmailBadge.className = "badge error"; }
  } finally {
    luckmailLoginEmailFillBtn.disabled = false;
    if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = false;
    if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = false;
    setText(luckmailLoginEmailFillBtn, originalText || "登录页填入邮箱");
  }
}

async function runLuckMailTokenCode() {
  if (!luckmailTokenCodeBtn) return;
  var originalText = luckmailTokenCodeBtn.textContent;
  var startedAt = performance.now();
  latestLuckMailCode = "";
  luckmailTokenCodeBtn.disabled = true;
  if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = true;
  if (luckmailCopyCodeBtn) luckmailCopyCodeBtn.disabled = true;
  setText(luckmailTokenCodeBtn, "等待已购邮箱中...");
  if (luckmailBadge) { setText(luckmailBadge, "等待中"); luckmailBadge.className = "badge neutral"; }
  if (luckmailResult) luckmailResult.hidden = false;
  setText(luckmailEmailAddress, "-");
  setText(luckmailVerificationCode, "-");
  if (!hasLuckMailToken()) {
    switchLuckMailManualMode("未填写 LuckMail Token，请手动填写邮箱地址和验证码。");
    setText(luckmailLatency, Math.round(performance.now() - startedAt) + "ms");
    renderLuckMailOutput({ ok: false, stage: "luckmail_manual_mode", manual_mode: true, error: "luckmail_token is required" });
    luckmailTokenCodeBtn.disabled = false;
    if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = false;
    setText(luckmailTokenCodeBtn, originalText || "等待已购邮箱验证码");
    return;
  }
  setText(luckmailMailMeta, "正在通过已购邮箱 Token 轮询验证码...");
  announceLuckMailScene("token_code_waiting", "正在等待已购邮箱验证码");
  renderLuckMailOutput({ stage: "luckmail_token_waiting" });

  try {
    var resp = await fetch("/api/luckmail/token-code", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify(luckMailTokenPayload()),
    });
    var data = await resp.json();
    renderLuckMailData(data, Math.round(performance.now() - startedAt));
    if (!data.ok) {
      switchLuckMailManualMode(tokenFallbackMessage(data));
      renderLuckMailOutput(Object.assign({}, data, { manual_mode: true }));
    }
  } catch (error) {
    var elapsed = Math.round(performance.now() - startedAt);
    setText(luckmailLatency, elapsed + "ms");
    setText(luckmailMailMeta, error.message || "LuckMail 已购邮箱请求失败");
    announceLuckMailScene("token_code_error", error.message || "LuckMail 已购邮箱请求失败");
    renderLuckMailOutput({ ok: false, error: error.message });
    if (luckmailBadge) { setText(luckmailBadge, "错误"); luckmailBadge.className = "badge error"; }
  } finally {
    luckmailTokenCodeBtn.disabled = false;
    if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = false;
    setText(luckmailTokenCodeBtn, originalText || "等待已购邮箱验证码");
  }
}

async function runLuckMailTokenMails() {
  if (!luckmailTokenMailsBtn) return;
  var originalText = luckmailTokenMailsBtn.textContent;
  var startedAt = performance.now();
  luckmailTokenMailsBtn.disabled = true;
  if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = true;
  setText(luckmailTokenMailsBtn, "查询邮件中...");
  if (luckmailBadge) { setText(luckmailBadge, "查询中"); luckmailBadge.className = "badge neutral"; }
  if (luckmailResult) luckmailResult.hidden = false;
  setText(luckmailMailMeta, "正在查询已购邮箱邮件列表...");
  if (!hasLuckMailToken()) {
    switchLuckMailManualMode("未填写 LuckMail Token，无法自动查询邮件列表，请手动填写邮箱地址和验证码。");
    setText(luckmailLatency, Math.round(performance.now() - startedAt) + "ms");
    renderLuckMailMails({ ok: false, error: "luckmail_token is required", mails: [] });
    renderLuckMailOutput({ ok: false, stage: "luckmail_manual_mode", manual_mode: true, error: "luckmail_token is required" });
    luckmailTokenMailsBtn.disabled = false;
    if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = false;
    setText(luckmailTokenMailsBtn, originalText || "查询邮件列表");
    return;
  }
  announceLuckMailScene("token_mails_loading", "正在查询邮件列表");
  renderLuckMailOutput({ stage: "luckmail_token_mails_loading" });

  try {
    var resp = await fetch("/api/luckmail/token-mails", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify(luckMailTokenPayload()),
    });
    var data = await resp.json();
    if (data.email_address && hasLuckMailToken()) {
      applyLuckMailPurchase({ email_address: data.email_address, luckmail_token: (luckmailToken?.value || "").trim() });
    }
    var elapsed = Math.round(performance.now() - startedAt);
    renderLuckMailMails(data);
    setText(luckmailEmailAddress, data.email_address || "-");
    setText(luckmailLatency, (data.elapsed_ms || elapsed) + "ms");
    setText(luckmailMailMeta, data.ok ? ("邮件数：" + ((data.mails || []).length) + (data.warranty_until ? " · 保修至：" + data.warranty_until : "")) : tokenFallbackMessage(data));
    renderLuckMailOutput(data.ok ? data : Object.assign({}, data, { manual_mode: true }));
    if (luckmailBadge) {
      setText(luckmailBadge, data.ok ? "自动模式" : "手动模式");
      luckmailBadge.className = data.ok ? "badge" : "badge neutral";
    }
    if (data.ok) {
      switchLuckMailAutoMode("邮件列表已查询，可继续自动等待验证码");
    } else {
      switchLuckMailManualMode(tokenFallbackMessage(data));
    }
    announceLuckMailScene("token_mails_loaded", data.ok ? ("邮件列表已查询，共 " + ((data.mails || []).length) + " 封") : tokenFallbackMessage(data));
  } catch (error) {
    var elapsed = Math.round(performance.now() - startedAt);
    setText(luckmailLatency, elapsed + "ms");
    renderLuckMailMails({ ok: false, error: error.message, mails: [] });
    setText(luckmailMailMeta, error.message || "LuckMail 邮件列表请求失败");
    announceLuckMailScene("token_mails_error", error.message || "LuckMail 邮件列表请求失败");
    renderLuckMailOutput({ ok: false, error: error.message });
    if (luckmailBadge) { setText(luckmailBadge, "错误"); luckmailBadge.className = "badge error"; }
  } finally {
    luckmailTokenMailsBtn.disabled = false;
    if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = false;
    setText(luckmailTokenMailsBtn, originalText || "查询邮件列表");
  }
}

async function runLuckMailCreateAndWait() {
  if (!luckmailCreateAndWaitBtn) return;
  var originalText = luckmailCreateAndWaitBtn.textContent;
  var startedAt = performance.now();
  latestLuckMailCode = "";
  luckmailCreateAndWaitBtn.disabled = true;
  if (luckmailCopyCodeBtn) luckmailCopyCodeBtn.disabled = true;
  setText(luckmailCreateAndWaitBtn, "等待邮件中...");
  if (luckmailBadge) { setText(luckmailBadge, "等待中"); luckmailBadge.className = "badge neutral"; }
  if (luckmailResult) luckmailResult.hidden = false;
  setText(luckmailEmailAddress, "-");
  setText(luckmailVerificationCode, "-");
  setText(luckmailMailMeta, "正在创建 LuckMail 订单并轮询验证码...");
  announceLuckMailScene("temp_order_waiting", "正在创建临时接码订单并等待验证码");
  renderLuckMailOutput({ stage: "luckmail_waiting" });

  var payload = {
    project_code: (luckmailProjectCode?.value || "openai").trim(),
    email_type: (luckmailEmailType?.value || "ms_graph").trim(),
    domain: (luckmailDomain?.value || "").trim(),
    specified_email: (luckmailSpecifiedEmail?.value || "").trim(),
    timeout_s: numberInputValue(luckmailTimeoutS, 300),
    interval_s: numberInputValue(luckmailIntervalS, 3),
  };

  try {
    var resp = await fetch("/api/luckmail/create-and-wait", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify(payload),
    });
    var data = await resp.json();
    var elapsed = Math.round(performance.now() - startedAt);
    latestLuckMailCode = data.verification_code || "";
    setText(luckmailEmailAddress, data.email_address || "-");
    setText(luckmailVerificationCode, latestLuckMailCode || "-");
    setText(luckmailLatency, (data.elapsed_ms || elapsed) + "ms");
    setText(luckmailMailMeta, [data.mail_from, data.mail_subject].filter(Boolean).join(" · ") || (data.error || data.status || "-"));
    renderLuckMailOutput(data);
    if (luckmailCopyCodeBtn) luckmailCopyCodeBtn.disabled = !latestLuckMailCode;
    if (luckmailBadge) {
      setText(luckmailBadge, data.ok ? "已收到" : "未收到");
      luckmailBadge.className = data.ok ? "badge" : "badge error";
    }
    announceLuckMailScene("temp_order_done", latestLuckMailCode ? "验证码已收到" : (data.error || "临时接码订单未收到验证码"), !!latestLuckMailCode);
  } catch (error) {
    var elapsed = Math.round(performance.now() - startedAt);
    setText(luckmailLatency, elapsed + "ms");
    setText(luckmailMailMeta, error.message || "LuckMail 请求失败");
    announceLuckMailScene("temp_order_error", error.message || "LuckMail 请求失败");
    renderLuckMailOutput({ ok: false, error: error.message });
    if (luckmailBadge) { setText(luckmailBadge, "错误"); luckmailBadge.className = "badge error"; }
  } finally {
    luckmailCreateAndWaitBtn.disabled = false;
    setText(luckmailCreateAndWaitBtn, originalText || "创建并等待验证码");
  }
}

async function runLuckMailManualCodeFill() {
  var code = manualLuckMailCodeValue();
  if (!code) {
    if (luckmailResult) luckmailResult.hidden = false;
    switchLuckMailManualMode("请先填写手动验证码，再提交到当前验证码页面。");
    renderLuckMailOutput({ ok: false, stage: "manual_code_required", manual_mode: true, error: "code is required" });
    return;
  }
  var originalText = luckmailManualCodeFillBtn?.textContent || "手动填验证码";
  if (luckmailManualCodeFillBtn) {
    luckmailManualCodeFillBtn.disabled = true;
    setText(luckmailManualCodeFillBtn, "填验证码...");
  }
  if (luckmailResult) luckmailResult.hidden = false;
  setText(luckmailVerificationCode, code);
  switchLuckMailManualMode("正在把手动验证码填入当前验证码页面。");
  try {
    await runLuckMailCodeFill(code);
  } catch (error) {
    setText(luckmailMailMeta, error.message || "手动验证码填入失败");
    announceLuckMailScene("manual_code_fill_error", error.message || "手动验证码填入失败");
    renderLuckMailOutput({ ok: false, stage: "manual_code_fill_failed", manual_mode: true, error: error.message });
    if (luckmailBadge) { setText(luckmailBadge, "填码失败"); luckmailBadge.className = "badge error"; }
  } finally {
    if (luckmailManualCodeFillBtn) {
      luckmailManualCodeFillBtn.disabled = false;
      setText(luckmailManualCodeFillBtn, originalText);
    }
  }
}

luckmailDeviceLoginGuideBtn?.addEventListener("click", runLuckMailDeviceLoginGuide);
luckmailLoadPurchaseBtn?.addEventListener("click", runLuckMailLoadPurchase);
luckmailLoginEmailFillBtn?.addEventListener("click", runLuckMailLoginEmailFill);
luckmailManualCodeFillBtn?.addEventListener("click", runLuckMailManualCodeFill);
luckmailTokenCodeBtn?.addEventListener("click", runLuckMailTokenCode);
luckmailTokenMailsBtn?.addEventListener("click", runLuckMailTokenMails);
luckmailShareCodeLinkBtn?.addEventListener("click", copyLuckMailShareCodeLink);
luckmailCreateAndWaitBtn?.addEventListener("click", runLuckMailCreateAndWait);
luckmailCopyCodeBtn?.addEventListener("click", async function () {
  if (!latestLuckMailCode) return;
  var copied = await copyText(latestLuckMailCode);
  setText(luckmailCopyCodeBtn, copied ? "已复制" : "复制失败");
  window.setTimeout(function () { setText(luckmailCopyCodeBtn, "复制验证码"); }, 1200);
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
